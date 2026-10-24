package http

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	conc "github.com/panyam/gocurrent"
	gut "github.com/panyam/goutils/utils"
)

// OutgoingMessage represents any message that can be sent over the WebSocket.
// This union type allows pings, errors, and data messages to all go through
// the same Writer, avoiding concurrent write issues.
type OutgoingMessage[O any] struct {
	// Data is a regular output message (mutually exclusive with Ping/Error)
	Data *O

	// Ping is a heartbeat message (mutually exclusive with Data/Error)
	Ping *PingData

	// Error is an error message (mutually exclusive with Data/Ping)
	Error error

	// Close asks the Writer to send a WebSocket close frame. Queued by
	// CloseAfterSend; anything queued after it fails with ErrCloseSent.
	Close *CloseData
}

// CloseData carries the status code and reason for a WebSocket close frame.
type CloseData struct {
	Code   int
	Reason string
}

// PingData contains ping message metadata.
type PingData struct {
	PingID int64
	ConnID string
	Name   string
}

// BaseConn is a generic WebSocket connection that separates transport from encoding.
// It uses a Codec to handle message serialization/deserialization.
//
// Type parameters:
//   - I: Input message type (received from client)
//   - O: Output message type (sent to client)
//
// Usage:
//
//	type MyConn struct {
//	    gohttp.BaseConn[MyInput, MyOutput]
//	}
//
//	func (c *MyConn) HandleMessage(msg MyInput) error {
//	    // msg is already typed!
//	    return nil
//	}
type BaseConn[I any, O any] struct {
	// Codec handles message encoding/decoding.
	// Must be set before the connection is used.
	Codec Codec[I, O]

	// Writer is the output channel for sending messages.
	// Handles all outgoing messages: data, pings, and errors.
	// Initialized in OnStart.
	Writer *conc.Writer[OutgoingMessage[O]]

	// NameStr is an optional human-readable name for this connection.
	NameStr string

	// ConnIDStr is a unique identifier for this connection.
	// Auto-generated if not set.
	ConnIDStr string

	// PingID tracks the current ping sequence number. SendPing advances it on
	// the connection loop while DebugInfo and embedders may read it from other
	// goroutines, so it is atomic and BaseConn must not be copied after use.
	PingID atomic.Int64

	// WriteTimeout bounds each write to the client. A write that does not
	// finish in time (a client that has stopped reading and filled its TCP
	// window) fails, and the connection is closed so WSHandleConn exits and
	// OnClose runs. Zero means no deadline, which lets a stuck client block
	// the Writer until the pong timeout.
	WriteTimeout time.Duration

	// Observer, if set, is told when this connection opens and closes and
	// about every frame it reads or writes. WSConnConfig.Observer fills it in
	// when it is nil.
	Observer ConnObserver

	// Info is what the connection reports itself as to Observer. A zero
	// Transport reports as "ws".
	Info ConnInfo

	stats connStats

	// wsConn is the underlying WebSocket connection.
	// Set during OnStart.
	wsConn *websocket.Conn

	// closeTimer is the hard close armed by CloseAfterSend.
	closeTimer atomic.Pointer[time.Timer]
}

// Name returns the connection name.
func (b *BaseConn[I, O]) Name() string {
	if b.NameStr == "" {
		b.NameStr = "BaseConn"
	}
	return b.NameStr
}

// ConnID returns the connection ID, generating one if not set.
func (b *BaseConn[I, O]) ConnID() string {
	if b.ConnIDStr == "" {
		b.ConnIDStr = gut.RandString(10, "")
	}
	return b.ConnIDStr
}

// DebugInfo returns debug information about the connection.
func (b *BaseConn[I, O]) DebugInfo() any {
	info := map[string]any{
		"name":   b.NameStr,
		"connId": b.ConnIDStr,
		"pingId": b.PingID.Load(),
	}
	if b.Writer != nil {
		info["writer"] = b.Writer.DebugInfo()
	}
	return info
}

