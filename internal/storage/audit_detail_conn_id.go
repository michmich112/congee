package storage

import (
	"regexp"
	"strings"
)

var auditConnIDHex = regexp.MustCompile(`^[0-9a-f]{8}$`)

// AuditDetailConnIDLikePattern returns a SQL LIKE pattern matching relay audit detail
// fragments "conn_id=<8-hex>" (see internal/relay/nip01.go), or ok=false when connID is invalid.
func AuditDetailConnIDLikePattern(connID string) (pattern string, ok bool) {
	connID = strings.ToLower(strings.TrimSpace(connID))
	if !auditConnIDHex.MatchString(connID) {
		return "", false
	}
	return "%conn_id=" + connID + "%", true
}
