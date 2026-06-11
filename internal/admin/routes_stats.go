package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/version"
)

const statsDBTimeout = 3 * time.Second

func handleStats(cfg *config.Config, relaySrv *relay.Server, store storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var oc int64
		var subs int
		var started int64
		var uptime int64
		relayCounters := zeroRelayCountersShape()
		recentLatency := []relay.RecentLatencySample{}
		var partialStart int64
		var partialBucket storage.RelayMetricBucket

		if relaySrv != nil {
			oc = relaySrv.OpenConnections()
			subs = relaySrv.Subscriptions().TotalSubscriptions()
			started = relaySrv.StartedAtUnix()
			if started > 0 {
				uptime = time.Now().Unix() - started
			}
			if m := relaySrv.Metrics(); m != nil {
				relayCounters = m.CountersJSON()
				if s := m.RecentLatencySamples(); s != nil {
					recentLatency = s
				}
				partialStart, partialBucket = m.PartialMinuteBucket()
				partialBucket.SubscriptionsOpen = int64(subs)
			}
		}

		dbCtx, cancel := context.WithTimeout(r.Context(), statsDBTimeout)
		defer cancel()

		snap := storage.AdminStorageSnapshot{}
		var persisted []storage.RelayMetricBucket
		if store != nil {
			snap, _ = store.AdminStorageSnapshot(dbCtx)

			bucketCtx, bucketCancel := context.WithTimeout(r.Context(), statsDBTimeout)
			since := time.Now().Add(-24 * time.Hour).Unix()
			since = (since / 60) * 60
			persisted, _ = store.QueryRelayMetricBuckets(bucketCtx, storage.RelayMetricBucketQuery{
				MinBucketStartUnix: since,
				Limit:              1440,
			})
			bucketCancel()
		}

		bucketsJSON := mergeSeriesBuckets(persisted, partialStart, partialBucket, started > 0)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"open_connections":   oc,
			"relay_port":         cfg.Relay.Port,
			"admin_port":         cfg.Admin.Port,
			"relay_version":      version.Version,
			"subscriptions_open": subs,
			"started_at_unix":    started,
			"uptime_sec":         uptime,
			"relay_counters":     relayCounters,
			"recent_query_latency": recentLatency,
			"storage": map[string]any{
				"bytes":      snap.Bytes,
				"meta_bytes": snap.MetaBytes,
				"events":     snap.Events,
				"tags":       snap.Tags,
				"audit":      snap.Audit,
			},
			"series": map[string]any{
				"bucket_sec": 60,
				"buckets":    bucketsJSON,
			},
		})
	}
}

func zeroRelayCountersShape() map[string]any {
	return map[string]any{
		"events_stored_ok":           int64(0),
		"events_rejected":            int64(0),
		"events_ephemeral_ok":        int64(0),
		"req_total":                  int64(0),
		"close_total":                int64(0),
		"rate_limit_messages":        int64(0),
		"rate_limit_bandwidth":       int64(0),
		"rate_limit_events":          int64(0),
		"rate_limit_reqs":            int64(0),
		"rate_limit_new_connections": int64(0),
		"rate_limit_max_connections": int64(0),
		"rate_limit_per_ip_open":     int64(0),
		"idle_disconnect_total":      int64(0),
	}
}

func mergeSeriesBuckets(persisted []storage.RelayMetricBucket, partialStart int64, partial storage.RelayMetricBucket, relayStarted bool) []map[string]any {
	out := make([]map[string]any, 0, len(persisted)+1)
	for i := range persisted {
		out = append(out, relayBucketToJSON(persisted[i]))
	}
	if !relayStarted || partialStart == 0 {
		return out
	}
	if len(out) > 0 {
		last := persisted[len(persisted)-1]
		if last.BucketStartUnix == partialStart {
			out[len(out)-1] = relayBucketToJSON(partial)
			return out
		}
	}
	out = append(out, relayBucketToJSON(partial))
	return out
}

func relayBucketToJSON(b storage.RelayMetricBucket) map[string]any {
	m := map[string]any{
		"bucket_start_unix":     b.BucketStartUnix,
		"events_stored":         b.EventsStored,
		"events_rejected":       b.EventsRejected,
		"req_count":             b.ReqCount,
		"close_count":           b.CloseCount,
		"query_ms_sum":          b.QueryMsSum,
		"query_ms_count":        b.QueryMsCount,
		"subscriptions_open":    b.SubscriptionsOpen,
	}
	if b.QueryMsCount > 0 {
		m["query_ms_avg"] = float64(b.QueryMsSum) / float64(b.QueryMsCount)
	} else {
		m["query_ms_avg"] = 0.0
	}
	return m
}
