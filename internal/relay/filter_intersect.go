package relay

import (
	"fmt"
	"slices"

	"github.com/michmich112/congee/internal/nostr"
)

// FilterSubset reports whether narrowed is a subset of original (intersect-only REQ transforms).
func FilterSubset(original, narrowed nostr.Filter) bool {
	return filterSubset(original, narrowed)
}

// CloneFilter returns a deep copy of f.
func CloneFilter(f nostr.Filter) nostr.Filter {
	return cloneFilter(f)
}

func filterSubset(original, narrowed nostr.Filter) bool {
	if !idsSubset(original.IDs, narrowed.IDs) {
		return false
	}
	if !idsSubset(original.Authors, narrowed.Authors) {
		return false
	}
	if !kindsSubset(original.Kinds, narrowed.Kinds) {
		return false
	}
	if !int64GE(original.Since, narrowed.Since) {
		return false
	}
	if !int64LE(original.Until, narrowed.Until) {
		return false
	}
	if !limitLE(original.Limit, narrowed.Limit) {
		return false
	}
	if narrowed.HasSearch() {
		if !original.HasSearch() {
			return false
		}
		if original.SearchText() != narrowed.SearchText() {
			return false
		}
	}
	for key, want := range narrowed.Tag {
		orig, ok := original.Tag[key]
		if !ok {
			return false
		}
		if !idsSubset(orig, want) {
			return false
		}
	}
	return true
}

func idsSubset(superset, subset []string) bool {
	if len(subset) == 0 {
		return true
	}
	if len(superset) == 0 {
		return false
	}
	for _, v := range subset {
		if !slices.Contains(superset, v) {
			return false
		}
	}
	return true
}

func kindsSubset(superset, subset []int) bool {
	if len(subset) == 0 {
		return true
	}
	if len(superset) == 0 {
		return false
	}
	for _, v := range subset {
		if !slices.Contains(superset, v) {
			return false
		}
	}
	return true
}

func int64GE(original, narrowed *int64) bool {
	if narrowed == nil {
		return true
	}
	if original == nil {
		return false
	}
	return *original <= *narrowed
}

func int64LE(original, narrowed *int64) bool {
	if narrowed == nil {
		return true
	}
	if original == nil {
		return false
	}
	return *original >= *narrowed
}

func limitLE(original, narrowed *int) bool {
	if narrowed == nil {
		return true
	}
	if original == nil {
		return false
	}
	return *original >= *narrowed
}

// intersectFilter merges extra constraints into base when the result stays within base.
func intersectFilter(base, extra nostr.Filter) (nostr.Filter, error) {
	out := cloneFilter(base)
	if len(extra.IDs) > 0 {
		out.IDs = intersectStringSlice(base.IDs, extra.IDs)
		if len(out.IDs) == 0 && len(base.IDs) > 0 {
			return base, fmt.Errorf("relay: transform would empty ids constraint")
		}
	}
	if len(extra.Authors) > 0 {
		out.Authors = intersectStringSlice(base.Authors, extra.Authors)
		if len(out.Authors) == 0 && len(base.Authors) > 0 {
			return base, fmt.Errorf("relay: transform would empty authors constraint")
		}
	}
	if len(extra.Kinds) > 0 {
		out.Kinds = intersectIntSlice(base.Kinds, extra.Kinds)
		if len(out.Kinds) == 0 && len(base.Kinds) > 0 {
			return base, fmt.Errorf("relay: transform would empty kinds constraint")
		}
	}
	if extra.Since != nil {
		if base.Since == nil || *extra.Since > *base.Since {
			v := *extra.Since
			out.Since = &v
		}
	}
	if extra.Until != nil {
		if base.Until == nil || *extra.Until < *base.Until {
			v := *extra.Until
			out.Until = &v
		}
	}
	if extra.Limit != nil {
		if base.Limit == nil || *extra.Limit < *base.Limit {
			v := *extra.Limit
			out.Limit = &v
		}
	}
	for key, vals := range extra.Tag {
		if len(vals) == 0 {
			continue
		}
		if out.Tag == nil {
			out.Tag = make(map[string][]string)
		}
		if len(base.Tag[key]) > 0 {
			out.Tag[key] = intersectStringSlice(base.Tag[key], vals)
			if len(out.Tag[key]) == 0 {
				return base, fmt.Errorf("relay: transform would empty tag %q", key)
			}
		} else {
			out.Tag[key] = append([]string(nil), vals...)
		}
	}
	if !filterSubset(base, out) {
		return base, fmt.Errorf("relay: transform broadened filter")
	}
	return out, nil
}

func cloneFilter(f nostr.Filter) nostr.Filter {
	out := f
	if len(f.IDs) > 0 {
		out.IDs = append([]string(nil), f.IDs...)
	}
	if len(f.Authors) > 0 {
		out.Authors = append([]string(nil), f.Authors...)
	}
	if len(f.Kinds) > 0 {
		out.Kinds = append([]int(nil), f.Kinds...)
	}
	if f.Since != nil {
		v := *f.Since
		out.Since = &v
	}
	if f.Until != nil {
		v := *f.Until
		out.Until = &v
	}
	if f.Limit != nil {
		v := *f.Limit
		out.Limit = &v
	}
	if f.Search != nil {
		v := *f.Search
		out.Search = &v
	}
	if len(f.Tag) > 0 {
		out.Tag = make(map[string][]string, len(f.Tag))
		for k, v := range f.Tag {
			out.Tag[k] = append([]string(nil), v...)
		}
	}
	return out
}

func intersectStringSlice(a, b []string) []string {
	if len(a) == 0 {
		return append([]string(nil), b...)
	}
	var out []string
	for _, v := range b {
		if slices.Contains(a, v) {
			out = append(out, v)
		}
	}
	return out
}

func intersectIntSlice(a, b []int) []int {
	if len(a) == 0 {
		return append([]int(nil), b...)
	}
	var out []int
	for _, v := range b {
		if slices.Contains(a, v) {
			out = append(out, v)
		}
	}
	return out
}
