package audit

import "strings"

// Audit action names for NIP-01 EVENT outcomes (persisted in audit_log.action).
const (
	ActionEventRejected  = "event_rejected"
	ActionEventStored    = "event_stored"
	ActionEventEphemeral = "event_ephemeral"

	ActionNegOpen                 = "neg_open"
	ActionNegComplete             = "neg_complete"
	ActionNegBlocked              = "neg_blocked"
	ActionNegErr                  = "neg_err"
	ActionNegUpstreamSyncComplete = "neg_upstream_sync_complete"
	ActionNegUpstreamSyncFailed   = "neg_upstream_sync_failed"
)

// SanitizeAuditDetailFragment collapses whitespace so a reason (or fragment) fits one logical line
// in audit detail and cannot spoof a trailing " kind=<n>" suffix.
func SanitizeAuditDetailFragment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}
