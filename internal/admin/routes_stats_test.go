package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func TestHandleStatsJSONKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "stats.db")
	st, err := sqlite.Open(ctx, p, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := config.DefaultConfig()
	h := handleStats(cfg, nil, st)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/stats", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{
		"open_connections", "relay_port", "admin_port", "relay_version",
		"subscriptions_open", "started_at_unix", "uptime_sec",
		"relay_counters", "recent_query_ms", "storage", "series",
	} {
		if _, ok := body[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	rc, _ := body["relay_counters"].(map[string]any)
	if rc == nil {
		t.Fatal("relay_counters not object")
	}
	for _, k := range []string{
		"events_stored_ok", "events_rejected", "req_total", "close_total",
		"rate_limit_messages", "rate_limit_bandwidth", "rate_limit_events", "rate_limit_reqs",
		"rate_limit_new_connections", "rate_limit_max_connections",
	} {
		if _, ok := rc[k]; !ok {
			t.Errorf("relay_counters missing %q", k)
		}
	}
	stg, _ := body["storage"].(map[string]any)
	if stg == nil {
		t.Fatal("storage not object")
	}
	for _, k := range []string{"bytes", "events", "tags", "audit"} {
		if _, ok := stg[k]; !ok {
			t.Errorf("storage missing %q", k)
		}
	}
	ser, _ := body["series"].(map[string]any)
	if ser == nil {
		t.Fatal("series not object")
	}
	if _, ok := ser["bucket_sec"]; !ok {
		t.Error("series missing bucket_sec")
	}
	if _, ok := ser["buckets"]; !ok {
		t.Error("series missing buckets")
	}
}

func TestMergeSeriesBucketsReplaceTail(t *testing.T) {
	t.Parallel()
	persisted := []storage.RelayMetricBucket{
		{BucketStartUnix: 100, EventsStored: 1, ReqCount: 2},
		{BucketStartUnix: 160, EventsStored: 3, ReqCount: 1},
	}
	partial := storage.RelayMetricBucket{
		BucketStartUnix: 160,
		EventsStored:    9,
		ReqCount:        7,
	}
	out := mergeSeriesBuckets(persisted, 160, partial, true)
	if len(out) != 2 {
		t.Fatalf("len=%d %#v", len(out), out)
	}
	if asInt64(t, out[1]["events_stored"]) != 9 {
		t.Fatalf("tail merge events_stored got %v", out[1]["events_stored"])
	}
}

func TestMergeSeriesBucketsAppendPartial(t *testing.T) {
	t.Parallel()
	persisted := []storage.RelayMetricBucket{
		{BucketStartUnix: 100, EventsStored: 1},
	}
	partial := storage.RelayMetricBucket{BucketStartUnix: 300, ReqCount: 4}
	out := mergeSeriesBuckets(persisted, 300, partial, true)
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if asInt64(t, out[1]["req_count"]) != 4 {
		t.Fatalf("append req_count %v", out[1]["req_count"])
	}
}

func asInt64(t *testing.T, v any) int64 {
	t.Helper()
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			t.Fatalf("json.Number: %v", err)
		}
		return i
	default:
		t.Fatalf("unexpected type %T value %v", v, v)
		return 0
	}
}
