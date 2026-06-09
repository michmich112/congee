package relay

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// sameSlice reports whether two slices are the same slice value (both nil or same underlying array).
func sameSlice(a, b []nostr.Filter) bool {
	if len(a) == 0 || len(b) == 0 {
		return (a == nil) == (b == nil)
	}
	return unsafe.Pointer(&a[0]) == unsafe.Pointer(&b[0])
}

func TestQueryInitialREQEvents_SearchORWithKinds(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	pk := strings.Repeat("b", 64)
	sig := strings.Repeat("s", 128)
	ev1 := &nostr.Event{
		ID: strings.Repeat("1", 64), PubKey: pk, CreatedAt: 2, Kind: 1,
		Tags: nil, Content: "alpha beta gamma", Sig: sig,
	}
	ev2 := &nostr.Event{
		ID: strings.Repeat("2", 64), PubKey: pk, CreatedAt: 1, Kind: 7,
		Tags: nil, Content: "alpha ignored", Sig: sig,
	}
	for _, e := range []*nostr.Event{ev1, ev2} {
		if err := st.SaveEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	q := "alpha"
	fSearch := nostr.Filter{Search: &q, Kinds: []int{1}}
	fKind := nostr.Filter{Kinds: []int{7}}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{fSearch, fKind}, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 events (search+kind OR), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_SearchDisabledSkipsSearchBranch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q2.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	q := "nope"
	f := nostr.Filter{Search: &q}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty when search branch skipped, got %d", len(out))
	}
}

