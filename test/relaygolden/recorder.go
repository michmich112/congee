package relaygolden

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Recorder collects outbound relay WebSocket text frames until closed or timed out.
type Recorder struct {
	mu     sync.Mutex
	frames [][]byte
}

// ReadAll reads messages until read deadline expires with no new data for idleGap,
// or until maxFrames is reached. Returns captured frames in order.
func ReadAll(c *websocket.Conn, idleGap time.Duration, maxFrames int) ([][]byte, error) {
	var rec Recorder
	defer func() { _ = c.SetReadDeadline(time.Time{}) }()
	if idleGap <= 0 {
		idleGap = 50 * time.Millisecond
	}
	if maxFrames <= 0 {
		maxFrames = 64
	}
	deadline := time.Now().Add(idleGap)
	for len(rec.frames) < maxFrames {
		_ = c.SetReadDeadline(deadline)
		_, data, err := c.ReadMessage()
		if err != nil {
			if len(rec.frames) > 0 {
				return rec.frames, nil
			}
			return rec.frames, err
		}
		rec.mu.Lock()
		rec.frames = append(rec.frames, append([]byte(nil), data...))
		rec.mu.Unlock()
		deadline = time.Now().Add(idleGap)
	}
	return rec.frames, nil
}
