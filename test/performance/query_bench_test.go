//go:build bench

package performance

import (
	"context"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	sq "github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

const hexChars = "0123456789abcdef"

func hexStr(r *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = hexChars[r.Intn(len(hexChars))]
	}
	return string(b)
}

// seedData holds the pools and event IDs generated during seeding so
// benchmarks can query for rows that actually exist.
type seedData struct {
	AuthorPool []string
	ETagPool   []string
	PTagPool   []string
	EventIDs   []string
}

func openSeededStore(t testing.TB) (*sq.Store, seedData) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bench.db")
	ctx := context.Background()
	store, err := sq.Open(ctx, dir, nil, zerolog.Nop())
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatalf("open: %v", err)
	}
	r := rand.New(rand.NewSource(42))
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

	authorPool := make([]string, 20)
	for i := range authorPool {
		authorPool[i] = hexStr(r, 64)
	}
	eTagPool := make([]string, 30)
	for i := range eTagPool {
		eTagPool[i] = hexStr(r, 64)
	}
	pTagPool := make([]string, 15)
	for i := range pTagPool {
		pTagPool[i] = hexStr(r, 64)
	}
	tTagValues := []string{"hello", "world", "test", "nostr", "relay", "congee", "benchmark", "foo", "bar", "baz"}
	contents := []string{
		"hello world test content",
		"this is a test message for benchmarking",
		"world hello relay nostr",
		"just another test event content here",
		"benchmarking congee relay performance now",
	}

	eventIDs := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		kind := 1
		if r.Intn(5) == 0 {
			kind = 7
		}
		id := hexStr(r, 64)
		eventIDs[i] = id
		ev := &nostr.Event{
			ID:        id,
			PubKey:    authorPool[r.Intn(len(authorPool))],
			CreatedAt: baseTime + int64(i)*10,
			Kind:      kind,
			Content:   contents[r.Intn(len(contents))],
			Sig:       hexStr(r, 128),
		}
		numTags := 3 + r.Intn(4)
		for numTags > 0 {
			switch r.Intn(4) {
			case 0:
				ev.Tags = append(ev.Tags, []string{"e", eTagPool[r.Intn(len(eTagPool))]})
			case 1:
				ev.Tags = append(ev.Tags, []string{"p", pTagPool[r.Intn(len(pTagPool))]})
			default:
				ev.Tags = append(ev.Tags, []string{"t", tTagValues[r.Intn(len(tTagValues))]})
			}
			numTags--
		}
		if err := store.SaveEvent(ctx, ev); err != nil {
			t.Fatalf("seed event %d: %v", i, err)
		}
	}
	sd := seedData{
		AuthorPool: authorPool,
		ETagPool:   eTagPool,
		PTagPool:   pTagPool,
		EventIDs:   eventIDs,
	}
	return store, sd
}

func BenchmarkQueryEventsByID(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	f := nostr.Filter{IDs: []string{sd.EventIDs[0]}}
	var sink []*nostr.Event
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.QueryEvents(ctx, []nostr.Filter{f})
	}
	b.ReportMetric(float64(len(sink)), "events/op")
}

func BenchmarkQueryEventsByAuthor(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	f := nostr.Filter{Authors: []string{sd.AuthorPool[0]}}
	var sink []*nostr.Event
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.QueryEvents(ctx, []nostr.Filter{f})
	}
	b.ReportMetric(float64(len(sink)), "events/op")
}

func BenchmarkQueryEventsByKind(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	f := nostr.Filter{Kinds: []int{1}}
	var sink []*nostr.Event
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.QueryEvents(ctx, []nostr.Filter{f})
	}
	b.ReportMetric(float64(len(sink)), "events/op")
	_ = sd
}

func BenchmarkQueryEventsByTag(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	f := nostr.Filter{
		Tag: map[string][]string{"#e": {sd.ETagPool[0]}},
	}
	var sink []*nostr.Event
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.QueryEvents(ctx, []nostr.Filter{f})
	}
	b.ReportMetric(float64(len(sink)), "events/op")
}

func BenchmarkQueryEventsComplex(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	since := int64(1735689600)
	until := int64(1735776000)
	f := nostr.Filter{
		Authors: []string{sd.AuthorPool[0]},
		Kinds:   []int{1},
		Since:   &since,
		Until:   &until,
	}
	var sink []*nostr.Event
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.QueryEvents(ctx, []nostr.Filter{f})
	}
	b.ReportMetric(float64(len(sink)), "events/op")
}

func BenchmarkQueryEventsMultiFilter(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	f1 := nostr.Filter{Kinds: []int{1}}
	f2 := nostr.Filter{Authors: []string{sd.AuthorPool[1]}}
	filters := []nostr.Filter{f1, f2}
	var sink []*nostr.Event
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.QueryEvents(ctx, filters)
	}
	b.ReportMetric(float64(len(sink)), "events/op")
}

func BenchmarkCountEventsAll(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	var sink int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.CountEvents(ctx, nil)
	}
	b.ReportMetric(float64(sink), "events/op")
	_ = sd
}

func BenchmarkCountEventsByFilter(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	f := nostr.Filter{Kinds: []int{1}}
	var sink int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.CountEvents(ctx, []nostr.Filter{f})
	}
	b.ReportMetric(float64(sink), "events/op")
	_ = sd
}

func BenchmarkSearchEvents(b *testing.B) {
	store, sd := openSeededStore(b)
	defer store.Close()
	ctx := context.Background()
	var sink []*nostr.Event
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = store.SearchEvents(ctx, "hello", nostr.Filter{})
	}
	b.ReportMetric(float64(len(sink)), "events/op")
	_ = sd
}
