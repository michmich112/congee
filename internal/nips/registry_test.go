package nips

import "testing"

func TestKnownNIPsIncludesMandatory01And11(t *testing.T) {
	for _, n := range []int{1, 11} {
		m, ok := KnownNIPs[n]
		if !ok || !m.Mandatory || m.Title == "" {
			t.Fatalf("nip %d: %+v", n, m)
		}
		if !IsKnown(n) {
			t.Fatalf("IsKnown(%d)", n)
		}
	}
	if IsKnown(99999) {
		t.Fatal()
	}
}
