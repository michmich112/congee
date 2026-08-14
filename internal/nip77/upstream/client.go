package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michmich112/congee/internal/nostr"
)

type wsClient struct {
	conn *websocket.Conn
}

func dialUpstream(ctx context.Context, url string) (*wsClient, error) {
	d := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := d.DialContext(ctx, url, http.Header{})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	return &wsClient{conn: conn}, nil
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

func (c *wsClient) readMessage(ctx context.Context) (typ string, raw []json.RawMessage, err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(60 * time.Second)
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
		typ, raw, err := c.readMessage(ctx)
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
