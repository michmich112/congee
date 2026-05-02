package storage

// AdminStorageSnapshot is cheap-ish row counts plus optional on-disk size for the admin dashboard.
type AdminStorageSnapshot struct {
	Bytes  int64
	Events int64
	Tags   int64
	Audit  int64
}

// RelayMetricBucket is one persisted UTC-minute aggregate (wire-level relay telemetry).
type RelayMetricBucket struct {
	BucketStartUnix int64 `json:"bucket_start_unix"`
	EventsStored    int64 `json:"events_stored"`
	EventsRejected  int64 `json:"events_rejected"`
	ReqCount        int64 `json:"req_count"`
	CloseCount      int64 `json:"close_count"`
	QueryMsSum          int64 `json:"query_ms_sum"`
	QueryMsCount        int64 `json:"query_ms_count"`
	SubscriptionsOpen   int64 `json:"subscriptions_open"`
}

// RelayMetricBucketQuery loads recent buckets for charting (newest last or first — callers document order).
type RelayMetricBucketQuery struct {
	MinBucketStartUnix int64
	Limit              int
}
