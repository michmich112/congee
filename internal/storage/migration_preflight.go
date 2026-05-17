package storage

// Migration target preflight statuses for admin /api/migration/target-preflight.
const (
	MigrationPreflightEmpty      = "empty"
	MigrationPreflightCurrent    = "current"
	MigrationPreflightBehind     = "behind"
	MigrationPreflightAhead      = "ahead"
	MigrationPreflightUnreadable = "unreadable"
)

// MigrationTargetPreflight is the JSON body for migration target inspection (read-only).
type MigrationTargetPreflight struct {
	Status          string `json:"status"`
	ExpectedVersion int    `json:"expected_version"`
	ReportedVersion *int   `json:"reported_version,omitempty"`
	HasEventsTable  bool   `json:"has_events_table"`
	HasVersionTable bool   `json:"has_version_table"`
	Detail          string `json:"detail"`
}
