package relay

import (
	"testing"

	"github.com/michmich112/congee/internal/nostr"
)

func TestFilterSubsetRejectsWideningAuthors(t *testing.T) {
	orig := nostr.Filter{Authors: []string{"aaaa"}}
	wide := nostr.Filter{Authors: []string{"aaaa", "bbbb"}}
	if FilterSubset(orig, wide) {
		t.Fatal("extra author must not be subset")
	}
}

func TestFilterSubsetAcceptsNarrowingAuthors(t *testing.T) {
	orig := nostr.Filter{Authors: []string{"aaaa", "bbbb"}}
	narrow := nostr.Filter{Authors: []string{"aaaa"}}
	if !FilterSubset(orig, narrow) {
		t.Fatal("single author must be subset of pair")
	}
}

func TestFilterSubsetRejectsWideningKinds(t *testing.T) {
	orig := nostr.Filter{Kinds: []int{1}}
	wide := nostr.Filter{Kinds: []int{1, 2}}
	if FilterSubset(orig, wide) {
		t.Fatal("extra kind must not be subset")
	}
}

func TestFilterSubsetAcceptsNarrowingSince(t *testing.T) {
	since := int64(100)
	later := int64(200)
	orig := nostr.Filter{Since: &since}
	narrow := nostr.Filter{Since: &later}
	if !FilterSubset(orig, narrow) {
		t.Fatal("later since is narrower")
	}
}

func TestIntersectFilterRejectsBroadening(t *testing.T) {
	base := nostr.Filter{Authors: []string{"aaaa"}}
	extra := nostr.Filter{Authors: []string{"bbbb"}}
	_, err := intersectFilter(base, extra)
	if err == nil {
		t.Fatal("expected error when intersect empties authors")
	}
}
