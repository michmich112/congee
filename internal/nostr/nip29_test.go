package nostr

import (
	"strings"
	"testing"
)

func TestNIP29GroupHTag(t *testing.T) {
	ev := &Event{Tags: [][]string{{"h", "abc"}, {"p", "x"}}}
	if g := NIP29GroupHTag(ev); g != "abc" {
		t.Fatalf("got %q", g)
	}
	if NIP29GroupHTag(&Event{Tags: [][]string{{"h", ""}}}) != "" {
		t.Fatal("empty h value should be ignored")
	}
}

func TestNIP29PreviousTagValues(t *testing.T) {
	ev := &Event{Tags: [][]string{{"previous", "a1b2c3d4"}, {"previous", "ffffffff"}}}
	v := NIP29PreviousTagValues(ev)
	if len(v) != 2 || v[0] != "a1b2c3d4" || v[1] != "ffffffff" {
		t.Fatalf("got %#v", v)
	}
}

func TestNIP29IsValidPreviousPrefix(t *testing.T) {
	if !NIP29IsValidPreviousPrefix("a1B2c3D4") {
		t.Fatal("expected valid")
	}
	if NIP29IsValidPreviousPrefix("gggggggg") {
		t.Fatal("invalid hex")
	}
	if NIP29IsValidPreviousPrefix("abcd") {
		t.Fatal("too short")
	}
}

func TestNIP29IsModerationKind(t *testing.T) {
	if !NIP29IsModerationKind(9000) || !NIP29IsModerationKind(9020) {
		t.Fatal("9000-9020")
	}
	if NIP29IsModerationKind(9021) || NIP29IsModerationKind(8999) {
		t.Fatal("boundary")
	}
}

func TestNIP29MetadataTags(t *testing.T) {
	md := &Event{Kind: NIP29KindGroupMetadata, Tags: [][]string{{"d", "g"}, {"private"}, {"restricted"}}}
	if !NIP29MetadataIsPrivate(md) || !NIP29MetadataIsRestricted(md) {
		t.Fatal("expected private+restricted")
	}
	if NIP29MetadataIsPrivate(&Event{Kind: 1}) {
		t.Fatal("wrong kind")
	}
}

func TestNIP29MetadataIsClosed(t *testing.T) {
	md := &Event{Kind: NIP29KindGroupMetadata, Tags: [][]string{{"d", "g"}, {"closed"}}}
	if !NIP29MetadataIsClosed(md) {
		t.Fatal("expected closed")
	}
	if NIP29MetadataIsClosed(&Event{Kind: NIP29KindGroupMetadata, Tags: [][]string{{"d", "g"}}}) {
		t.Fatal("open group")
	}
	if NIP29MetadataIsClosed(&Event{Kind: 1}) {
		t.Fatal("wrong kind")
	}
}

func TestNIP29Admins39001ContainsPubkey(t *testing.T) {
	pk := "Ab" + strings.Repeat("c", 62)
	admins := &Event{
		Kind: NIP29KindGroupAdmins,
		Tags: [][]string{{"d", "g"}, {"p", strings.ToLower(pk), "admin"}, {"p", "other"}},
	}
	if !NIP29Admins39001ContainsPubkey(admins, strings.ToUpper(pk[:2])+pk[2:]) {
		t.Fatal("EqualFold match")
	}
	if NIP29Admins39001ContainsPubkey(admins, strings.Repeat("f", 64)) {
		t.Fatal("non-member")
	}
	if NIP29Admins39001ContainsPubkey(nil, pk) || NIP29Admins39001ContainsPubkey(&Event{Kind: 1}, pk) {
		t.Fatal("invalid admins event")
	}
}
