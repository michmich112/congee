package storage

import "encoding/json"

// WSConnectionSessionToRow maps a domain session to a Bun row (JSON columns as string).
func WSConnectionSessionToRow(e WSConnectionSession) WSConnectionSessionRow {
	ser := string(e.SeriesJSON)
	if ser == "" {
		ser = "[]"
	}
	sub := string(e.SubsJSON)
	if sub == "" {
		sub = "[]"
	}
	return WSConnectionSessionRow{
		ConnID:           e.ConnID,
		PeerIP:           e.PeerIP,
		RemoteAddr:       e.RemoteAddr,
		StartedUnix:      e.StartedUnix,
		EndedUnix:        e.EndedUnix,
		TotalReq:         e.TotalReq,
		TotalClientEvent: e.TotalClientEvent,
		SeriesJSON:       ser,
		SubsJSON:         sub,
	}
}

// WSConnectionSessionFromRow maps a Bun row to API JSON-friendly structs.
func WSConnectionSessionFromRow(r WSConnectionSessionRow) WSConnectionSession {
	return WSConnectionSession{
		ID:               r.ID,
		ConnID:           r.ConnID,
		PeerIP:           r.PeerIP,
		RemoteAddr:       r.RemoteAddr,
		StartedUnix:      r.StartedUnix,
		EndedUnix:        r.EndedUnix,
		TotalReq:         r.TotalReq,
		TotalClientEvent: r.TotalClientEvent,
		SeriesJSON:       json.RawMessage(r.SeriesJSON),
		SubsJSON:         json.RawMessage(r.SubsJSON),
	}
}
