package nips

import "testing"

func TestKnownNIPsIncludesMandatory01(t *testing.T) {
	m, ok := KnownNIPs[1]
	if !ok || !m.Mandatory || m.Title == "" {
		t.Fatalf("%+v", m)
	}
	if !IsKnown(1) || IsKnown(99999) {
		t.Fatal()
	}
}
