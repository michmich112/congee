//go:build bench

package performance

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	sq "github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

var seedEpoch = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

var (
	seedStore *sq.Store
	seedData  benchSeedData
	seedCount int
)

var seedCtx = context.Background()

type benchSeedData struct {
	AuthorPool []string
	ETagPool   []string
	PTagPool   []string
	EventIDs   []string
}

func TestMain(m *testing.M) {
	numEvents, _ := strconv.Atoi(os.Getenv("BENCH_NUM_EVENTS"))
	if numEvents <= 0 {
		numEvents = 1000
	}

	tmpDir, err := ioutil.TempDir("", "congee-bench-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "bench: create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "bench.db")

	str, data, count, epoch := seedStoreAndEvents(numEvents, dbPath)
	seedStore = str
	seedData = data
	seedCount = count
	_ = epoch

	code := m.Run()

	if err := str.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "bench: close store: %v\n", err)
	}

	os.Exit(code)
}

func seedStoreAndEvents(numEvents int, dbPath string) (*sq.Store, benchSeedData, int, int64) {
	rng := rand.New(rand.NewSource(42))

	store, err := sq.Open(seedCtx, dbPath, storage.NoopNotifier{}, zerolog.Nop())
	if err != nil {
		panic(fmt.Sprintf("bench: open store: %v", err))
	}

	authorPool := generateHexPool(rng, 20)
	eTagPool := generateHexPool(rng, 30)
	pTagPool := generateHexPool(rng, 15)
	tTagValues := make([]string, 15)
	for i := range tTagValues {
		tTagValues[i] = fmt.Sprintf("tag%d", i)
	}
	contents := []string{
		"hello world",
		"this is a test event",
		"benchmark content for testing",
		"nostr is decentralized",
		"yet another event content",
	}

	seedData := benchSeedData{
		AuthorPool: authorPool,
		ETagPool:   eTagPool,
		PTagPool:   pTagPool,
	}

	var eventIDs []string
	tagsPerEvent := []int{3, 4, 5, 6}

	for i := 0; i < numEvents; i++ {
		sec := rng.Int63n(365 * 86400)
		createdAt := seedEpoch + sec
		kind := 1
		author := authorPool[rng.Intn(len(authorPool))]
		content := contents[rng.Intn(len(contents))]

		tags := make([][]string, 0, tagsPerEvent[rng.Intn(len(tagsPerEvent))])
		numTags := tagsPerEvent[rng.Intn(len(tagsPerEvent))]
		for j := 0; j < numTags; j++ {
			tagKind := rng.Intn(3)
			switch tagKind {
			case 0:
				// e tag
				eid := eTagPool[rng.Intn(len(eTagPool))]
				tags = append(tags, []string{"e", eid})
			case 1:
				// p tag
				apid := pTagPool[rng.Intn(len(pTagPool))]
				tags = append(tags, []string{"p", apid})
			case 2:
				// t tag
				tval := tTagValues[rng.Intn(len(tTagValues))]
				tags = append(tags, []string{"t", tval})
			}
		}

		event := &nostr.Event{
			PubKey:    author,
			CreatedAt: createdAt,
			Kind:      kind,
			Tags:      tags,
			Content:   content,
		}
		if id, err := event.ComputeID(); err != nil {
			panic(fmt.Sprintf("bench: compute id: %v", err))
		} else {
			event.ID = id
		}
		eventIDs = append(eventIDs, event.ID)

		if err := store.SaveEvent(seedCtx, event); err != nil {
			panic(fmt.Sprintf("bench: save event %d: %v", i, err))
		}
	}

	seedData.EventIDs = eventIDs
	return store, seedData, numEvents, seedEpoch
}

func generateHexPool(rng *rand.Rand, n int) []string {
	pool := make([]string, n)
	for i := range pool {
		var b strings.Builder
		for j := 0; j < 64; j++ {
			fmt.Fprintf(&b, "%x", rng.Intn(16))
		}
		pool[i] = b.String()
	}
	return pool
}

func BenchmarkQuery(b *testing.B) {
	st := seedStore
	sd := seedData
	epoch := seedEpoch
	ctx := seedCtx

	b.Run("QueryEventsByID", func(b *testing.B) {
		f := nostr.Filter{IDs: []string{sd.EventIDs[0]}}
		var sink []*nostr.Event
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.QueryEvents(ctx, []nostr.Filter{f})
		}
		b.ReportMetric(float64(len(sink)), "events/op")
	})

	b.Run("QueryEventsByAuthor", func(b *testing.B) {
		f := nostr.Filter{Authors: []string{sd.AuthorPool[0]}}
		var sink []*nostr.Event
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.QueryEvents(ctx, []nostr.Filter{f})
		}
		b.ReportMetric(float64(len(sink)), "events/op")
	})

	b.Run("QueryEventsByKind", func(b *testing.B) {
		f := nostr.Filter{Kinds: []int{1}}
		var sink []*nostr.Event
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.QueryEvents(ctx, []nostr.Filter{f})
		}
		b.ReportMetric(float64(len(sink)), "events/op")
	})

	b.Run("QueryEventsByTag", func(b *testing.B) {
		f := nostr.Filter{
			Tag: map[string][]string{"#e": {sd.ETagPool[0]}},
		}
		var sink []*nostr.Event
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.QueryEvents(ctx, []nostr.Filter{f})
		}
		b.ReportMetric(float64(len(sink)), "events/op")
	})

	b.Run("QueryEventsComplex", func(b *testing.B) {
		since := epoch
		until := epoch + 86400
		f := nostr.Filter{
			Authors: []string{sd.AuthorPool[0]},
			Kinds:   []int{1},
			Since:   &since,
			Until:   &until,
		}
		var sink []*nostr.Event
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.QueryEvents(ctx, []nostr.Filter{f})
		}
		b.ReportMetric(float64(len(sink)), "events/op")
	})

	b.Run("QueryEventsMultiFilter", func(b *testing.B) {
		f1 := nostr.Filter{Kinds: []int{1}}
		f2 := nostr.Filter{Authors: []string{sd.AuthorPool[1]}}
		filters := []nostr.Filter{f1, f2}
		var sink []*nostr.Event
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.QueryEvents(ctx, filters)
		}
		b.ReportMetric(float64(len(sink)), "events/op")
	})

	b.Run("CountEventsAll", func(b *testing.B) {
		var sink int
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.CountEvents(ctx, nil)
		}
		b.ReportMetric(float64(sink), "events/op")
	})

	b.Run("CountEventsByFilter", func(b *testing.B) {
		f := nostr.Filter{Kinds: []int{1}}
		var sink int
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.CountEvents(ctx, []nostr.Filter{f})
		}
		b.ReportMetric(float64(sink), "events/op")
	})

	b.Run("SearchEvents", func(b *testing.B) {
		var sink []*nostr.Event
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sink, _ = st.SearchEvents(ctx, "hello", nostr.Filter{})
		}
		b.ReportMetric(float64(len(sink)), "events/op")
	})
}
