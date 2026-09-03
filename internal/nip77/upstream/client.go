package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michmich112/congee/internal/nostr"
)

type wsClient struct {
	conn *websocket.Conn
}

func dialUpstream(ctx context.Context, rawURL string) (*wsClient, error) {
	d := websocket.Dialer{HandshakeTimeout: 15 * time.Second}

	// Many relay-facing nginx/caddy configs reject WebSocket upgrades that
	// lack an Origin header (RFC 6455 §10.2; browsers always send it).
	// Derive an https:// Origin from the wss:// URL so the upgrade is accepted.
	origin := originFromWSURL(rawURL)
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}

	conn, resp, err := d.DialContext(ctx, rawURL, hdr)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	return &wsClient{conn: conn}, nil
}

// originFromWSURL converts ws:// → http:// and wss:// → https:// for the
// Origin header, keeping only scheme+host (no path/query).
// Returns empty string if the URL scheme is unrecognised.
func originFromWSURL(rawURL string) string {
	var httpScheme, rest string
	switch {
	case strings.HasPrefix(rawURL, "wss://"):
		httpScheme = "https://"
		rest = strings.TrimPrefix(rawURL, "wss://")
	case strings.HasPrefix(rawURL, "ws://"):
		httpScheme = "http://"
		rest = strings.TrimPrefix(rawURL, "ws://")
	default:
		return ""
	}
	// Drop any path/query so Origin is scheme+host only.
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	return httpScheme + rest
}

func (c *wsClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *wsClient) sendJSON(v any) error {
	return c.conn.WriteJSON(v)
}

func (c *wsClient) readMessage(ctx context.Context, timeout time.Duration) (typ string, raw []json.RawMessage, err error) {
	if timeout <= 0 {
		timeout = time.Duration(60) * time.Second
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = c.conn.SetReadDeadline(deadline)
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return "", nil, err
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", nil, err
	}
	if len(raw) == 0 {
		return "", nil, fmt.Errorf("empty message")
	}
	if err := json.Unmarshal(raw[0], &typ); err != nil {
		return "", nil, err
	}
	return typ, raw, nil
}

func (c *wsClient) reqEventByID(ctx context.Context, id string) (*nostr.Event, error) {
	subID := "fetch-" + id[:8]
	filter := map[string]any{"ids": []string{id}}
	if err := c.sendJSON([]any{"REQ", subID, filter}); err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	for time.Now().Before(deadline) {
		typ, raw, err := c.readMessage(ctx, 30*time.Second)
		if err != nil {
			return nil, err
		}
		switch typ {
		case "EVENT":
			if len(raw) < 3 {
				continue
			}
			var ev nostr.Event
			if err := json.Unmarshal(raw[2], &ev); err != nil {
				continue
			}
			if ev.ID == id {
				_ = c.sendJSON([]any{"CLOSE", subID})
				return &ev, nil
			}
		case "EOSE":
			_ = c.sendJSON([]any{"CLOSE", subID})
			return nil, fmt.Errorf("event not found: %s", id)
		}
	}
	return nil, context.DeadlineExceeded
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
