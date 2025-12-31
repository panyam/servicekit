package grpcws

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
// Message Types - JSON envelope for control messages
// ============================================================================

// MessageType constants for the JSON envelope
const (
	TypeData      = "data"
	TypeError     = "error"
	TypeStreamEnd = "stream_end"
	TypePing      = "ping"
	TypePong      = "pong"
	TypeCancel    = "cancel"
	TypeEndSend   = "end_send"
)

// ControlMessage represents the JSON envelope for all WebSocket messages
type ControlMessage struct {
	Type   string `json:"type"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
	PingId int64  `json:"pingId,omitempty"`
}

// ============================================================================
// Codec for gRPC-WebSocket (JSON envelope with proto payload)
// ============================================================================

// GRPCWSCodec handles encoding/decoding of gRPC messages over WebSocket.
// It uses a JSON envelope for control messages with protojson payloads.
type GRPCWSCodec[Req proto.Message, Resp proto.Message] struct {
	// NewRequest creates a new request message instance
	NewRequest func() Req

	// MarshalOptions for protojson encoding
	MarshalOptions protojson.MarshalOptions

	// UnmarshalOptions for protojson decoding
	UnmarshalOptions protojson.UnmarshalOptions
}

// Decode parses a control message from raw WebSocket data
func (c *GRPCWSCodec[Req, Resp]) Decode(data []byte, msgType gohttp.MessageType) (ControlMessage, error) {
	var msg ControlMessage
	err := json.Unmarshal(data, &msg)
	return msg, err
}

// Encode wraps a response in a control message envelope
func (c *GRPCWSCodec[Req, Resp]) Encode(msg ControlMessage) ([]byte, gohttp.MessageType, error) {
	data, err := json.Marshal(msg)
	return data, gohttp.TextMessage, err
}

// EncodePing creates a ping control message
func (c *GRPCWSCodec[Req, Resp]) EncodePing(pingId int64, connId, name string) ([]byte, gohttp.MessageType, error) {
	msg := ControlMessage{
		Type:   TypePing,
		PingId: pingId,
	}
	data, err := json.Marshal(msg)
	return data, gohttp.TextMessage, err
}

// EncodeData wraps a proto response in a data message
func (c *GRPCWSCodec[Req, Resp]) EncodeData(resp Resp) (ControlMessage, error) {
	protoData, err := c.MarshalOptions.Marshal(resp)
	if err != nil {
		return ControlMessage{}, err
	}

	// Convert to map for JSON envelope
	var dataMap any
	if err := json.Unmarshal(protoData, &dataMap); err != nil {
		return ControlMessage{}, err
	}

	return ControlMessage{
		Type: TypeData,
		Data: dataMap,
	}, nil
}

// DecodeRequest parses a request from a control message's data field
func (c *GRPCWSCodec[Req, Resp]) DecodeRequest(msg ControlMessage) (Req, error) {
	req := c.NewRequest()

	// Convert data back to JSON bytes
	dataBytes, err := json.Marshal(msg.Data)
	if err != nil {
		return req, err
	}

	err = c.UnmarshalOptions.Unmarshal(dataBytes, req)
	return req, err
}

// ============================================================================
// Stream Interfaces - abstraction over gRPC stream types
// ============================================================================

// ServerStream is the interface for server-streaming gRPC clients
type ServerStream[Resp proto.Message] interface {
	Recv() (Resp, error)
	grpc.ClientStream
}

// ClientStream is the interface for client-streaming gRPC clients
type ClientStream[Req proto.Message, Resp proto.Message] interface {
	Send(Req) error
	CloseAndRecv() (Resp, error)
	grpc.ClientStream
}

// BidiStream is the interface for bidirectional streaming gRPC clients
type BidiStream[Req proto.Message, Resp proto.Message] interface {
	Send(Req) error
	Recv() (Resp, error)
	CloseSend() error
	grpc.ClientStream
}

// ============================================================================
// Connection Metrics
// ============================================================================

// StreamMetrics tracks connection statistics
type StreamMetrics struct {
	ConnectedAt  time.Time
	MsgsSent     int64
	MsgsReceived int64
}

// IncrementSent atomically increments the sent counter
func (m *StreamMetrics) IncrementSent() int64 {
	return atomic.AddInt64(&m.MsgsSent, 1)
}

// IncrementReceived atomically increments the received counter
func (m *StreamMetrics) IncrementReceived() int64 {
	return atomic.AddInt64(&m.MsgsReceived, 1)
}

// ============================================================================
// Base gRPC-WS Connection
// ============================================================================

// baseGRPCConn provides common functionality for all gRPC-WS connection types
type baseGRPCConn struct {
	gohttp.BaseConn[ControlMessage, ControlMessage]

	streamCtx  context.Context
	cancelFunc context.CancelFunc
	metrics    StreamMetrics
}

// initContext creates a cancellable context for the gRPC stream
func (c *baseGRPCConn) initContext(parent context.Context) {
	c.streamCtx, c.cancelFunc = context.WithCancel(parent)
	c.metrics.ConnectedAt = time.Now()
}

// cancel cancels the gRPC stream context
func (c *baseGRPCConn) cancel() {
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
}

// sendData sends a proto message wrapped in a data envelope
func (c *baseGRPCConn) sendData(data any) {
	c.SendOutput(ControlMessage{
		Type: TypeData,
		Data: data,
	})
}

// sendError sends an error message
func (c *baseGRPCConn) sendError(errMsg string) {
	c.SendOutput(ControlMessage{
		Type:  TypeError,
		Error: errMsg,
	})
}

// sendStreamEnd sends a stream_end message
func (c *baseGRPCConn) sendStreamEnd() {
	c.SendOutput(ControlMessage{
		Type: TypeStreamEnd,
	})
}

// OnStart initializes the base connection
func (c *baseGRPCConn) OnStart(conn *websocket.Conn) error {
	return c.BaseConn.OnStart(conn)
}

// OnClose cleans up the connection
func (c *baseGRPCConn) OnClose() {
	c.cancel()
	c.BaseConn.OnClose()
}

// ============================================================================
// Compile-time interface check
// ============================================================================

var _ gohttp.Codec[ControlMessage, ControlMessage] = (*GRPCWSCodec[proto.Message, proto.Message])(nil)
