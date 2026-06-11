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

const (
	auditConnDBTimeout        = 5 * time.Second
	auditConnDetailEntriesMax = 500
)

// auditConnListResponse is GET /audit/connections JSON.
type auditConnListResponse struct {
	RetentionDays int                          `json:"retention_days"`
	Live          []relay.ConnAuditLiveSummary `json:"live"`
	Closed        []auditConnClosedRow         `json:"closed"`
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
	// AuditEntries lists EVENT-related audit rows for this connection (newest first); not full events.
	AuditEntries []storage.AuditEntry `json:"audit_entries,omitempty"`
}

// HandleAuditConnectionsList serves GET /audit/connections (mounted under /api/ after StripPrefix).
func HandleAuditConnectionsList(cfg *config.Config, relaySrv *relay.Server, store storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		includeLive := r.URL.Query().Get("include_live") != "0"
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

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auditConnListResponse{
			RetentionDays: cfg.Audit.RetentionDays,
			Live:          live,
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
			dbCtx, cancel := context.WithTimeout(r.Context(), auditConnDBTimeout)
			defer cancel()
			auditEntries, _ := queryConnEventAuditEntries(dbCtx, store, d.ConnID, d.StartedUnix, 0)
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
				AuditEntries:        auditEntries,
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
			auditEntries, _ := queryConnEventAuditEntries(dbCtx, store, sess.ConnID, sess.StartedUnix, sess.EndedUnix)
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
				AuditEntries:        auditEntries,
			})
			return
		default:
			http.Error(w, "invalid ref", http.StatusBadRequest)
		}
	}
}

// queryConnEventAuditEntries returns newest-first audit rows for EVENT outcomes on one connection
// within [since, until] (until=0 means no upper bound). Rows without event_id= in detail are omitted.
func queryConnEventAuditEntries(ctx context.Context, store storage.Store, connID string, since, until int64) ([]storage.AuditEntry, error) {
	if store == nil || connID == "" {
		return nil, nil
	}
	rows, err := store.QueryAuditLog(ctx, storage.AuditQuery{
		ConnID: connID,
		Since:  since,
		Until:  until,
		Limit:  auditConnDetailEntriesMax,
	})
	if err != nil {
		return nil, err
	}
	out := make([]storage.AuditEntry, 0, len(rows))
	for i := range rows {
		if storage.ExtractAuditDetailEventID(rows[i].Detail) == "" {
			continue
		}
		out = append(out, rows[i])
	}
	return out, nil
}

func parseAuditConnRef(ref string) (kind string, liveConnID string, sessionID int64, ok bool) {
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
