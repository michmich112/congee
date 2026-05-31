package nip29

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/rs/zerolog"
)

type nip29Config struct {
	LatePublicationMaxPastSeconds int  `json:"late_publication_max_past_seconds"`
	StrictPreviousSameH           bool `json:"strict_previous_same_h"`
}

type nip29 struct {
	host plugin.HostAPI
	cfg  nip29Config
}

func New() plugin.Plugin { return &nip29{} }

func (p *nip29) Manifest() plugin.Manifest { return Manifest() }

func (p *nip29) Init(host plugin.HostAPI) error {
	p.host = host
	p.cfg = defaultConfig()
	if raw := host.Config(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &p.cfg); err != nil {
			return err
		}
	}
	return nil
}

// ValidateConfig implements plugin.ConfigValidator for nip-29 settings.
func (p *nip29) ValidateConfig(settings json.RawMessage) error {
	return validateNIP29Settings(settings)
}

func validateNIP29Settings(settings json.RawMessage) error {
	var cfg nip29Config
	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return fmt.Errorf("config: nips[\"nip-29\"].settings: %w", err)
		}
	}
	if cfg.LatePublicationMaxPastSeconds < 0 {
		return errors.New("config: nips[\"nip-29\"].settings.late_publication_max_past_seconds must be >= 0 (0 uses relay default)")
	}
	return nil
}

func defaultConfig() nip29Config {
	return nip29Config{LatePublicationMaxPastSeconds: 0}
}

func (p *nip29) maxPastSec() int64 {
	if p.cfg.LatePublicationMaxPastSeconds <= 0 {
		return 86400
	}
	return int64(p.cfg.LatePublicationMaxPastSeconds)
}

func (p *nip29) ValidateEvent(ctx context.Context, ec *plugin.EventContext) error {
	if ec == nil || ec.Event == nil {
		return nil
	}
	ev := ec.Event
	if err := p.validatePrevious(ctx, ev); err != nil {
		return err
	}
	if err := p.validateLatePublication(ev); err != nil {
		return err
	}
	if err := p.validateRestrictedWrite(ctx, ev); err != nil {
		return err
	}
	return p.validateModerationAndJoinLeave(ctx, ev)
}

func (p *nip29) validatePrevious(ctx context.Context, ev *nostr.Event) error {
	gid := nostr.NIP29GroupHTag(ev)
	if gid == "" {
		return nil
	}
	prevs := nostr.NIP29PreviousTagValues(ev)
	if len(prevs) == 0 {
		return nil
	}
	for _, pref := range prevs {
		if !nostr.NIP29IsValidPreviousPrefix(pref) {
			return reject("nip-29: invalid previous id prefix")
		}
		ok, err := p.host.EventIDPrefixExists(ctx, pref, gid, p.cfg.StrictPreviousSameH)
		if err != nil {
			return fmt.Errorf("nip-29: previous check failed: %w", err)
		}
		if !ok {
			return reject("nip-29: previous references unknown event")
		}
	}
	return nil
}

func (p *nip29) validateLatePublication(ev *nostr.Event) error {
	if nostr.NIP29GroupHTag(ev) == "" {
		return nil
	}
	now := time.Now().Unix()
	if now-ev.CreatedAt > p.maxPastSec() {
		return reject("nip-29: created_at too far in the past for this group")
	}
	return nil
}

func (p *nip29) validateRestrictedWrite(ctx context.Context, ev *nostr.Event) error {
	rpk := p.host.RelayPubkey()
	if rpk == "" {
		return nil
	}
	gid := nostr.NIP29GroupHTag(ev)
	if gid == "" {
		return nil
	}
	if ev.PubKey == rpk {
		return nil
	}
	if ev.Kind == nostr.NIP29KindJoinRequest || ev.Kind == nostr.NIP29KindLeaveReq {
		return nil
	}
	if nostr.NIP29IsModerationKind(ev.Kind) {
		return nil
	}
	md, err := p.host.GetLatestGroupMetadata39000(ctx, gid)
	if err != nil || md == nil || !nostr.NIP29MetadataIsRestricted(md) {
		return nil
	}
	member, err := p.host.IsGroupMember(ctx, gid, ev.PubKey)
	if err != nil {
		return fmt.Errorf("nip-29: membership check failed: %w", err)
	}
	if !member {
		return reject("nip-29: group is restricted: not a member")
	}
	return nil
}

