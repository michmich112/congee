package relay

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"
	"github.com/gobwas/ws/wsutil"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

// wsInboundDebugTypes are client commands logged at debug (logging.level=debug).
var wsInboundDebugTypes = map[string]struct{}{
	"REQ": {}, "EVENT": {}, "AUTH": {}, "INFO": {}, "COUNT": {},
}

// ErrSlowConsumer indicates the outbound buffer is full.
var ErrSlowConsumer = errors.New("relay: send buffer full")

// Conn is one WebSocket client attachment to the relay.
type Conn struct {
	ID          string
	server      *Server
	peerIP      string
	remoteAddr  string
	wsTransport string // "plain" or "permessage-deflate" (for diagnostics)
	nc          net.Conn
	send        chan []byte
	writerDone  chan struct{}

	ctx    context.Context
	cancel context.CancelFunc

	limiter *ConnLimiter
	log     zerolog.Logger

	authMu         sync.RWMutex
	nip42Challenge string
	nip42AuthSent  bool // true after ["AUTH", challenge] was enqueued for this connection
	nip42Pubkeys   map[string]struct{}
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
			c.logReadError(err)
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
		CheckUTF8:      false,
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
			c.logReadError(err)
			return
		}
		c.dispatchPayload(payload)
	}
}

func readNextTextMessage(conn net.Conn, maxFrame int64) ([]byte, error) {
	rd := wsutil.Reader{
		Source:         conn,
		State:          ws.StateServerSide,
		CheckUTF8:      false,
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
		payload := buf.Bytes()
		if !utf8.Valid(payload) {
			return nil, newErrTextNotUTF8(payload)
		}
		return payload, nil
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
		b := payload.Bytes()
		if !utf8.Valid(b) {
			return nil, newErrTextNotUTF8(b)
		}
		return b, nil
	}
}

func (c *Conn) logInboundWSDebug(payload []byte) {
	if c.log.GetLevel() > zerolog.DebugLevel {
		return
	}
	cmd, err := nostr.PeekClientCommand(payload)
	if err != nil {
		return
	}
	if _, ok := wsInboundDebugTypes[cmd]; !ok {
		return
	}
	c.log.Debug().
		Str("remote_addr", c.remoteAddr).
		Str("ws_transport", c.wsTransport).
		Str("ws_msg_type", cmd).
		RawJSON("payload", payload).
		Msg("ws inbound client message")
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
	c.logInboundWSDebug(payload)
	msg, err := nostr.ParseMessage(payload)
	if err != nil {
		c.log.Debug().Err(err).
			Str("remote_addr", c.remoteAddr).
			Str("ws_transport", c.wsTransport).
			Int("payload_bytes", len(payload)).
			Str("payload_preview_hex", hex.EncodeToString(payloadPrefix(payload, 128))).
			Msg("client message JSON parse failed")
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
	case *nostr.AuthMessage:
		if !c.limiter.AllowReq() {
			_ = c.sendNotice("rate limited: subscription requests")
			return
		}
	}
	if err := c.server.registry.Dispatch(c.ctx, c, msg); err != nil {
		c.log.Debug().Err(err).
			Str("remote_addr", c.remoteAddr).
			Str("ws_transport", c.wsTransport).
			Msg("dispatch error")
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
	var closed wsutil.ClosedError
	if errors.As(err, &closed) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// errTextNotUTF8 marks a complete WebSocket text message whose payload is not valid UTF-8 (RFC 6455).
type errTextNotUTF8 struct {
	lenBytes   int
	previewHex string
}

func newErrTextNotUTF8(payload []byte) *errTextNotUTF8 {
	n := 64
	if len(payload) < n {
		n = len(payload)
	}
	return &errTextNotUTF8{
		lenBytes:   len(payload),
		previewHex: hex.EncodeToString(payload[:n]),
	}
}

func (e *errTextNotUTF8) Error() string {
	return fmt.Sprintf("websocket text message: invalid UTF-8 (%d bytes)", e.lenBytes)
}

func (e *errTextNotUTF8) Unwrap() error { return wsutil.ErrInvalidUTF8 }

func payloadPrefix(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	return b[:max]
}

func (c *Conn) logReadError(err error) {
	if isBenignClose(err) {
		return
	}
	var utf8Detail *errTextNotUTF8
	evt := c.log.Debug().Err(err).
		Str("remote_addr", c.remoteAddr).
		Str("peer_ip", c.peerIP).
		Str("ws_transport", c.wsTransport)
	if errors.As(err, &utf8Detail) {
		evt.Int("payload_bytes", utf8Detail.lenBytes).
			Str("payload_preview_hex", utf8Detail.previewHex).
			Msg("websocket read failed: text frame is not valid UTF-8 (RFC 6455); likely non-UTF-8/binary in a text frame, a broken client, or probes; Nostr JSON must be UTF-8")
		return
	}
	if errors.Is(err, wsutil.ErrFrameTooLarge) {
		evt.Msg("websocket read failed: frame or message exceeds max_message_bytes")
		return
	}
	if errors.Is(err, ws.ErrProtocolNonZeroRsv) {
		evt.Msg("websocket read failed: client set RSV bits but extension not negotiated for this connection (check permessage-deflate vs plain)")
		return
	}
	if errors.Is(err, wsutil.ErrInvalidUTF8) {
		evt.Msg("websocket read failed: invalid UTF-8 in text message")
		return
	}
	evt.Msg("websocket read failed")
}

// Log returns the per-connection logger (full pubkeys should be logged at call sites).
func (c *Conn) Log() zerolog.Logger { return c.log }
