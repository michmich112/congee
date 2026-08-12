package relay

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nip77"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

// NegConnAudit is one open NIP-77 session for admin connection audit.
type NegConnAudit struct {
	SubID            string `json:"sub_id"`
	OpenedUnix       int64  `json:"opened_unix"`
	FilterKinds      []int  `json:"filter_kinds,omitempty"`
	RecordCount      int    `json:"record_count"`
	Rounds           int    `json:"rounds"`
	LastActivityUnix int64  `json:"last_activity_unix"`
}

// RegisterNIP77 registers NEG-* handlers and starts the negentropy worker queue.
func RegisterNIP77(s *Server, _ storage.Store) {
	s.negQueue = newNegQueue(s)
	s.negQueue.start()
	s.negLoadSlots = make(chan struct{}, config.EffectiveNIP77MaxConcurrentLoads(s.cfg))
	for i := 0; i < cap(s.negLoadSlots); i++ {
		s.negLoadSlots <- struct{}{}
	}

	s.RegisterMessageHandler("NEG-OPEN", func(ctx context.Context, c *Conn, msg any) error {
		return handleNEGOpen(ctx, s, c, msg.(*nostr.NegOpenMessage))
	})
	s.RegisterMessageHandler("NEG-MSG", func(ctx context.Context, c *Conn, msg any) error {
		return handleNEGMsg(ctx, s, c, msg.(*nostr.NegMsgMessage))
	})
	s.RegisterMessageHandler("NEG-CLOSE", func(ctx context.Context, c *Conn, msg any) error {
		return handleNEGClose(ctx, s, c, msg.(*nostr.NegCloseMessage))
	})
}

func relayNIP77Enabled(cfg *config.Config) bool {
	return config.NIP77Enabled(cfg)
}

// RelayBusyForNeg reports whether inbound NEG-OPEN should be rejected for REQ backpressure.
func (s *Server) RelayBusyForNeg() bool {
	depth := config.EffectiveNIP77BackpressureReqQueueDepth(s.cfg)
	if depth <= 0 || s.readQueue == nil {
		return false
	}
	return s.readQueue.PendingDepth() >= depth
}

func (s *Server) sendNegErr(c *Conn, subID, reason string) error {
	if s.metrics != nil {
		s.metrics.IncNegErr()
	}
	audit.Enqueue(storage.AuditEntry{
		CreatedAt: time.Now().Unix(),
		Action:    audit.ActionNegErr,
		Detail:    fmt.Sprintf("conn_id=%s sub_id=%s reason=%s", c.ID, subID, audit.SanitizeAuditDetailFragment(reason)),
	})
	b, err := nostr.MarshalRelayNegErr(subID, reason)
	if err != nil {
		return err
	}
	return c.enqueue(b)
}

func (s *Server) sendNegBlocked(c *Conn, subID, reason string) error {
	if s.metrics != nil {
		s.metrics.IncNegBlocked()
	}
	audit.Enqueue(storage.AuditEntry{
		CreatedAt: time.Now().Unix(),
		Action:    audit.ActionNegBlocked,
		Detail:    fmt.Sprintf("conn_id=%s sub_id=%s reason=%s", c.ID, subID, audit.SanitizeAuditDetailFragment(reason)),
	})
	return s.sendNegErr(c, subID, reason)
}

func validateNegFilter(cfg *config.Config, c *Conn, f *nostr.Filter) error {
	if f == nil {
		return fmt.Errorf("blocked: missing filter")
	}
	if f.HasSearch() {
		return fmt.Errorf("blocked: search filters not supported")
	}
	if f.Limit != nil {
		return fmt.Errorf("blocked: limit filters not supported for NEG-OPEN")
	}
	if subscribeAuthRequired(cfg, []nostr.Filter{*f}) && !c.nip42HasAnyAuth() {
		return fmt.Errorf("auth-required: subscription requires authentication")
	}
	return nil
}

