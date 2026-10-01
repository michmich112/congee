package storage

import (
	"strings"

	"github.com/michmich112/congee/internal/nostr"
)

// ReadScopeSQL supplies the same visibility predicate to both SQL stores,
// full-text search, and COUNT. prefix is an internal column qualifier.
func ReadScopeSQL(scope *nostr.ReadScope, prefix string) (string, []interface{}) {
	if scope == nil {
		return "", nil
	}
	var clauses []string
	var args []interface{}
	placeholders := func(n int) string { return "?" + strings.Repeat(", ?", n-1) }
	if len(scope.ExcludedKinds) > 0 {
		clauses = append(clauses, prefix+"kind NOT IN ("+placeholders(len(scope.ExcludedKinds))+")")
		for _, kind := range scope.ExcludedKinds {
			args = append(args, kind)
		}
	}
	if len(scope.RecipientKinds) > 0 {
		public := prefix + "kind NOT IN (" + placeholders(len(scope.RecipientKinds)) + ")"
		for _, kind := range scope.RecipientKinds {
			args = append(args, kind)
		}
		if len(scope.RecipientPubkeys) == 0 {
			clauses = append(clauses, public)
		} else {
			// Every tag position is indexed, including malformed p tags. COUNT
			// therefore rejects duplicate/malformed recipients in legacy rows too.
			recipient := prefix + "id IN (SELECT event_id FROM event_tags WHERE name = 'p' GROUP BY event_id HAVING COUNT(*) = 1 AND MIN(value) IN (" + placeholders(len(scope.RecipientPubkeys)) + "))"
			for _, pubkey := range scope.RecipientPubkeys {
				args = append(args, pubkey)
			}
			clauses = append(clauses, "("+public+" OR "+recipient+")")
		}
	}
	return strings.Join(clauses, " AND "), args
}
