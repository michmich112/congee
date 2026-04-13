package nips

import "testing"

func TestIsImplemented(t *testing.T) {
	for _, n := range []struct {
		n    int
		want bool
	}{
		{1, true},
		{2, true},
		{11, true},
		{42, true},
		{50, true},
		{99, false},
	} {
		if got := IsImplemented(n.n); got != n.want {
			t.Errorf("IsImplemented(%d) = %v, want %v", n.n, got, n.want)
		}
	}
}