func (p *nip29) validateModerationAndJoinLeave(ctx context.Context, ev *nostr.Event) error {
	gid := nostr.NIP29GroupHTag(ev)
	if nostr.NIP29IsModerationKind(ev.Kind) {
		if gid == "" {
			return reject("nip-29: moderation events require an h tag")
		}
		rpk := p.host.RelayPubkey()
		if rpk == "" {
			return reject("nip-29: relay identity required for moderation kinds")
		}
		if ev.PubKey == rpk {
			return nil
		}
		switch ev.Kind {
		case nostr.NIP29KindCreateGroup, nostr.NIP29KindDeleteGroup:
			return reject(fmt.Sprintf("nip-29: moderation kind %d must be signed by the relay", ev.Kind))
		default:
			admins, err := p.host.GetLatestGroupAdmins39001(ctx, gid)
			if err != nil {
				return fmt.Errorf("nip-29: group admins lookup failed: %w", err)
			}
			if admins == nil || !nostr.NIP29Admins39001ContainsPubkey(admins, ev.PubKey) {
				return reject("nip-29: moderation not permitted for this pubkey")
			}
			return nil
		}
	}
	switch ev.Kind {
	case nostr.NIP29KindJoinRequest:
		if gid == "" {
			return reject("nip-29: join request requires an h tag")
		}
		if p.host.RelayPubkey() == "" {
			return reject("nip-29: relay identity required")
		}
		member, err := p.host.IsGroupMember(ctx, gid, ev.PubKey)
		if err != nil {
			return fmt.Errorf("nip-29: membership check failed: %w", err)
		}
		if member {
			return reject("duplicate: already a group member")
		}
		md, err := p.host.GetLatestGroupMetadata39000(ctx, gid)
		if err != nil {
			return fmt.Errorf("nip-29: group metadata lookup failed: %w", err)
		}
		if md != nil && nostr.NIP29MetadataIsClosed(md) {
			return reject("nip-29: group is closed; join requests are not accepted (invite flow not implemented)")
		}
		return nil
	case nostr.NIP29KindLeaveReq:
		if gid == "" {
			return reject("nip-29: leave request requires an h tag")
		}
		if p.host.RelayPubkey() == "" {
			return reject("nip-29: relay identity required")
		}
		member, err := p.host.IsGroupMember(ctx, gid, ev.PubKey)
		if err != nil {
			return fmt.Errorf("nip-29: membership check failed: %w", err)
		}
		if !member {
			return reject("rejected: not a group member")
		}
		return nil
	default:
		return nil
	}
}

func (p *nip29) OnEventStored(ctx context.Context, ec *plugin.EventContext) error {
	if !ec.Stored || ec.Event == nil || p.host.RelayPubkey() == "" {
		return nil
	}
	ev := ec.Event
	gid := nostr.NIP29GroupHTag(ev)
	log := p.host.Logger()
	if gid != "" {
		if err := p.ensureGroupMetadata(ctx, gid); err != nil {
			logStoreErr(log, zerolog.WarnLevel, "ensureGroupMetadata", err, "nip29 ensure group metadata failed", func(e *zerolog.Event) {
				e.Str("group_id", gid).Str("event_id", ev.ID)
			})
		}
	}
	if ev.Kind == nostr.NIP29KindJoinRequest {
		if err := p.relayPutUser(ctx, ev); err != nil {
			logStoreErr(log, zerolog.WarnLevel, "relayPutUser", err, "nip29 relay put-user after join failed", func(e *zerolog.Event) {
				e.Str("group_id", gid).Str("event_id", ev.ID)
			})
		}
	}
	if ev.Kind == nostr.NIP29KindLeaveReq {
		if err := p.relayRemoveUser(ctx, ev); err != nil {
			logStoreErr(log, zerolog.WarnLevel, "relayRemoveUser", err, "nip29 relay remove-user failed", func(e *zerolog.Event) {
				e.Str("group_id", gid).Str("event_id", ev.ID)
			})
		}
	}
	return nil
}

