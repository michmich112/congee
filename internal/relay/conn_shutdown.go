package relay

import (
	"time"
)

const connShutdownWriterWait = 5 * time.Second

// initiateShutdown forcefully stops a WebSocket client attachment. It is safe to
// call from any goroutine and idempotent (sync.Once).
//
// Read and write loops are unblocked via immediate I/O deadlines, the outbound
// queue is closed, and the underlying conn is closed.
func (c *Conn) initiateShutdown() {
	if c == nil {
		return
	}
	c.shutdownOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		if c.nc != nil {
			now := time.Now()
			_ = c.nc.SetReadDeadline(now)
			_ = c.nc.SetWriteDeadline(now)
			_ = c.nc.Close()
		}
		c.sendMu.Lock()
		if !c.outboundClosed {
			c.outboundClosed = true
			close(c.send)
		}
		c.sendMu.Unlock()
	})
}

// waitWriterDone blocks until the write loop exits or timeout elapses.
func (c *Conn) waitWriterDone(timeout time.Duration) bool {
	if c == nil || c.writerDone == nil {
		return true
	}
	if timeout <= 0 {
		<-c.writerDone
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-c.writerDone:
		return true
	case <-timer.C:
		return false
	}
}