// ReadMessage reads and decodes the next message from the WebSocket connection.
// Uses the configured Codec to decode the raw bytes.
func (b *BaseConn[I, O]) ReadMessage(conn *websocket.Conn) (I, error) {
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		var zero I
		return zero, err
	}
	msg, err := b.Codec.Decode(data, MessageType(msgType))
	if err == nil && b.Observer != nil {
		b.Observer.MessageReceived(b.info())
	}
	return msg, err
}

// OnStart initializes the connection after WebSocket upgrade.
// Creates the Writer with codec-aware encoding.
func (b *BaseConn[I, O]) OnStart(conn *websocket.Conn) error {
	log.Printf("Starting %s connection: %s", b.Name(), b.ConnID())

	b.wsConn = conn
	b.stats.open(b.Observer, b.info())
	b.Writer = conc.NewWriter(func(msg OutgoingMessage[O]) error {
		if b.WriteTimeout > 0 {
			conn.SetWriteDeadline(time.Now().Add(b.WriteTimeout))
		}
		kind, err := b.write(conn, msg)
		if err != nil {
			// The Writer stops on any error and gorilla fails every later
			// write, so close the socket to end the read loop too.
			b.stats.noteReason(writeFailReason(err))
			conn.Close()
		} else if kind != "" && b.Observer != nil {
			b.Observer.MessageSent(b.info(), kind)
		}
		return err
	})

	return nil
}

// write sends one message and returns the frame kind it wrote ("" if none).
func (b *BaseConn[I, O]) write(conn *websocket.Conn, msg OutgoingMessage[O]) (string, error) {
	if msg.Ping != nil {
		return "ping", b.writePing(conn, msg.Ping)
	} else if msg.Error != nil {
		if msg.Error == io.EOF {
			log.Println("Stream closed...", msg.Error)
			return "", nil
		}
		return "error", b.writeError(conn, msg.Error)
	} else if msg.Data != nil {
		return "data", b.writeMessage(conn, *msg.Data)
	} else if msg.Close != nil {
		deadline := time.Now().Add(time.Second)
		if b.WriteTimeout > 0 {
			deadline = time.Now().Add(b.WriteTimeout)
		}
		return "close", conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(msg.Close.Code, msg.Close.Reason), deadline)
	}
	return "", nil
}

func (b *BaseConn[I, O]) info() ConnInfo {
	info := b.Info
	if info.Transport == "" {
		info.Transport = "ws"
	}
	return info
}

func (b *BaseConn[I, O]) settleCloseReason(r CloseReason) { b.stats.settle(r) }

func (b *BaseConn[I, O]) defaultObserver(o ConnObserver) {
	if b.Observer == nil {
		b.Observer = o
	}
}