func handleNEGOpen(ctx context.Context, s *Server, c *Conn, msg *nostr.NegOpenMessage) error {
	log := relayLogger(c, ctx)
	subID := msg.SubID

	if !relayNIP77Enabled(s.cfg) {
		return s.sendNegBlocked(c, subID, "blocked: NIP-77 is not enabled")
	}
	if len(subID) > s.cfg.MaxSubscriptionIDLength {
		return s.sendNegBlocked(c, subID, fmt.Sprintf("blocked: %v", ErrSubscriptionIDTooLong))
	}
	if !c.limiter.AllowNegOpen() {
		if s.metrics != nil {
			s.metrics.IncNegBlocked()
		}
		return s.sendNegBlocked(c, subID, "blocked: rate limited")
	}
	if s.RelayBusyForNeg() {
		return s.sendNegBlocked(c, subID, "blocked: relay busy")
	}
	if int(s.negActiveSessions.Load()) >= config.EffectiveNIP77MaxConcurrentSessions(s.cfg) {
		return s.sendNegBlocked(c, subID, "blocked: too many sync sessions")
	}
	if err := validateNegFilter(s.cfg, c, &msg.Filter); err != nil {
		return s.sendNegBlocked(c, subID, err.Error())
	}

	if c.negSessions.close(subID) {
		s.negActiveSessions.Add(-1)
	}

	// Reserve a concurrent-session slot at enqueue time so the configured
	// limit is enforced synchronously rather than advisory (the async job
	// releases it if no session ends up being created).
	s.negActiveSessions.Add(1)

	job := &negOpenJob{ctx: ctx, c: c, msg: msg}
	if !s.negQueue.Enqueue(job) {
		s.negActiveSessions.Add(-1)
		return s.sendNegBlocked(c, subID, "blocked: sync queue full")
	}
	log.Debug().Str("sub_id", subID).Msg("nip77 neg-open enqueued")
	return nil
}

func (s *Server) runNegOpenJob(job *negOpenJob) {
	c := job.c
	msg := job.msg
	subID := msg.SubID
	log := c.log

	select {
	case <-s.negLoadSlots:
	default:
		s.negActiveSessions.Add(-1)
		_ = s.sendNegBlocked(c, subID, "blocked: sync load capacity reached")
		return
	}
	defer func() { s.negLoadSlots <- struct{}{} }()

	maxRec := config.EffectiveNIP77MaxRecordsPerQuery(s.cfg)
	if maxRec > 0 {
		n, err := s.store.CountEvents(job.ctx, []nostr.Filter{msg.Filter})
		if err != nil {
			s.negActiveSessions.Add(-1)
			log.Warn().Err(err).Str("sub_id", subID).Msg("nip77 count failed")
			_ = s.sendNegErr(c, subID, "error: count failed")
			return
		}
		if n > maxRec {
			s.negActiveSessions.Add(-1)
			reason := fmt.Sprintf("blocked: this query is too big (%d records, max %d)", n, maxRec)
			_ = s.sendNegBlocked(c, subID, reason)
			return
		}
	}

	t0 := time.Now()
	items, err := s.store.QueryEventSyncItems(job.ctx, msg.Filter)
	if err != nil {
		s.negActiveSessions.Add(-1)
		log.Warn().Err(err).Str("sub_id", subID).Msg("nip77 sync query failed")
		_ = s.sendNegErr(c, subID, "error: query failed")
		return
	}
	loadDur := time.Since(t0)
	if s.metrics != nil {
		s.metrics.RecordNegLoadLatency(loadDur)
		s.metrics.IncNegOpen()
	}

	vec := nip77.BuildVector(items)
	frameLimit := config.EffectiveNIP77FrameSizeLimit(s.cfg)
	neg := nip77.NewServerNegentropy(vec, frameLimit)

	t1 := time.Now()
	out, err := neg.Reconcile(msg.InitialHex)
	if err != nil {
		s.negActiveSessions.Add(-1)
		log.Warn().Err(err).Str("sub_id", subID).Msg("nip77 reconcile failed")
		_ = s.sendNegErr(c, subID, "error: "+err.Error())
		return
	}
	if s.metrics != nil {
		s.metrics.RecordNegReconcileLatency(time.Since(t1))
	}

	now := time.Now().Unix()
	sess := &negSession{
		subID:       subID,
		filter:      msg.Filter,
		filterKinds: slices.Clone(msg.Filter.Kinds),
		recordCount: len(items),
		openedUnix:  now,
		lastActUnix: now,
		neg:         neg,
	}
	s.scheduleNegIdle(c, sess)

	if out != "" {
		c.negSessions.set(subID, sess)
	} else {
		// No session was created (sync already complete): release the
		// concurrent-session slot reserved at enqueue time.
		s.negActiveSessions.Add(-1)
	}

	log.Info().
		Str("sub_id", subID).
		Ints("filter_kinds", sess.filterKinds).
		Int("record_count", len(items)).
		Int64("load_duration_ms", loadDur.Milliseconds()).
		Msg("nip77 neg-open complete")

	audit.Enqueue(storage.AuditEntry{
		CreatedAt: now,
		Action:    audit.ActionNegOpen,
		Detail:    fmt.Sprintf("conn_id=%s sub_id=%s record_count=%d filter_kinds=%s", c.ID, subID, len(items), negFilterKindsDetail(msg.Filter.Kinds)),
	})

	b, err := nostr.MarshalRelayNegMsg(subID, out)
	if err != nil {
		_ = s.sendNegErr(c, subID, "error: marshal failed")
		return
	}
	_ = c.enqueue(b)
}

