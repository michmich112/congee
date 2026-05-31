package nostr

import "testing"

func TestParseAddressableCoordinate(t *testing.T) {
	kind, pk, d, ok := ParseAddressableCoordinate("30023:abc123:my-doc")
	if !ok || kind != 30023 || pk != "abc123" || d != "my-doc" {
		t.Fatalf("parse: kind=%d pk=%q d=%q ok=%v", kind, pk, d, ok)
	}

	kind, pk, d, ok = ParseAddressableCoordinate("30023:abc123:part:two")
	if !ok || d != "part:two" {
		t.Fatalf("d-tag with colon: d=%q ok=%v", d, ok)
	}

	for _, bad := range []string{"", "x", "1:2", "1:2:", ":2:3", "bad:2:3"} {
		if _, _, _, ok := ParseAddressableCoordinate(bad); ok {
			t.Fatalf("expected invalid coordinate %q", bad)
		}
	}
}