// writeMessage encodes and sends a typed message.
func (b *BaseConn[I, O]) writeMessage(conn *websocket.Conn, msg O) error {
	data, msgType, err := b.Codec.Encode(msg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(int(msgType), data)
}

// writeError sends an error message.
// Errors are always sent as JSON text for readability.
func (b *BaseConn[I, O]) writeError(conn *websocket.Conn, err error) error {
	errMsg := map[string]any{
		"type":  "error",
		"error": err.Error(),
	}
	data, marshalErr := json.Marshal(errMsg)
	if marshalErr != nil {
		log.Printf("Failed to marshal error message: %v", marshalErr)
		return marshalErr
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// writePing sends a ping message.
// Pings are always sent as JSON text for readability and debugging.
func (b *BaseConn[I, O]) writePing(conn *websocket.Conn, ping *PingData) error {
	pingMsg := map[string]any{
		"type":   "ping",
		"pingId": ping.PingID,
		"connId": ping.ConnID,
		"name":   ping.Name,
	}
	data, marshalErr := json.Marshal(pingMsg)
	if marshalErr != nil {
		log.Printf("Failed to marshal ping message: %v", marshalErr)
		return marshalErr
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// SendPing sends a ping message through the Writer.
// This ensures thread-safe writes by going through the serialized Writer.
func (b *BaseConn[I, O]) SendPing() error {
	pingId := b.PingID.Add(1)
	if b.Writer != nil {
		b.Writer.Send(OutgoingMessage[O]{
			Ping: &PingData{
				PingID: pingId,
				ConnID: b.ConnID(),
				Name:   b.Name(),
			},
		})
	}
	return nil
}

// HandleMessage processes an incoming message.
// Default implementation just logs; override in embedding struct.
func (b *BaseConn[I, O]) HandleMessage(msg I) error {
	log.Println("Received message:", msg)
	return nil
}

// OnError handles connection errors.
// Return nil to suppress the error and continue, or return the error to close.
func (b *BaseConn[I, O]) OnError(err error) error {
	return err
}

// OnClose cleans up when the connection closes.
func (b *BaseConn[I, O]) OnClose() {
	if t := b.closeTimer.Load(); t != nil {
		t.Stop()
	}
	if b.Writer != nil {
		b.Writer.Stop()
	}
	b.stats.close(b.Observer, b.info(), CloseClient)
	log.Printf("Closed %s connection: %s", b.Name(), b.ConnID())
}

// OnTimeout handles read timeout.
// Return true to close the connection, false to keep it alive.
func (b *BaseConn[I, O]) OnTimeout() bool {
	return true
}

// SendOutput sends a typed output message to the client.
// This is a convenience method that wraps the Writer.Send call.
func (b *BaseConn[I, O]) SendOutput(msg O) {
	if b.Writer != nil {
		b.Writer.Send(OutgoingMessage[O]{Data: &msg})
	}
}

// SendError sends an error to the client.
func (b *BaseConn[I, O]) SendError(err error) {
	if b.Writer != nil {
		b.Writer.Send(OutgoingMessage[O]{Error: err})
	}
}

// InputChan returns the Writer's input channel for use with FanOut.
func (b *BaseConn[I, O]) InputChan() chan<- OutgoingMessage[O] {
	if b.Writer != nil {
		return b.Writer.InputChan()
	}
	return nil
}

// ============================================================================
// BaseHandler - Generic handler for BaseConn
// ============================================================================

// BaseHandler is a simple handler that creates BaseConn instances.
// Useful for quick prototyping or when you don't need custom validation.
type BaseHandler[I any, O any] struct {
	// Codec is used for all connections created by this handler.
	Codec Codec[I, O]

	// Name is the optional name for connections.
	Name string
}

// Validate implements WSHandler. Creates a new BaseConn with the configured codec.
func (h *BaseHandler[I, O]) Validate(w http.ResponseWriter, r *http.Request) (*BaseConn[I, O], bool) {
	return &BaseConn[I, O]{
		Codec:   h.Codec,
		NameStr: h.Name,
	}, true
}

// CloseAfterSend ends the connection from the server side. It queues last,
// then a close frame with code and reason, all behind whatever is already
// queued, so the client reads every message before the close.
//
// A client that answers the close frame ends the read loop the normal way.
// One that has stopped reading may leave the Writer blocked, so before
// queueing anything CloseAfterSend arms a timer that closes the socket after
// grace. That unblocks the Writer and the read loop, WSHandleConn returns, and
// OnClose runs (which stops the timer if the close finished first).
//
// It may block until the messages are queued or the socket is closed, so call
// it from a stream goroutine rather than from HandleMessage if the client may
// be stuck. Only the first call has any effect. A non-positive grace closes
// the socket right away. Must be called after OnStart.
func (b *BaseConn[I, O]) CloseAfterSend(code int, reason string, grace time.Duration, last ...O) {
	conn := b.wsConn
	if conn == nil || b.Writer == nil {
		return
	}
	timer := time.AfterFunc(grace, func() { conn.Close() })
	if !b.closeTimer.CompareAndSwap(nil, timer) {
		timer.Stop()
		return
	}
	b.stats.noteReason(CloseServer)
	for _, m := range last {
		b.SendOutput(m)
	}
	b.Writer.Send(OutgoingMessage[O]{Close: &CloseData{Code: code, Reason: reason}})
}
