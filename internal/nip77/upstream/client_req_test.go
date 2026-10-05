package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReqEventByIDClosesOnTimeout(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	gotCLOSE := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var raw []json.RawMessage
		if err := conn.ReadJSON(&raw); err != nil {
			return
		}
		if jsonStringAt(raw, 0) != "REQ" {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var next []json.RawMessage
		if err := conn.ReadJSON(&next); err != nil {
			return
		}
		if jsonStringAt(next, 0) == "CLOSE" {
			gotCLOSE <- jsonStringAt(next, 1)
		}
	}))
	defer upstream.Close()

	c, err := dialUpstream(context.Background(), "ws"+strings.TrimPrefix(upstream.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.reqEventByID(context.Background(), "ab", 200*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected timeout")
	}
	select {
	case sub := <-gotCLOSE:
		if sub == "" {
			t.Fatal("CLOSE missing subscription id")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected CLOSE after timed-out REQ")
	}
}
