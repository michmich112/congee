package storage

import "strings"

// ExtractAuditDetailEventID returns the 64-hex event id from an audit detail string
// that uses the relay's "event_id=<hex> ..." prefix (see internal/relay/nip01.go), or "".
func ExtractAuditDetailEventID(detail string) string {
	const p = "event_id="
	i := strings.Index(detail, p)
	if i < 0 {
		return ""
	}
	j := i + len(p)
	if j+64 > len(detail) {
		return ""
	}
	slice := detail[j : j+64]
	for _, c := range slice {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return ""
	}
	return strings.ToLower(slice)
}
