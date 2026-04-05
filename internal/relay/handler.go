package relay

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"
	"github.com/gobwas/ws/wsutil"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

// ErrSlowConsumer indicates the outbound buffer is full.
var ErrSlowConsumer = errors.New("relay: send buffer full")

// Conn is one WebSocket client attachment to the relay.
type Conn struct {
	ID         string
	server     *Server
	peerIP     string
	remoteAddr string
	nc         net.Conn
	send       chan []byte
	writerDone chan struct{}

	ctx    context.Context
	cancel context.CancelFunc

	limiter *ConnLimiter
	log     zerolog.Logger
}

func newConnID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (c *Conn) writeLoop() {
	defer close(c.writerDone)
	wd := time.Duration(c.server.cfg.ConnectionLimits.WriteDeadlineSeconds) * time.Second
	for b := range c.send {
		if err := c.nc.SetWriteDeadline(time.Now().Add(wd)); err != nil {
			return
		}
		if err := wsutil.WriteServerMessage(c.nc, ws.OpText, b); err != nil {
			return
		}
	}
}

func (c *Conn) readLoopPlain() {
	max := int64(c.server.cfg.WebSocket.MaxMessageBytes)
	rd := time.Duration(c.server.cfg.ConnectionLimits.ReadDeadlineSeconds) * time.Second
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		if err := c.nc.SetReadDeadline(time.Now().Add(rd)); err != nil {
			return
		}
		payload, err := readNextTextMessage(c.nc, max)
		if err != nil {
			if !isBenignClose(err) {
				c.log.Debug().Err(err).Msg("read error")
			}
			return
		}
		c.dispatchPayload(payload)
	}
}

func (c *Conn) readLoopFlate() {
	max := int64(c.server.cfg.WebSocket.MaxMessageBytes)
	rdSec := time.Duration(c.server.cfg.ConnectionLimits.ReadDeadlineSeconds) * time.Second

	fr := wsflate.NewReader(nil, func(r io.Reader) wsflate.Decompressor {
		return flate.NewReader(r)
	})
	var msg wsflate.MessageState
	rd := wsutil.Reader{
		Source:         c.nc,
		State:          ws.StateServerSide | ws.StateExtended,
		CheckUTF8:      true,
		MaxFrameSize:   max,
		OnIntermediate: wsutil.ControlFrameHandler(c.nc, ws.StateServerSide),
		Extensions:     []wsutil.RecvExtension{&msg},
	}

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		if err := c.nc.SetReadDeadline(time.Now().Add(rdSec)); err != nil {
			return
		}
		payload, err := readOneFlateText(c.nc, &rd, fr, &msg, max)
		if err != nil {
			if !isBenignClose(err) {
				c.log.Debug().Err(err).Msg("read error")
			}
			return
		}
		c.dispatchPayload(payload)
	}
}

func readNextTextMessage(conn net.Conn, maxFrame int64) ([]byte, error) {
	rd := wsutil.Reader{
		Source:         conn,
		State:          ws.StateServerSide,
		CheckUTF8:      true,
		MaxFrameSize:   maxFrame,
		OnIntermediate: wsutil.ControlFrameHandler(conn, ws.StateServerSide),
	}
	for {
		h, err := rd.NextFrame()
		if err != nil {
			return nil, err
		}
		if h.OpCode.IsControl() {
			if err := wsutil.ControlFrameHandler(conn, ws.StateServerSide)(h, &rd); err != nil {
				return nil, err
			}
			continue
		}
		if h.OpCode != ws.OpText {
			_, _ = io.Copy(io.Discard, &rd)
			continue
		}
		var buf bytes.Buffer
		_, err = io.Copy(&buf, &rd)
		if err != nil {
			return nil, err
		}
		if maxFrame > 0 && int64(buf.Len()) > maxFrame {
			return nil, wsutil.ErrFrameTooLarge
		}
		return buf.Bytes(), nil
	}
}

func readOneFlateText(
	conn net.Conn,
	rd *wsutil.Reader,
	fr *wsflate.Reader,
	msg *wsflate.MessageState,
	maxTotal int64,
) ([]byte, error) {
	for {
		h, err := rd.NextFrame()
		if err != nil {
			return nil, err
		}
		if h.OpCode.IsControl() {
			if err := wsutil.ControlFrameHandler(conn, ws.StateServerSide)(h, rd); err != nil {
				return nil, err
			}
			continue
		}
		if h.OpCode != ws.OpText {
			_, _ = io.Copy(io.Discard, rd)
			continue
		}
		var payload bytes.Buffer
		src := io.Reader(rd)
		if msg.IsCompressed() {
			fr.Reset(src)
			src = fr
		}
		if _, err := io.Copy(&payload, src); err != nil {
			return nil, err
		}
		if maxTotal > 0 && int64(payload.Len()) > maxTotal {
			return nil, wsutil.ErrFrameTooLarge
		}
		return payload.Bytes(), nil
	}
}

func (c *Conn) dispatchPayload(payload []byte) {
	if !c.server.limiter.AllowMessage(c.peerIP) {
		_ = c.sendNotice("rate limited: too many messages from this IP")
		return
	}
	if !c.limiter.AllowInboundBytes(len(payload)) {
		_ = c.sendNotice("rate limited: bandwidth")
		return
	}
	msg, err := nostr.ParseMessage(payload)
	if err != nil {
		_ = c.sendNotice("invalid message")
		return
	}
	switch msg.(type) {
	case *nostr.EventMessage:
		if !c.limiter.AllowEvent() {
			_ = c.sendNotice("rate limited: events")
			return
		}
	case *nostr.ReqMessage:
		if !c.limiter.AllowReq() {
			_ = c.sendNotice("rate limited: subscription requests")
			return
		}
	}
	if err := c.server.registry.Dispatch(c.ctx, c, msg); err != nil {
		c.log.Debug().Err(err).Msg("dispatch error")
		_ = c.sendNotice(err.Error())
	}
}

func (c *Conn) sendNotice(msg string) error {
	b, err := nostr.MarshalRelayNotice(msg)
	if err != nil {
		return err
	}
	return c.enqueue(b)
}

func (c *Conn) enqueue(b []byte) error {
	select {
	case c.send <- b:
		return nil
	case <-c.ctx.Done():
		return context.Canceled
	default:
		return ErrSlowConsumer
	}
}

func (c *Conn) sendOK(eventID string, ok bool, msg string) error {
	b, err := nostr.MarshalRelayOK(eventID, ok, msg)
	if err != nil {
		return err
	}
	return c.enqueue(b)
}

func (c *Conn) sendEOSE(subID string) error {
	b, err := nostr.MarshalRelayEOSE(subID)
	if err != nil {
		return err
	}
	return c.enqueue(b)
}

func (c *Conn) sendEvent(subID string, ev *nostr.Event) error {
	b, err := nostr.MarshalRelayEvent(subID, ev)
	if err != nil {
		return err
	}
	return c.enqueue(b)
}

func (c *Conn) sendClosed(subID, msg string) error {
	b, err := nostr.MarshalRelayClosed(subID, msg)
	if err != nil {
		return err
	}
	return c.enqueue(b)
}

func isBenignClose(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// Log returns the per-connection logger (full pubkeys should be logged at call sites).
func (c *Conn) Log() zerolog.Logger { return c.log }