func createTestEvents(t *testing.T, st storage.EventStore, ctx context.Context, n int) {
	t.Helper()
	pk := strings.Repeat("b", 64)
	sig := strings.Repeat("s", 128)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%064d", i)
		ev := &nostr.Event{
			ID:        id,
			PubKey:    pk,
			CreatedAt: int64(n - i),
			Kind:      1,
			Tags:      nil,
			Content:   "test event",
			Sig:       sig,
		}
		if err := st.SaveEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplyDefaultQueryLimit_NoMutationWhenPositive(t *testing.T) {
	f1 := nostr.Filter{Kinds: []int{1}}
	f2 := nostr.Filter{Kinds: []int{7}}
	orig := []nostr.Filter{f1, f2}

	result := applyDefaultQueryLimit(orig, 500)

	if sameSlice(result, orig) {
		t.Fatal("expected a new slice when default is applied, got the same slice")
	}
	if orig[0].Limit != nil || orig[1].Limit != nil {
		t.Fatal("original filters must not be mutated")
	}
	if result[0].Limit == nil || *result[0].Limit != 500 {
		t.Fatalf("result[0].Limit: want 500, got %v", result[0].Limit)
	}
	if result[1].Limit == nil || *result[1].Limit != 500 {
		t.Fatalf("result[1].Limit: want 500, got %v", result[1].Limit)
	}
}

func TestApplyDefaultQueryLimit_NoMutationWhenZeroDefault(t *testing.T) {
	orig := []nostr.Filter{{Kinds: []int{1}}}
	result := applyDefaultQueryLimit(orig, 0)
	if !sameSlice(result, orig) {
		t.Fatal("zero default must return the same slice")
	}
	if orig[0].Limit != nil {
		t.Fatal("limit must remain nil when config default is 0")
	}
}

func TestApplyDefaultQueryLimit_NoMutationWhenNegative(t *testing.T) {
	orig := []nostr.Filter{{Kinds: []int{1}}}
	result := applyDefaultQueryLimit(orig, -1)
	if !sameSlice(result, orig) {
		t.Fatal("negative default must return the same slice")
	}
}

func TestApplyDefaultQueryLimit_NoCopyWhenAllExplicit(t *testing.T) {
	lim := 99
	orig := []nostr.Filter{{Kinds: []int{1}, Limit: &lim}}
	result := applyDefaultQueryLimit(orig, 500)
	if !sameSlice(result, orig) {
		t.Fatal("when all filters have explicit limits, must return original slice")
	}
}

func TestApplyDefaultQueryLimit_MixedExplicitAndNil(t *testing.T) {
	lim := 2
	orig := []nostr.Filter{
		{Kinds: []int{1}, Limit: &lim},
		{Kinds: []int{7}},
	}
	defaultLim := 500
	result := applyDefaultQueryLimit(orig, defaultLim)

	if sameSlice(result, orig) {
		t.Fatal("expected a new slice when some filters need the default")
	}
	if orig[1].Limit != nil {
		t.Fatal("original filter[1] must not be mutated")
	}
	if *result[0].Limit != 2 {
		t.Fatalf("filter[0] explicit limit should be preserved, got %d", *result[0].Limit)
	}
	if *result[1].Limit != 500 {
		t.Fatalf("filter[1] should get default limit 500, got %d", *result[1].Limit)
	}
}

func TestApplyDefaultQueryLimit_EmptyFilters(t *testing.T) {
	result := applyDefaultQueryLimit([]nostr.Filter{}, 500)
	if result == nil {
		t.Fatal("got nil for empty input, expected empty slice")
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got len %d", len(result))
	}
}

func TestApplyDefaultQueryLimit_NilFilters(t *testing.T) {
	result := applyDefaultQueryLimit(nil, 500)
	if result == nil {
		return
	}
	if len(result) != 0 {
		t.Fatalf("expected nil/empty for nil input, got len %d", len(result))
	}
}

func TestApplyDefaultQueryLimit_LeavesNegativeClientLimitUnchanged(t *testing.T) {
	neg := -5
	orig := []nostr.Filter{{Kinds: []int{1}, Limit: &neg}}
	result := applyDefaultQueryLimit(orig, 100)

	if !sameSlice(result, orig) {
		t.Fatal("expected same slice when no nil limits need default")
	}
	if result[0].Limit == nil || *result[0].Limit != -5 {
		t.Fatalf("want client limit -5 preserved, got %v", result[0].Limit)
	}
}

func TestApplyDefaultQueryLimit_LeavesZeroClientLimitUnchanged(t *testing.T) {
	zero := 0
	orig := []nostr.Filter{{Kinds: []int{1}, Limit: &zero}}
	result := applyDefaultQueryLimit(orig, 100)

	if !sameSlice(result, orig) {
		t.Fatal("expected same slice when no nil limits need default")
	}
	if result[0].Limit == nil || *result[0].Limit != 0 {
		t.Fatalf("want client limit 0 preserved, got %v", result[0].Limit)
	}
}

func TestApplyDefaultQueryLimit_ZeroConfigDefaultWithNegativeClientLimit(t *testing.T) {
	neg := -3
	orig := []nostr.Filter{{Kinds: []int{1}, Limit: &neg}}
	result := applyDefaultQueryLimit(orig, 0)

	if !sameSlice(result, orig) {
		t.Fatal("zero config default must return original slice even with negative client limit")
	}
}

func TestQueryInitialREQEvents_DefaultLimitApplies(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q3.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	f := nostr.Filter{Kinds: []int{1}}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 events (config default limit 3), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_RespectsDefaultCap500(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q500.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 505)

	f := nostr.Filter{Kinds: []int{1}}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 500 {
		t.Fatalf("want 500 events (default cap), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_ExplicitLimitOverridesDefault(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q4.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	lim := 2
	f := nostr.Filter{Kinds: []int{1}, Limit: &lim}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 events (explicit filter limit overrides config default), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_ZeroDefaultIsUnlimited(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q5.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	f := nostr.Filter{Kinds: []int{1}}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 events (zero default = unlimited), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_NegativeClientLimitIsUnlimited(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q6.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	neg := -5
	f := nostr.Filter{Kinds: []int{1}, Limit: &neg}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 events (negative client limit = unlimited), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_ZeroClientLimitIsUnlimited(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q7.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	zero := 0
	f := nostr.Filter{Kinds: []int{1}, Limit: &zero}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 events (zero client limit = unlimited), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_DoesNotMutateOriginalFilters(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q8.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	f1 := nostr.Filter{Kinds: []int{1}}
	f2 := nostr.Filter{Kinds: []int{7}}
	filters := []nostr.Filter{f1, f2}

	_, err = queryInitialREQEvents(ctx, st, filters, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if filters[0].Limit != nil {
		t.Fatalf("original filter[0] was mutated with limit %d, want nil", *filters[0].Limit)
	}
	if filters[1].Limit != nil {
		t.Fatalf("original filter[1] was mutated with limit %d, want nil", *filters[1].Limit)
	}
}

func TestQueryInitialREQEvents_ZeroLimitWithZeroDefaultIsUnlimited(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q9.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	zero := 0
	f := nostr.Filter{Kinds: []int{1}, Limit: &zero}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 events (explicit 0 limit with 0 config default = unlimited), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_MultipleFiltersMixedLimits(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "q10.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	two := 2
	f1 := nostr.Filter{Kinds: []int{1}, Limit: &two}
	f2 := nostr.Filter{Kinds: []int{1}}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f1, f2}, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 events after dedup (filter1=2, filter2=3, OR dedup), got %d", len(out))
	}
}
