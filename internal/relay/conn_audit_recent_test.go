package relay

import (
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testRecentClosedServer() *Server {
	cfg := testRelayConfig()
	return &Server{
		cfg:  cfg,
		subs: NewSubscriptionManager(cfg, zerolog.Nop()),
	}
}

func TestConnAuditRecentClosedCap(t *testing.T) {
	t.Parallel()
	srv := testRecentClosedServer()
	ended := time.Now().Unix()
	for i := 0; i < connAuditRecentClosedMax+5; i++ {
		c := &Conn{
			server:      srv,
			ID:          fmt.Sprintf("%08x", i),
			peerIP:      "127.0.0.1",
			remoteAddr:  "127.0.0.1:1",
			startedUnix: ended - 10,
		}
		srv.recordRecentClosedSession(c, ended+int64(i))
	}
	got := srv.ConnAuditRecentClosedSummaries()
	if len(got) != connAuditRecentClosedMax {
		t.Fatalf("len=%d want %d", len(got), connAuditRecentClosedMax)
	}
	if got[0].ConnID != fmt.Sprintf("%08x", connAuditRecentClosedMax+4) {
		t.Fatalf("newest conn_id=%q want %q", got[0].ConnID, fmt.Sprintf("%08x", connAuditRecentClosedMax+4))
	}
	if got[len(got)-1].ConnID != fmt.Sprintf("%08x", 5) {
		t.Fatalf("oldest conn_id=%q want %q", got[len(got)-1].ConnID, fmt.Sprintf("%08x", 5))
	}
}

func TestConnAuditRecentClosedDetailByConnID(t *testing.T) {
	t.Parallel()
	srv := testRecentClosedServer()
	c := &Conn{
		server:      srv,
		ID:          "abc12345",
		peerIP:      "10.0.0.2",
		remoteAddr:  "10.0.0.2:99",
		startedUnix: 100,
	}
	srv.recordRecentClosedSession(c, 200)
	d, ok := srv.ConnAuditRecentClosedDetailByConnID("abc12345")
	if !ok || d == nil {
		t.Fatal("expected detail")
	}
	if d.EndedUnix != 200 || d.ConnID != "abc12345" {
		t.Fatalf("detail: %+v", d)
	}
	if _, ok := srv.ConnAuditRecentClosedDetailByConnID("missing"); ok {
		t.Fatal("expected missing")
	}
}
