package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage"
)

const auditConnDBTimeout = 5 * time.Second

// auditConnListResponse is GET /audit/connections JSON.
type auditConnListResponse struct {
	RetentionDays int                            `json:"retention_days"`
	Live          []relay.ConnAuditLiveSummary   `json:"live"`
	Recent        []relay.ConnAuditClosedSummary `json:"recent"`
	Closed        []auditConnClosedRow           `json:"closed"`
	// ClosedTotal is set only when closed rows are included (include_closed != 0).
	ClosedTotal *int64 `json:"closed_total,omitempty"`
}

type auditConnClosedRow struct {
	Ref              string          `json:"ref"`
	ID               int64           `json:"id"`
	ConnID           string          `json:"conn_id"`
	PeerIP           string          `json:"peer_ip"`
	RemoteAddr       string          `json:"remote_addr"`
	StartedUnix      int64           `json:"started_unix"`
	EndedUnix        int64           `json:"ended_unix"`
	TotalAuth        int64           `json:"total_auth"`
	TotalReq         int64           `json:"total_req"`
	TotalClientEvent int64           `json:"total_client_event"`
	Series           json.RawMessage `json:"series"`
}

// auditConnDetailResponse is GET /audit/connections/{ref} JSON (live or persisted session).
type auditConnDetailResponse struct {
	Kind                string               `json:"kind"`
	Ref                 string               `json:"ref"`
	ConnID              string               `json:"conn_id"`
	PeerIP              string               `json:"peer_ip"`
	RemoteAddr          string               `json:"remote_addr"`
	StartedUnix         int64                `json:"started_unix"`
	EndedUnix           *int64               `json:"ended_unix,omitempty"`
	Subscriptions       int                  `json:"subscriptions"`
	TotalAuth           int64                `json:"total_auth"`
	TotalReq            int64                `json:"total_req"`
	TotalClientEvent    int64                `json:"total_client_event"`
	Series              json.RawMessage      `json:"series"`
	SubscriptionDetails []relay.SubConnAudit `json:"subscription_details,omitempty"`
}

// HandleAuditConnectionsList serves GET /audit/connections (mounted under /api/ after StripPrefix).
func HandleAuditConnectionsList(cfg *config.Config, relaySrv *relay.Server, store storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		includeLive := r.URL.Query().Get("include_live") != "0"
		includeRecent := r.URL.Query().Get("include_recent") != "0"
		includeClosed := r.URL.Query().Get("include_closed") != "0"
		limit := atoiDefault(r.URL.Query().Get("limit"), 100, 1, 500)
		offset := atoiDefault(r.URL.Query().Get("offset"), 0, 0, 1_000_000)

		dbCtx, cancel := context.WithTimeout(r.Context(), auditConnDBTimeout)
		defer cancel()

		closedOut := make([]auditConnClosedRow, 0)
		var closedTotal *int64
		if includeClosed && store != nil {
			n, err := store.CountWSConnectionSessions(dbCtx)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			t := n
			closedTotal = &t
			closed, err := store.QueryWSConnectionSessions(dbCtx, storage.WSConnectionSessionQuery{
				Limit:  limit,
				Offset: offset,
			})
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			closedOut = make([]auditConnClosedRow, 0, len(closed))
			for i := range closed {
				c := closed[i]
				closedOut = append(closedOut, auditConnClosedRow{
					Ref:              "session:" + strconv.FormatInt(c.ID, 10),
					ID:               c.ID,
					ConnID:           c.ConnID,
					PeerIP:           c.PeerIP,
					RemoteAddr:       c.RemoteAddr,
					StartedUnix:      c.StartedUnix,
					EndedUnix:        c.EndedUnix,
					TotalAuth:        c.TotalAuth,
					TotalReq:         c.TotalReq,
					TotalClientEvent: c.TotalClientEvent,
					Series:           json.RawMessage(c.SeriesJSON),
				})
			}
		}

		var live []relay.ConnAuditLiveSummary
		if includeLive && relaySrv != nil {
			live = relaySrv.ConnAuditLiveSummaries()
		}

		var recent []relay.ConnAuditClosedSummary
		if includeRecent && relaySrv != nil {
			recent = relaySrv.ConnAuditRecentClosedSummaries()
		}
		if recent == nil {
			recent = []relay.ConnAuditClosedSummary{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auditConnListResponse{
			RetentionDays: cfg.Audit.RetentionDays,
			Live:          live,
			Recent:        recent,
			Closed:        closedOut,
			ClosedTotal:   closedTotal,
		})
	}
}

