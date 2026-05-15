package audit

import "testing"

func TestSanitizeAuditDetailFragment(t *testing.T) {
	if got := SanitizeAuditDetailFragment("  hello\n\tworld  "); got != "hello world" {
		t.Fatalf("got %q", got)
	}
	if got := SanitizeAuditDetailFragment(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
	if got := SanitizeAuditDetailFragment("   "); got != "" {
		t.Fatalf("spaces: got %q", got)
	}
}
