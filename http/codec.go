package http

import (
	"encoding/json"
	"reflect"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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
// ProtoJSONCodec - Protobuf messages serialized as JSON
// ============================================================================

// ProtoJSONCodec handles encoding/decoding of protobuf messages using JSON format.
// This provides human-readable wire format while maintaining proto type safety.
//
// For best performance, provide NewInput factory function. If not provided,
// the codec falls back to reflection using the Input exemplar (slower).
type ProtoJSONCodec[I proto.Message, O proto.Message] struct {
	// NewInput is an optional factory function to create new input instances.
	// When provided, this is used for optimal performance.
	// Example: func() *pb.PlayerAction { return &pb.PlayerAction{} }
	NewInput func() I

	// Input is an exemplar instance used as fallback when NewInput is nil.
	// The codec uses reflection to create new instances of the same type.
	// Example: &pb.PlayerAction{}
	Input I

	// MarshalOptions configures protojson marshaling behavior.
	MarshalOptions protojson.MarshalOptions

	// UnmarshalOptions configures protojson unmarshaling behavior.
	UnmarshalOptions protojson.UnmarshalOptions
}

// Decode unmarshals JSON data into a new protobuf message instance.
func (c *ProtoJSONCodec[I, O]) Decode(data []byte, msgType MessageType) (I, error) {
	msg := c.newInput()
	err := c.UnmarshalOptions.Unmarshal(data, msg)
	return msg, err
}

// Encode marshals a protobuf message to JSON bytes.
func (c *ProtoJSONCodec[I, O]) Encode(msg O) ([]byte, MessageType, error) {
	data, err := c.MarshalOptions.Marshal(msg)
	return data, TextMessage, err
}

// newInput creates a new instance of the input type.
// Uses factory function if available, otherwise falls back to reflection.
func (c *ProtoJSONCodec[I, O]) newInput() I {
	// Fast path: use factory if provided
	if c.NewInput != nil {
		return c.NewInput()
	}

	// Slow path: use reflection from exemplar
	t := reflect.TypeOf(c.Input)
	if t.Kind() == reflect.Ptr {
		return reflect.New(t.Elem()).Interface().(I)
	}
	return reflect.New(t).Elem().Interface().(I)
}

// ============================================================================
// BinaryProtoCodec - Protobuf messages in binary format
// ============================================================================

// BinaryProtoCodec handles encoding/decoding of protobuf messages using binary format.
// This provides maximum efficiency for high-throughput scenarios.
//
// For best performance, provide NewInput factory function. If not provided,
// the codec falls back to reflection using the Input exemplar (slower).
type BinaryProtoCodec[I proto.Message, O proto.Message] struct {
	// NewInput is an optional factory function to create new input instances.
	// When provided, this is used for optimal performance.
	NewInput func() I

	// Input is an exemplar instance used as fallback when NewInput is nil.
	Input I
}

// Decode unmarshals binary protobuf data into a new message instance.
func (c *BinaryProtoCodec[I, O]) Decode(data []byte, msgType MessageType) (I, error) {
	msg := c.newInput()
	err := proto.Unmarshal(data, msg)
	return msg, err
}

// Encode marshals a protobuf message to binary bytes.
func (c *BinaryProtoCodec[I, O]) Encode(msg O) ([]byte, MessageType, error) {
	data, err := proto.Marshal(msg)
	return data, BinaryMessage, err
}

// newInput creates a new instance of the input type.
// Uses factory function if available, otherwise falls back to reflection.
func (c *BinaryProtoCodec[I, O]) newInput() I {
	// Fast path: use factory if provided
	if c.NewInput != nil {
		return c.NewInput()
	}

	// Slow path: use reflection from exemplar
	t := reflect.TypeOf(c.Input)
	if t.Kind() == reflect.Ptr {
		return reflect.New(t.Elem()).Interface().(I)
	}
	return reflect.New(t).Elem().Interface().(I)
}

// ============================================================================
// Compile-time interface compliance checks
// ============================================================================

var (
	_ Codec[any, any] = (*JSONCodec)(nil)
	_ Codec[any, any] = (*TypedJSONCodec[any, any])(nil)
)