// HandleAuditConnectionsDetail serves GET /audit/connections/{ref}.
func HandleAuditConnectionsDetail(cfg *config.Config, relaySrv *relay.Server, store storage.Store) http.HandlerFunc {
	_ = cfg
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ref := strings.TrimSpace(r.PathValue("ref"))
		if ref == "" {
			http.Error(w, "missing ref", http.StatusBadRequest)
			return
		}
		kind, liveConnID, sessionID, ok := parseAuditConnRef(ref)
		if !ok {
			http.Error(w, "invalid ref", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch kind {
		case "recent":
			if relaySrv == nil {
				http.Error(w, "relay not available", http.StatusServiceUnavailable)
				return
			}
			d, ok := relaySrv.ConnAuditRecentClosedDetailByConnID(liveConnID)
			if !ok {
				http.NotFound(w, r)
				return
			}
			ended := d.EndedUnix
			_ = json.NewEncoder(w).Encode(auditConnDetailResponse{
				Kind:                "recent",
				Ref:                 ref,
				ConnID:              d.ConnID,
				PeerIP:              d.PeerIP,
				RemoteAddr:          d.RemoteAddr,
				StartedUnix:         d.StartedUnix,
				EndedUnix:           &ended,
				Subscriptions:       d.SubscriptionCount,
				TotalAuth:           d.TotalAuth,
				TotalReq:            d.TotalReq,
				TotalClientEvent:    d.TotalClientEvent,
				Series:              d.Series,
				SubscriptionDetails: d.SubscriptionDetails,
			})
			return
		case "live":
			if relaySrv == nil {
				http.Error(w, "relay not available", http.StatusServiceUnavailable)
				return
			}
			d, ok := relaySrv.ConnAuditLiveDetailByConnID(liveConnID)
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(auditConnDetailResponse{
				Kind:                "live",
				Ref:                 ref,
				ConnID:              d.ConnID,
				PeerIP:              d.PeerIP,
				RemoteAddr:          d.RemoteAddr,
				StartedUnix:         d.StartedUnix,
				Subscriptions:       d.SubscriptionCount,
				TotalAuth:           d.TotalAuth,
				TotalReq:            d.TotalReq,
				TotalClientEvent:    d.TotalClientEvent,
				Series:              d.Series,
				SubscriptionDetails: d.SubscriptionDetails,
			})
			return
		case "session":
			if store == nil {
				http.Error(w, "database not available", http.StatusServiceUnavailable)
				return
			}
			dbCtx, cancel := context.WithTimeout(r.Context(), auditConnDBTimeout)
			defer cancel()
			sess, err := store.GetWSConnectionSessionByID(dbCtx, sessionID)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			if sess == nil {
				http.NotFound(w, r)
				return
			}
			var subs []relay.SubConnAudit
			if len(sess.SubsJSON) > 0 {
				_ = json.Unmarshal(sess.SubsJSON, &subs)
			}
			ended := sess.EndedUnix
			_ = json.NewEncoder(w).Encode(auditConnDetailResponse{
				Kind:                "session",
				Ref:                 ref,
				ConnID:              sess.ConnID,
				PeerIP:              sess.PeerIP,
				RemoteAddr:          sess.RemoteAddr,
				StartedUnix:         sess.StartedUnix,
				EndedUnix:           &ended,
				Subscriptions:       len(subs),
				TotalAuth:           sess.TotalAuth,
				TotalReq:            sess.TotalReq,
				TotalClientEvent:    sess.TotalClientEvent,
				Series:              json.RawMessage(sess.SeriesJSON),
				SubscriptionDetails: subs,
			})
			return
		default:
			http.Error(w, "invalid ref", http.StatusBadRequest)
		}
	}
}

func parseAuditConnRef(ref string) (kind string, liveConnID string, sessionID int64, ok bool) {
	if strings.HasPrefix(ref, "recent:") {
		id := strings.TrimSpace(strings.TrimPrefix(ref, "recent:"))
		if id == "" {
			return "", "", 0, false
		}
		return "recent", id, 0, true
	}
	if strings.HasPrefix(ref, "live:") {
		id := strings.TrimSpace(strings.TrimPrefix(ref, "live:"))
		if id == "" {
			return "", "", 0, false
		}
		return "live", id, 0, true
	}
	if strings.HasPrefix(ref, "session:") {
		raw := strings.TrimSpace(strings.TrimPrefix(ref, "session:"))
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 {
			return "", "", 0, false
		}
		return "session", "", n, true
	}
	return "", "", 0, false
}

func atoiDefault(s string, def, min, max int) int {
	if strings.TrimSpace(s) == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