func (s *Server) scheduleNegIdle(c *Conn, sess *negSession) {
	if sess.idleCancel != nil {
		sess.idleCancel()
	}
	timeout := time.Duration(config.EffectiveNIP77SessionIdleTimeout(s.cfg)) * time.Second
	if timeout <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(s.metricsCtx)
	sess.idleCancel = cancel
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(timeout):
			if _, ok := c.negSessions.remove(sess.subID); ok {
				s.negActiveSessions.Add(-1)
				c.log.Info().Str("sub_id", sess.subID).Msg("nip77 session idle closed")
				_ = s.sendNegErr(c, sess.subID, "closed: you took too long to respond!")
			}
		}
	}()
}

func handleNEGMsg(ctx context.Context, s *Server, c *Conn, msg *nostr.NegMsgMessage) error {
	log := relayLogger(c, ctx)
	if !relayNIP77Enabled(s.cfg) {
		return s.sendNegBlocked(c, msg.SubID, "blocked: NIP-77 is not enabled")
	}
	if !c.limiter.AllowNegMsg() {
		return s.sendNegBlocked(c, msg.SubID, "blocked: rate limited")
	}
	if s.metrics != nil {
		s.metrics.IncNegMsg()
	}

	sess, ok := c.negSessions.get(msg.SubID)
	if !ok {
		return s.sendNegErr(c, msg.SubID, "closed: unknown subscription")
	}
	sess.touchActivity()
	s.scheduleNegIdle(c, sess)

	t0 := time.Now()
	out, err := sess.neg.Reconcile(msg.MessageHex)
	if err != nil {
		log.Warn().Err(err).Str("sub_id", msg.SubID).Msg("nip77 neg-msg reconcile failed")
		c.negSessions.remove(msg.SubID)
		s.negActiveSessions.Add(-1)
		return s.sendNegErr(c, msg.SubID, "error: "+err.Error())
	}
	if s.metrics != nil {
		s.metrics.RecordNegReconcileLatency(time.Since(t0))
	}

	log.Debug().Str("sub_id", msg.SubID).Int("round", sess.rounds).Msg("nip77 neg-msg")

	if out == "" {
		if removed, _ := c.negSessions.remove(msg.SubID); removed != nil {
			s.negActiveSessions.Add(-1)
			audit.Enqueue(storage.AuditEntry{
				CreatedAt: time.Now().Unix(),
				Action:    audit.ActionNegComplete,
				Detail:    fmt.Sprintf("conn_id=%s sub_id=%s rounds=%d", c.ID, msg.SubID, removed.rounds),
			})
		}
	}

	b, err := nostr.MarshalRelayNegMsg(msg.SubID, out)
	if err != nil {
		return s.sendNegErr(c, msg.SubID, "error: marshal failed")
	}
	return c.enqueue(b)
}

func handleNEGClose(ctx context.Context, s *Server, c *Conn, msg *nostr.NegCloseMessage) error {
	_ = ctx
	if removed, _ := c.negSessions.remove(msg.SubID); removed != nil {
		c.server.negActiveSessions.Add(-1)
		audit.Enqueue(storage.AuditEntry{
			CreatedAt: time.Now().Unix(),
			Action:    audit.ActionNegComplete,
			Detail:    fmt.Sprintf("conn_id=%s sub_id=%s rounds=%d reason=neg-close", c.ID, msg.SubID, removed.rounds),
		})
	}
	return nil
}

func negFilterKindsDetail(kinds []int) string {
	if len(kinds) == 0 {
		return "any"
	}
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = fmt.Sprintf("%d", k)
	}
	return strings.Join(parts, ",")
}
