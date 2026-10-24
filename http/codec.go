package http

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

// MessageType represents the WebSocket frame type
type MessageType int

const (
	// TextMessage denotes a text data message (UTF-8 encoded)
	TextMessage MessageType = websocket.TextMessage // 1

	// BinaryMessage denotes a binary data message
	BinaryMessage MessageType = websocket.BinaryMessage // 2
)

// Codec handles encoding/decoding of messages over WebSocket.
// The type parameters I and O represent input (received) and output (sent) message types.
// Note: Pings are handled at the transport layer (BaseConn), not by codecs.
type Codec[I any, O any] interface {
	// Decode converts raw WebSocket data into a typed input message.
	// msgType indicates whether the data was received as text or binary.
	Decode(data []byte, msgType MessageType) (I, error)

	// Encode converts a typed output message to raw bytes for sending.
	// Returns the encoded bytes and the appropriate message type (text/binary).
	Encode(msg O) ([]byte, MessageType, error)
}

// ============================================================================
// JSONCodec - Untyped JSON for dynamic messages
// ============================================================================

// JSONCodec handles encoding/decoding of arbitrary JSON messages.
// This is useful for dynamic message handling where the structure isn't known at compile time.
type JSONCodec struct{}

// Decode unmarshals JSON data into an untyped any value.
func (c *JSONCodec) Decode(data []byte, msgType MessageType) (any, error) {
	var out any
	err := json.Unmarshal(data, &out)
	return out, err
}

// Encode marshals any value to JSON bytes.
func (c *JSONCodec) Encode(msg any) ([]byte, MessageType, error) {
	data, err := json.Marshal(msg)
	return data, TextMessage, err
}

// ============================================================================
// TypedJSONCodec - Strongly-typed JSON messages
// ============================================================================

// TypedJSONCodec handles encoding/decoding of strongly-typed JSON messages.
// Use this when you have known Go struct types for your messages.
type TypedJSONCodec[I any, O any] struct{}

// Decode unmarshals JSON data into a typed value.
// Creates a new instance of I using Go's zero value mechanism.
func (c *TypedJSONCodec[I, O]) Decode(data []byte, msgType MessageType) (I, error) {
	var out I
	err := json.Unmarshal(data, &out)
	return out, err
}

// Encode marshals a typed value to JSON bytes.
func (c *TypedJSONCodec[I, O]) Encode(msg O) ([]byte, MessageType, error) {
	data, err := json.Marshal(msg)
	return data, TextMessage, err
}

// ============================================================================
// Compile-time interface compliance checks
// ============================================================================

var (
	_ Codec[any, any] = (*JSONCodec)(nil)
	_ Codec[any, any] = (*TypedJSONCodec[any, any])(nil)
)
