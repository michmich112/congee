package db

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

type snapshotter interface {
	AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error)
}

// compositeStore implements storage.Store by delegating events and metadata to separate backends.
type compositeStore struct {
	events storage.EventStore
	meta   storage.MetaStore
	evSnap snapshotter
	metaSnap snapshotter
}

var _ storage.Store = (*compositeStore)(nil)

// NewCompositeForTest builds a storage.Store from separate event and meta backends (tests).
func NewCompositeForTest(events storage.EventStore, meta storage.MetaStore) storage.Store {
	var evSnap, metaSnap snapshotter
	if s, ok := events.(snapshotter); ok {
		evSnap = s
	}
	if s, ok := meta.(snapshotter); ok {
		metaSnap = s
	}
	return newCompositeStore(events, meta, evSnap, metaSnap)
}

func newCompositeStore(events storage.EventStore, meta storage.MetaStore, evSnap, metaSnap snapshotter) *compositeStore {
	return &compositeStore{
		events:   events,
		meta:     meta,
		evSnap:   evSnap,
		metaSnap: metaSnap,
	}
}

func (c *compositeStore) SaveEvent(ctx context.Context, ev *nostr.Event) error {
	return c.events.SaveEvent(ctx, ev)
}

func (c *compositeStore) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	return c.events.QueryEvents(ctx, filters)
}

func (c *compositeStore) DeleteEvent(ctx context.Context, id string) error {
	return c.events.DeleteEvent(ctx, id)
}

func (c *compositeStore) CountEvents(ctx context.Context, filters []nostr.Filter) (int, error) {
	return c.events.CountEvents(ctx, filters)
}

func (c *compositeStore) HasEventID(ctx context.Context, id string) (bool, error) {
	return c.events.HasEventID(ctx, id)
}

func (c *compositeStore) SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error) {
	return c.events.SearchEvents(ctx, searchQuery, constraints)
}

func (c *compositeStore) EventIDPrefixExists(ctx context.Context, prefix string, groupID string, requireSameH bool) (bool, error) {
	return c.events.EventIDPrefixExists(ctx, prefix, groupID, requireSameH)
}

func (c *compositeStore) GetLatestGroupMetadata39000(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error) {
	return c.events.GetLatestGroupMetadata39000(ctx, relayPubkey, groupID)
}

func (c *compositeStore) GetLatestGroupAdmins39001(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error) {
	return c.events.GetLatestGroupAdmins39001(ctx, relayPubkey, groupID)
}

func (c *compositeStore) IsGroupMember(ctx context.Context, relayPubkey, groupID, memberPubkey string) (bool, error) {
	return c.events.IsGroupMember(ctx, relayPubkey, groupID, memberPubkey)
}

func (c *compositeStore) SaveAuditEntry(ctx context.Context, e storage.AuditEntry) error {
	return c.meta.SaveAuditEntry(ctx, e)
}

func (c *compositeStore) HasAuditDuplicate(ctx context.Context, e storage.AuditEntry) (bool, error) {
	return c.meta.HasAuditDuplicate(ctx, e)
}

func (c *compositeStore) QueryAuditLog(ctx context.Context, q storage.AuditQuery) ([]storage.AuditEntry, error) {
	return c.meta.QueryAuditLog(ctx, q)
}

func (c *compositeStore) CountAuditLog(ctx context.Context, q storage.AuditQuery) (int64, error) {
	return c.meta.CountAuditLog(ctx, q)
}

func (c *compositeStore) ListDistinctAuditKinds(ctx context.Context, scanLimit int) ([]int, error) {
	return c.meta.ListDistinctAuditKinds(ctx, scanLimit)
}

func (c *compositeStore) PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error) {
	return c.meta.PurgeAuditLog(ctx, olderThanUnix)
}

func (c *compositeStore) SaveWSConnectionSession(ctx context.Context, s storage.WSConnectionSession) (int64, error) {
	return c.meta.SaveWSConnectionSession(ctx, s)
}

func (c *compositeStore) QueryWSConnectionSessions(ctx context.Context, q storage.WSConnectionSessionQuery) ([]storage.WSConnectionSession, error) {
	return c.meta.QueryWSConnectionSessions(ctx, q)
}

func (c *compositeStore) CountWSConnectionSessions(ctx context.Context) (int64, error) {
	return c.meta.CountWSConnectionSessions(ctx)
}

func (c *compositeStore) GetWSConnectionSessionByID(ctx context.Context, id int64) (*storage.WSConnectionSession, error) {
	return c.meta.GetWSConnectionSessionByID(ctx, id)
}

func (c *compositeStore) PurgeWSConnectionSessionsBefore(ctx context.Context, olderThanUnix int64) (int64, error) {
	return c.meta.PurgeWSConnectionSessionsBefore(ctx, olderThanUnix)
}

func (c *compositeStore) SaveConfigChange(ctx context.Context, ch storage.ConfigChange) error {
	return c.meta.SaveConfigChange(ctx, ch)
}

func (c *compositeStore) QueryConfigChangelog(ctx context.Context, limit int) ([]storage.ConfigChange, error) {
	return c.meta.QueryConfigChangelog(ctx, limit)
}

func (c *compositeStore) UpsertRelayMetricBucket(ctx context.Context, b storage.RelayMetricBucket) error {
	return c.meta.UpsertRelayMetricBucket(ctx, b)
}

func (c *compositeStore) QueryRelayMetricBuckets(ctx context.Context, q storage.RelayMetricBucketQuery) ([]storage.RelayMetricBucket, error) {
	return c.meta.QueryRelayMetricBuckets(ctx, q)
}

func (c *compositeStore) PurgeRelayMetricBucketsBefore(ctx context.Context, cutoffStartUnixExclusive int64) (int64, error) {
	return c.meta.PurgeRelayMetricBucketsBefore(ctx, cutoffStartUnixExclusive)
}

func (c *compositeStore) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	if c.evSnap != nil {
		ev, err := c.evSnap.AdminStorageSnapshot(ctx)
		if err != nil {
			return storage.AdminStorageSnapshot{}, err
		}
		out.Bytes = ev.Bytes
		out.Events = ev.Events
		out.Tags = ev.Tags
	}
	if c.metaSnap != nil {
		meta, err := c.metaSnap.AdminStorageSnapshot(ctx)
		if err != nil {
			return storage.AdminStorageSnapshot{}, err
		}
		out.MetaBytes = meta.MetaBytes
		out.Audit = meta.Audit
	}
	return out, nil
}
