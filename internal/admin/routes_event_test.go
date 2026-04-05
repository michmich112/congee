package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage/sqlite"
)

func TestHandleGetEvent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "e.db")
	st, err := sqlite.Open(ctx, path, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

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
