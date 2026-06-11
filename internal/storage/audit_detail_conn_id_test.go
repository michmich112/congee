package storage

import "testing"

func TestAuditDetailConnIDLikePattern(t *testing.T) {
	pat, ok := AuditDetailConnIDLikePattern("Ab12CD34")
	if !ok || pat != "%conn_id=ab12cd34%" {
		t.Fatalf("got %q ok=%v", pat, ok)
	}
	if _, ok := AuditDetailConnIDLikePattern("not-hex"); ok {
		t.Fatal("expected invalid conn id")
	}
	if _, ok := AuditDetailConnIDLikePattern("abcd"); ok {
		t.Fatal("expected short conn id invalid")
	}
}
