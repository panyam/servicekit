package http

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

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
}

// PingData contains ping message metadata.
type PingData struct {
	PingId int64
	ConnId string
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

	// ConnIdStr is a unique identifier for this connection.
	// Auto-generated if not set.
	ConnIdStr string

	// PingId tracks the current ping sequence number.
	PingId int64

	// wsConn is the underlying WebSocket connection.
	// Set during OnStart.
	wsConn *websocket.Conn
}

// Name returns the connection name.
func (b *BaseConn[I, O]) Name() string {
	if b.NameStr == "" {
		b.NameStr = "BaseConn"
	}
	return b.NameStr
}

// ConnId returns the connection ID, generating one if not set.
func (b *BaseConn[I, O]) ConnId() string {
	if b.ConnIdStr == "" {
		b.ConnIdStr = gut.RandString(10, "")
	}
	return b.ConnIdStr
}

// DebugInfo returns debug information about the connection.
func (b *BaseConn[I, O]) DebugInfo() any {
	info := map[string]any{
		"name":   b.NameStr,
		"connId": b.ConnIdStr,
		"pingId": b.PingId,
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
	return b.Codec.Decode(data, MessageType(msgType))
}

// OnStart initializes the connection after WebSocket upgrade.
// Creates the Writer with codec-aware encoding.
func (b *BaseConn[I, O]) OnStart(conn *websocket.Conn) error {
	log.Printf("Starting %s connection: %s", b.Name(), b.ConnId())

	b.wsConn = conn
	b.Writer = conc.NewWriter(func(msg OutgoingMessage[O]) error {
		// Handle the different message types
		if msg.Ping != nil {
			return b.writePing(conn, msg.Ping)
		} else if msg.Error != nil {
			if msg.Error == io.EOF {
				log.Println("Stream closed...", msg.Error)
				return nil
			}
			return b.writeError(conn, msg.Error)
		} else if msg.Data != nil {
			return b.writeMessage(conn, *msg.Data)
		}
		return nil
	})

	return nil
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
	data, _ := json.Marshal(errMsg)
	return conn.WriteMessage(websocket.TextMessage, data)
}

// writePing sends a ping message.
// Pings are always sent as JSON text for readability and debugging.
func (b *BaseConn[I, O]) writePing(conn *websocket.Conn, ping *PingData) error {
	pingMsg := map[string]any{
		"type":   "ping",
		"pingId": ping.PingId,
		"connId": ping.ConnId,
		"name":   ping.Name,
	}
	data, _ := json.Marshal(pingMsg)
	return conn.WriteMessage(websocket.TextMessage, data)
}

// SendPing sends a ping message through the Writer.
// This ensures thread-safe writes by going through the serialized Writer.
func (b *BaseConn[I, O]) SendPing() error {
	b.PingId++
	if b.Writer != nil {
		b.Writer.Send(OutgoingMessage[O]{
			Ping: &PingData{
				PingId: b.PingId,
				ConnId: b.ConnId(),
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
	if b.Writer != nil {
		b.Writer.Stop()
	}
	log.Printf("Closed %s connection: %s", b.Name(), b.ConnId())
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
