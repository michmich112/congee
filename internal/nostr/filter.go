package nostr

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Filter is a NIP-01 subscription / query filter.
type Filter struct {
	IDs     []string            `json:"ids,omitempty"`
	Authors []string            `json:"authors,omitempty"`
	Kinds   []int               `json:"kinds,omitempty"`
	Since   *int64              `json:"since,omitempty"`
	Until   *int64              `json:"until,omitempty"`
	Limit   *int                `json:"limit,omitempty"`
	// Tag filters: "#e", "#p", or "#" + single letter (a-zA-Z).
	Tag map[string][]string `json:"-"`
}

// UnmarshalJSON decodes a filter object, including "#x" tag keys.
func (f *Filter) UnmarshalJSON(data []byte) error {
	type alias Filter
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = Filter{Tag: make(map[string][]string)}
	for k, v := range raw {
		switch k {
		case "ids":
			if err := json.Unmarshal(v, &f.IDs); err != nil {
				return fmt.Errorf("nostr: filter ids: %w", err)
			}
		case "authors":
			if err := json.Unmarshal(v, &f.Authors); err != nil {
				return fmt.Errorf("nostr: filter authors: %w", err)
			}
		case "kinds":
			if err := json.Unmarshal(v, &f.Kinds); err != nil {
				return fmt.Errorf("nostr: filter kinds: %w", err)
			}
		case "since":
			if err := json.Unmarshal(v, &f.Since); err != nil {
				return fmt.Errorf("nostr: filter since: %w", err)
			}
		case "until":
			if err := json.Unmarshal(v, &f.Until); err != nil {
				return fmt.Errorf("nostr: filter until: %w", err)
			}
		case "limit":
			if err := json.Unmarshal(v, &f.Limit); err != nil {
				return fmt.Errorf("nostr: filter limit: %w", err)
			}
		default:
			// NIP-01: "#<single-letter (a-zA-Z)>"
			if len(k) == 2 && k[0] == '#' && unicode.IsLetter(rune(k[1])) {
				var vals []string
				if err := json.Unmarshal(v, &vals); err != nil {
					return fmt.Errorf("nostr: filter %q: %w", k, err)
				}
				f.Tag[k] = vals
			}
		}
	}
	return nil
}

// MarshalJSON encodes the filter, including tag maps.
func (f *Filter) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)
	if len(f.IDs) > 0 {
		m["ids"] = f.IDs
	}
	if len(f.Authors) > 0 {
		m["authors"] = f.Authors
	}
	if len(f.Kinds) > 0 {
		m["kinds"] = f.Kinds
	}
	if f.Since != nil {
		m["since"] = *f.Since
	}
	if f.Until != nil {
		m["until"] = *f.Until
	}
	if f.Limit != nil {
		m["limit"] = *f.Limit
	}
	for k, v := range f.Tag {
		m[k] = v
	}
	return json.Marshal(m)
}

// Matches reports whether e satisfies all set conditions of the filter.
// Limit is not applied (it governs query result size, not per-event matching).
func (f *Filter) Matches(e *Event) bool {
	if f == nil || e == nil {
		return false
	}
	if len(f.IDs) > 0 {
		ok := false
		for _, id := range f.IDs {
			if id == e.ID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Authors) > 0 {
		ok := false
		for _, a := range f.Authors {
			if a == e.PubKey {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Kinds) > 0 {
		ok := false
		for _, k := range f.Kinds {
			if k == e.Kind {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.Since != nil && e.CreatedAt < *f.Since {
		return false
	}
	if f.Until != nil && e.CreatedAt > *f.Until {
		return false
	}
	for key, wantVals := range f.Tag {
		if len(wantVals) == 0 {
			return false
		}
		tagName := strings.TrimPrefix(key, "#")
		if !eventTagMatches(e.Tags, tagName, wantVals) {
			return false
		}
	}
	return true
}

// eventTagMatches is true if some tag with given name has first value in wantVals.
func eventTagMatches(tags [][]string, name string, wantVals []string) bool {
	want := make(map[string]struct{}, len(wantVals))
	for _, v := range wantVals {
		want[v] = struct{}{}
	}
	for _, t := range tags {
		if len(t) == 0 {
			continue
		}
		if t[0] != name {
			continue
		}
		if len(t) < 2 {
			continue
		}
		if _, ok := want[t[1]]; ok {
			return true
		}
	}
	return false
}
