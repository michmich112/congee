package nostr

import (
	"encoding/hex"
	"strings"
)

const (
	NIP29KindGroupMetadata = 39000
	NIP29KindPutUser     = 9000
	NIP29KindRemoveUser  = 9001
	NIP29KindCreateGroup = 9007
	NIP29KindJoinRequest = 9021
	NIP29KindLeaveReq    = 9022
)

// NIP29GroupHTag returns the first "h" tag value on ev, or empty.
func NIP29GroupHTag(ev *Event) string {
	if ev == nil {
		return ""
	}
	for _, t := range ev.Tags {
		if len(t) >= 2 && t[0] == "h" && t[1] != "" {
			return t[1]
		}
	}
	return ""
}

// NIP29PreviousTagValues returns all "previous" tag values (typically 8-hex id prefixes).
func NIP29PreviousTagValues(ev *Event) []string {
	if ev == nil {
		return nil
	}
	var out []string
	for _, t := range ev.Tags {
		if len(t) >= 2 && t[0] == "previous" && t[1] != "" {
			out = append(out, t[1])
		}
	}
	return out
}

// NIP29IsValidPreviousPrefix reports whether s is exactly 8 lowercase hex nibbles.
func NIP29IsValidPreviousPrefix(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) != 8 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// NIP29IsModerationKind reports kinds 9000–9020 (relay moderation actions per NIP-29).
func NIP29IsModerationKind(kind int) bool {
	return kind >= 9000 && kind <= 9020
}

// NIP29MetadataHasBareTag reports whether ev has a tag whose name matches (single-letter or word tags with optional empty value).
func NIP29MetadataHasBareTag(ev *Event, name string) bool {
	if ev == nil || name == "" {
		return false
	}
	for _, t := range ev.Tags {
		if len(t) == 0 {
			continue
		}
		if t[0] != name {
			continue
		}
		return true
	}
	return false
}

// NIP29MetadataIsPrivate reports whether kind-39000 metadata marks the group private (read).
func NIP29MetadataIsPrivate(md *Event) bool {
	return md != nil && md.Kind == NIP29KindGroupMetadata && NIP29MetadataHasBareTag(md, "private")
}

// NIP29MetadataIsRestricted reports whether kind-39000 metadata marks the group restricted (write).
func NIP29MetadataIsRestricted(md *Event) bool {
	return md != nil && md.Kind == NIP29KindGroupMetadata && NIP29MetadataHasBareTag(md, "restricted")
}
