package http

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnInfo identifies the kind of connection an Observer is told about. It
// carries no per-connection identity, so it is safe to use as a metric label
// set.
type ConnInfo struct {
	// Transport is "ws", "sse" or "grpcws".
	Transport string

	// Mode is the grpcws stream kind ("server", "client" or "bidi"), and
	// empty for the other transports.
	Mode string
}

// CloseReason says why a connection ended, as far as the server can tell.
type CloseReason string

const (
	// CloseClient means the client closed the connection, cleanly or not.
	// For SSE it means the request context ended.
	CloseClient CloseReason = "client_close"

	// CloseServer means the server ended it: CloseAfterSend (which is how
	// grpcws CloseOnStreamEnd closes), or BaseSSEConn.Close.
	CloseServer CloseReason = "server_close"

	// CloseWriteTimeout means a write missed BaseConn.WriteTimeout, usually
	// because the client stopped reading. The socket was dropped.
	CloseWriteTimeout CloseReason = "write_timeout"

	// CloseWriteError means a write failed for any other reason.
	CloseWriteError CloseReason = "write_error"

	// ClosePongTimeout means nothing was read for BiDirStreamConfig.PongPeriod.
	ClosePongTimeout CloseReason = "pong_timeout"

	// CloseReadError means a read failed and OnError chose to end the
	// connection.
	CloseReadError CloseReason = "read_error"

	// CloseStartFailed means OnStart returned an error.
	CloseStartFailed CloseReason = "start_failed"
)

// closeRank orders reasons that can race. A write failure that happens while
// a server close is in flight is the more useful thing to report: the client
// was stuck, and the close only finished because the socket was dropped.
func closeRank(r CloseReason) int {
	switch r {
	case CloseWriteTimeout, CloseWriteError:
		return 3
	case CloseServer:
		return 2
	case "":
		return 0
	}
	return 1
}

// ConnObserver is told when connections open and close and when messages go
// each way. It is how servicekit exposes connection metrics without depending
// on a metrics library: servicekit/prom implements it for Prometheus, and an
// app using OpenTelemetry can implement it over its own meter.
//
// Set it on BaseConn.Observer or BaseSSEConn.Observer, or once on
// WSConnConfig.Observer / SSEConnConfig.Observer, or on grpcws.ConnOptions.
// A nil observer costs nothing.
//
// Methods are called from the connection's own goroutines (the read loop, the
// Writer) and from many connections at once, so they must be safe for
// concurrent use and should not block. ConnOpened and ConnClosed are called
// exactly once each for every connection whose BaseConn.OnStart (or
// BaseSSEConn.OnStart) ran, including one whose own OnStart then failed, so
// opened minus closed is the number open. A connection that implements
// WSConn or SSEConn without embedding them reports nothing.
type ConnObserver interface {
	// ConnOpened is called from OnStart.
	ConnOpened(info ConnInfo)

	// ConnClosed is called from OnClose with why the connection ended and
	// how long it was open.
	ConnClosed(info ConnInfo, reason CloseReason, lifetime time.Duration)

	// MessageSent is called after a frame is written. kind is "data",
	// "ping", "error" or "close" for WebSocket, and "data", "comment" or
	// "retry" for SSE. grpcws control frames (stream_end, pings) go out as
	// data frames, so they count as "data".
	MessageSent(info ConnInfo, kind string)

	// MessageReceived is called after a frame is read and decoded.
	MessageReceived(info ConnInfo)
}

// connStats is the observer bookkeeping shared by BaseConn and BaseSSEConn.
type connStats struct {
	startedAt time.Time

	mu       sync.Mutex
	reason   CloseReason
	settled  bool
	reported bool
}

func (s *connStats) open(o ConnObserver, info ConnInfo) {
	s.startedAt = time.Now()
	if o != nil {
		o.ConnOpened(info)
	}
}

// noteReason keeps the highest-ranked reason seen so far, until settle.
func (s *connStats) noteReason(r CloseReason) {
	s.mu.Lock()
	if !s.settled && closeRank(r) > closeRank(s.reason) {
		s.reason = r
	}
	s.mu.Unlock()
}

// settle records why the read loop ended and ignores anything noted after.
// Tearing a connection down sets off more closes (cancelling a grpcws stream
// ends it, which calls CloseAfterSend), and those are effects, not causes.
func (s *connStats) settle(r CloseReason) {
	s.noteReason(r)
	s.mu.Lock()
	s.settled = true
	s.mu.Unlock()
}

func (s *connStats) close(o ConnObserver, info ConnInfo, fallback CloseReason) {
	if o == nil || s.startedAt.IsZero() {
		return
	}
	s.mu.Lock()
	done, r := s.reported, s.reason
	s.reported = true
	s.mu.Unlock()
	if done {
		return
	}
	if r == "" {
		r = fallback
	}
	o.ConnClosed(info, r, time.Since(s.startedAt))
}

// closeSettler is how WSHandleConn and SSEServe tell a connection why its
// loop ended. BaseConn and BaseSSEConn implement it, and so does anything
// that embeds them.
type closeSettler interface {
	settleCloseReason(CloseReason)
}

// observerDefaulter lets WSConnConfig.Observer and SSEConnConfig.Observer
// reach connections that did not set their own.
type observerDefaulter interface {
	defaultObserver(ConnObserver)
}

func settleClose(conn any, r CloseReason) {
	if n, ok := conn.(closeSettler); ok {
		n.settleCloseReason(r)
	}
}

// writeFailReason classifies a failed write. ErrCloseSent means the close
// handshake is already under way (gorilla answers a client's close frame
// from inside ReadMessage), so it says nothing about why.
func writeFailReason(err error) CloseReason {
	if errors.Is(err, websocket.ErrCloseSent) {
		return ""
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return CloseWriteTimeout
	}
	return CloseWriteError
}
