package storage

import "encoding/json"

// DecodeTagFullJSON parses event_tags.full_json into one Nostr tag line ([]string).
//
// Normally full_json is a JSON array of strings. PostgreSQL inserts historically used
// bun's jsonb appender with a Go string field, which JSON-marshalled the payload again,
// storing a JSON string containing the array. This helper accepts both encodings.
func DecodeTagFullJSON(fullJSON string) ([]string, error) {
	var parts []string
	if err := json.Unmarshal([]byte(fullJSON), &parts); err == nil {
		return parts, nil
	}
	var wrapped string
	if err := json.Unmarshal([]byte(fullJSON), &wrapped); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(wrapped), &parts); err != nil {
		return nil, err
	}
	return parts, nil
}
