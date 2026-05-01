package storage

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DefaultAuditKindsScanLimit is how many newest audit rows ListDistinctAuditKinds scans by default.
const DefaultAuditKindsScanLimit = 50_000

// MaxAuditKindsScanLimit caps ?scan_limit= on the admin audit-kinds API and store scans.
const MaxAuditKindsScanLimit = 200_000

var auditDetailTrailingKind = regexp.MustCompile(` kind=(\d+)$`)

// ParseAuditDetailTrailingKind extracts the trailing NIP-01 post-hook kind from a relay audit detail line.
func ParseAuditDetailTrailingKind(detail string) (kind int, ok bool) {
	m := auditDetailTrailingKind.FindStringSubmatch(detail)
	if m == nil {
		return 0, false
	}
	k, err := strconv.Atoi(m[1])
	if err != nil || k < 0 {
		return 0, false
	}
	return k, true
}

// DedupeSortNonNegInts returns a sorted copy of in with duplicates removed; negative values are dropped.
func DedupeSortNonNegInts(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(in))
	for _, k := range in {
		if k < 0 {
			continue
		}
		seen[k] = struct{}{}
	}
	out := make([]int, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// AuditDetailKindSuffixMatchOr builds a SQL OR expression matching relay audit detail suffixes " kind=<n>"
// for any n in kinds (deduped). sqlite selects substr(detail, -n)=suffix; postgres uses right(detail, n)=suffix.
func AuditDetailKindSuffixMatchOr(sqlite bool, kinds []int) (sql string, args []interface{}) {
	kinds = DedupeSortNonNegInts(kinds)
	if len(kinds) == 0 {
		return "", nil
	}
	expr := "substr(detail, -?) = ?"
	if !sqlite {
		expr = "right(detail, ?) = ?"
	}
	parts := make([]string, len(kinds))
	args = make([]interface{}, 0, 2*len(kinds))
	for i, k := range kinds {
		suffix := fmt.Sprintf(" kind=%d", k)
		n := len(suffix)
		parts[i] = "(" + expr + ")"
		args = append(args, n, suffix)
	}
	return strings.Join(parts, " OR "), args
}
