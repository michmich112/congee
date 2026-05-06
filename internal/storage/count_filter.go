package storage

import (
	"strings"

	"github.com/michmich112/congee/internal/nostr"
)

// CountFilterSubQuery builds `SELECT id FROM events WHERE ...` with `?` placeholders and args
// for one nostr filter, for SQL COUNT aggregations. Semantics match applyFilterQuery with an
// empty table prefix (ids, authors, kinds, time range, tag map). When skip is true, omit this
// filter from the aggregate (e.g. filter carries NIP-50 search text only).
func CountFilterSubQuery(f *nostr.Filter) (sql string, args []interface{}, skip bool) {
	if f == nil {
		return "", nil, true
	}
	if f.HasSearch() {
		return "", nil, true
	}
	var sb strings.Builder
	sb.WriteString("SELECT id FROM events")
	first := true
	addWhere := func(cond string, a ...interface{}) {
		args = append(args, a...)
		if first {
			sb.WriteString(" WHERE ")
			first = false
		} else {
			sb.WriteString(" AND ")
		}
		sb.WriteString(cond)
	}
	appendIn := func(col string, vals []string) {
		if len(vals) == 0 {
			return
		}
		ph := strings.Repeat(", ?", len(vals)-1)
		if len(ph) > 0 {
			ph = "?" + ph
		} else {
			ph = "?"
		}
		ifaces := make([]interface{}, len(vals))
		for i, v := range vals {
			ifaces[i] = v
		}
		addWhere(col+" IN ("+ph+")", ifaces...)
	}
	if len(f.IDs) > 0 {
		appendIn("id", f.IDs)
	}
	if len(f.Authors) > 0 {
		appendIn("pubkey", f.Authors)
	}
	if len(f.Kinds) > 0 {
		parts := make([]string, len(f.Kinds))
		vals := make([]interface{}, len(f.Kinds))
		for i, k := range f.Kinds {
			parts[i] = "?"
			vals[i] = k
		}
		addWhere("kind IN ("+strings.Join(parts, ", ")+")", vals...)
	}
	if f.Since != nil {
		addWhere("created_at >= ?", *f.Since)
	}
	if f.Until != nil {
		addWhere("created_at <= ?", *f.Until)
	}
	for key, vals := range f.Tag {
		if len(vals) == 0 {
			if first {
				sb.WriteString(" WHERE ")
				first = false
			} else {
				sb.WriteString(" AND ")
			}
			sb.WriteString("FALSE")
			continue
		}
		name := key[1:]
		ph := strings.Repeat(", ?", len(vals)-1)
		if len(ph) > 0 {
			ph = "?" + ph
		} else {
			ph = "?"
		}
		tagArgs := make([]interface{}, 0, 1+len(vals))
		tagArgs = append(tagArgs, name)
		for _, v := range vals {
			tagArgs = append(tagArgs, v)
		}
		addWhere("id IN (SELECT event_id FROM event_tags WHERE name = ? AND value IN ("+ph+"))", tagArgs...)
	}
	return sb.String(), args, false
}
