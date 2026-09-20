package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

func TestHandleGetEvent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "e.db")
	st, closeStore, err := db.OpenTestStore(ctx, path, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	ev := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 1,
		Kind:      1,
		Tags:      [][]string{},
		Content:   "hi",
		Sig:       strings.Repeat("c", 128),
	}
	if err := st.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}

	h := handleGetEvent(st)

	t.Run("ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/events/"+ev.ID, nil)
		req.SetPathValue("id", ev.ID)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), `"content":"hi"`) {
			t.Fatalf("body: %s", rr.Body.String())
		}
	})

	t.Run("uppercase id normalized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/events/"+strings.ToUpper(ev.ID), nil)
		req.SetPathValue("id", strings.ToUpper(ev.ID))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		missing := strings.Repeat("f", 64)
		req := httptest.NewRequest(http.MethodGet, "/events/"+missing, nil)
		req.SetPathValue("id", missing)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("bad id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/events/nope", nil)
		req.SetPathValue("id", "nope")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status %d", rr.Code)
		}
	})
}

func TestHandleListEvents(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "e.db")
	st, closeStore, err := db.OpenTestStore(ctx, path, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	ev1 := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 1692854223,
		Kind:      34550,
		Tags:      [][]string{{"d", "IP2"}},
		Content:   "",
		Sig:       strings.Repeat("c", 128),
	}
	ev2 := &nostr.Event{
		ID:        strings.Repeat("d", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 1700955519,
		Kind:      34550,
		Tags:      [][]string{{"d", "MillionDollarExtreme"}},
		Content:   "",
		Sig:       strings.Repeat("e", 128),
	}
	evNote := &nostr.Event{
		ID:        strings.Repeat("1", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 2,
		Kind:      1,
		Tags:      [][]string{},
		Content:   "note",
		Sig:       strings.Repeat("f", 128),
	}
	for _, ev := range []*nostr.Event{ev1, ev2, evNote} {
		if err := st.SaveEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}

	h := handleListEvents(st)

	t.Run("kind required", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/events", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("kind 34550 from event store", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/events?kind=34550&limit=50&offset=0", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		var body struct {
			Events []storedEventListItem `json:"events"`
			Total  int                   `json:"total"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != 2 || len(body.Events) != 2 {
			t.Fatalf("want 2 stored 34550 events, got total=%d len=%d body=%s", body.Total, len(body.Events), rr.Body.String())
		}
		seen := map[string]bool{}
		for _, e := range body.Events {
			if e.Kind != 34550 {
				t.Fatalf("kind %d", e.Kind)
			}
			if e.PubKey != ev1.PubKey {
				t.Fatalf("pubkey truncated or wrong: %q", e.PubKey)
			}
			seen[e.ID] = true
		}
		if !seen[ev1.ID] || !seen[ev2.ID] {
			t.Fatalf("ids %v", seen)
		}
	})
}
