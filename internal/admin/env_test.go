package admin

import (
	"os"
	"testing"
)

func TestEnabled_CaseInsensitiveTrue(t *testing.T) {
	t.Setenv("ENABLE_ADMIN_UI", "TRUE")
	if !Enabled() {
		t.Fatal("expected TRUE to enable admin")
	}
}

func TestEnabled_Off(t *testing.T) {
	t.Setenv("ENABLE_ADMIN_UI", "0")
	if Enabled() {
		t.Fatal("expected 0 to disable")
	}
	_ = os.Unsetenv("ENABLE_ADMIN_UI")
	if Enabled() {
		t.Fatal("empty should disable")
	}
}
