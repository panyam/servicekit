// Package protocodec holds the protobuf codecs for servicekit/http connections.
//
// They live here rather than in servicekit/http so that a JSON-only WebSocket or
// SSE server doesn't link google.golang.org/protobuf (issue 17). Both satisfy
// gohttp.Codec[I, O] and plug into BaseConn and BaseSSEConn the same way
// JSONCodec does:
//
//	conn := &gohttp.BaseConn[*pb.Action, *pb.State]{
//	    Codec: &protocodec.ProtoJSONCodec[*pb.Action, *pb.State]{
//	        NewInput: func() *pb.Action { return &pb.Action{} },
//	    },
//	}
//
// Until v0.2.0 these were gohttp.ProtoJSONCodec and gohttp.BinaryProtoCodec.
// Type aliases can't stay behind in servicekit/http, since an alias would import
// this package and pull protobuf back in, so callers change the import path.
package protocodec

import (
	"reflect"

	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

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
func (c *ProtoJSONCodec[I, O]) Decode(data []byte, msgType gohttp.MessageType) (I, error) {
	msg := c.newInput()
	err := c.UnmarshalOptions.Unmarshal(data, msg)
	return msg, err
}

// Encode marshals a protobuf message to JSON bytes.
func (c *ProtoJSONCodec[I, O]) Encode(msg O) ([]byte, gohttp.MessageType, error) {
	data, err := c.MarshalOptions.Marshal(msg)
	return data, gohttp.TextMessage, err
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
func (c *BinaryProtoCodec[I, O]) Decode(data []byte, msgType gohttp.MessageType) (I, error) {
	msg := c.newInput()
	err := proto.Unmarshal(data, msg)
	return msg, err
}

// Encode marshals a protobuf message to binary bytes.
func (c *BinaryProtoCodec[I, O]) Encode(msg O) ([]byte, gohttp.MessageType, error) {
	data, err := proto.Marshal(msg)
	return data, gohttp.BinaryMessage, err
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
	_ gohttp.Codec[proto.Message, proto.Message] = (*ProtoJSONCodec[proto.Message, proto.Message])(nil)
	_ gohttp.Codec[proto.Message, proto.Message] = (*BinaryProtoCodec[proto.Message, proto.Message])(nil)
)
