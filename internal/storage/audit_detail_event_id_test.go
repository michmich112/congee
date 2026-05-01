package storage_test

import (
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/storage"
)

func TestExtractAuditDetailEventID(t *testing.T) {
	id := strings.Repeat("a", 64)
	d := "event_id=" + id + " conn_id=x stored=true kind=1"
	got := storage.ExtractAuditDetailEventID(d)
	if got != id {
		t.Fatalf("got %q want %q", got, id)
	}
	if storage.ExtractAuditDetailEventID("no prefix") != "" {
		t.Fatal("expected empty")
	}
}