func (p *nip29) ensureGroupMetadata(ctx context.Context, gid string) error {
	md, err := p.host.GetLatestGroupMetadata39000(ctx, gid)
	if err != nil || md != nil {
		return err
	}
	now := time.Now().Unix()
	create := &nostr.Event{CreatedAt: now, Kind: nostr.NIP29KindCreateGroup, Tags: [][]string{{"h", gid}}, Content: ""}
	if err := p.host.SignAsRelay(ctx, create); err != nil {
		return err
	}
	if err := p.host.SaveEvent(ctx, create); err != nil {
		return err
	}
	if err := p.host.Broadcast(ctx, create); err != nil {
		return err
	}
	meta := &nostr.Event{CreatedAt: now, Kind: nostr.NIP29KindGroupMetadata, Tags: [][]string{{"d", gid}}, Content: ""}
	if err := p.host.SignAsRelay(ctx, meta); err != nil {
		return err
	}
	if err := p.host.SaveEvent(ctx, meta); err != nil {
		return err
	}
	return p.host.Broadcast(ctx, meta)
}

func (p *nip29) relayPutUser(ctx context.Context, join *nostr.Event) error {
	gid := nostr.NIP29GroupHTag(join)
	if gid == "" {
		return nil
	}
	put := &nostr.Event{
		CreatedAt: time.Now().Unix(),
		Kind:      nostr.NIP29KindPutUser,
		Tags:      [][]string{{"h", gid}, {"p", join.PubKey}},
		Content:   "",
	}
	if err := p.host.SignAsRelay(ctx, put); err != nil {
		return err
	}
	if err := p.host.SaveEvent(ctx, put); err != nil {
		return err
	}
	return p.host.Broadcast(ctx, put)
}

func (p *nip29) relayRemoveUser(ctx context.Context, leave *nostr.Event) error {
	gid := nostr.NIP29GroupHTag(leave)
	if gid == "" {
		return nil
	}
	rem := &nostr.Event{
		CreatedAt: time.Now().Unix(),
		Kind:      nostr.NIP29KindRemoveUser,
		Tags:      [][]string{{"h", gid}, {"p", leave.PubKey}},
		Content:   "",
	}
	if err := p.host.SignAsRelay(ctx, rem); err != nil {
		return err
	}
	if err := p.host.SaveEvent(ctx, rem); err != nil {
		return err
	}
	return p.host.Broadcast(ctx, rem)
}

func (p *nip29) EventVisible(ctx context.Context, rc *plugin.ReqContext, ev *nostr.Event) (bool, error) {
	if p.host.RelayPubkey() == "" || ev == nil {
		return true, nil
	}
	h := nostr.NIP29GroupHTag(ev)
	if h == "" {
		return true, nil
	}
	md, err := p.host.GetLatestGroupMetadata39000(ctx, h)
	if err != nil {
		log := p.host.Logger()
		logStoreErr(log, zerolog.WarnLevel, "GetLatestGroupMetadata39000", err, "nip29 visibility metadata fetch failed; withholding event", func(e *zerolog.Event) {
			if rc != nil && rc.Conn != nil {
				e.Str("conn_id", rc.Conn.ID())
			}
			e.Str("group_id", h).Str("event_id", ev.ID).Int("kind", ev.Kind).
				Bool("context_deadline_exceeded", errors.Is(err, context.DeadlineExceeded))
		})
		return false, nil
	}
	if md == nil || !nostr.NIP29MetadataIsPrivate(md) {
		return true, nil
	}
	if rc == nil || rc.Conn == nil {
		return false, nil
	}
	pks := rc.Conn.AuthedPubkeys()
	if len(pks) == 0 {
		return false, nil
	}
	for _, pk := range pks {
		member, err := p.host.IsGroupMember(ctx, h, pk)
		if err != nil {
			log := p.host.Logger()
			log.Debug().Err(err).Str("operation", "IsGroupMember").
				Str("group_id", h).Str("pubkey", pk).Str("event_id", ev.ID).Msg("nip29 visibility member check failed")
			continue
		}
		if member {
			return true, nil
		}
	}
	return false, nil
}

func reject(reason string) error {
	return plugin.Reject{Reason: reason}
}

func logStoreErr(log zerolog.Logger, level zerolog.Level, operation string, err error, msg string, fields func(*zerolog.Event)) {
	if err == nil {
		return
	}
	e := log.WithLevel(level).Err(err).Str("operation", operation)
	if fields != nil {
		fields(e)
	}
	e.Msg(msg)
}
