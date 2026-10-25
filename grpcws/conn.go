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
	PingID int64  `json:"pingId,omitempty"`
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

// StreamMetrics tracks connection statistics.
//
// The stream goroutines update the counters while the connection is open, and
// OnClose reads them as the connection shuts down, so the counters are atomics.
// A StreamMetrics must not be copied after first use.
type StreamMetrics struct {
	ConnectedAt  time.Time
	MsgsSent     atomic.Int64
	MsgsReceived atomic.Int64
}

// IncrementSent atomically increments the sent counter and returns the new count.
func (m *StreamMetrics) IncrementSent() int64 {
	return m.MsgsSent.Add(1)
}

// IncrementReceived atomically increments the received counter and returns the new count.
func (m *StreamMetrics) IncrementReceived() int64 {
	return m.MsgsReceived.Add(1)
}

// Sent returns the number of messages sent so far. Safe to call while the
// stream goroutines are still sending.
func (m *StreamMetrics) Sent() int64 {
	return m.MsgsSent.Load()
}

// Received returns the number of messages received so far. Safe to call while
// the stream goroutines are still receiving.
func (m *StreamMetrics) Received() int64 {
	return m.MsgsReceived.Load()
}

// DefaultCloseGracePeriod is how long a CloseOnStreamEnd connection waits for
// the client to answer the close frame before closing the socket itself.
const DefaultCloseGracePeriod = 2 * time.Second

// ConnOptions holds the connection settings shared by the server, client and
// bidi stream handlers. It is embedded in each handler, so the fields can be
// set directly on the handler. The zero value keeps the original behaviour:
// the socket stays open after stream_end and writes have no deadline.
type ConnOptions struct {
	// CloseOnStreamEnd closes the WebSocket once the RPC finishes. After the
	// stream_end frame the server sends a close frame (1000, "stream_end")
	// and the connection ends, running OnClose. Without it the socket stays
	// open until the client closes it or the pong timeout fires.
	//
	// A bidi or server stream finishes when Recv returns io.EOF. A client
	// stream finishes after the response that follows end_send.
	CloseOnStreamEnd bool

	// CloseGracePeriod bounds how long CloseOnStreamEnd waits for the client
	// to read the final frames and answer the close. After that the socket is
	// closed regardless, which is what frees a client that stopped reading.
	// Zero means DefaultCloseGracePeriod.
	CloseGracePeriod time.Duration

	// WriteTimeout bounds each write to the client (see
	// gohttp.BaseConn.WriteTimeout). A client that stops reading while the
	// stream is still producing is dropped once a write times out. Zero means
	// no deadline.
	WriteTimeout time.Duration

	// Observer is told about every stream's connection, reported with
	// Transport "grpcws" and Mode "server", "client" or "bidi". Without it,
	// WSConnConfig.Observer still applies. See gohttp.ConnObserver.
	Observer gohttp.ConnObserver
}

func (o ConnOptions) newBaseConn(codec gohttp.Codec[ControlMessage, ControlMessage], name, mode string) baseGRPCConn {
	grace := o.CloseGracePeriod
	if grace <= 0 {
		grace = DefaultCloseGracePeriod
	}
	return baseGRPCConn{
		BaseConn: gohttp.BaseConn[ControlMessage, ControlMessage]{
			Codec:        codec,
			NameStr:      name,
			WriteTimeout: o.WriteTimeout,
			Observer:     o.Observer,
			Info:         gohttp.ConnInfo{Transport: "grpcws", Mode: mode},
		},
		closeOnStreamEnd: o.CloseOnStreamEnd,
		closeGrace:       grace,
	}
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

	closeOnStreamEnd bool
	closeGrace       time.Duration
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

// endStream tells the client the RPC is done and, with CloseOnStreamEnd,
// closes the socket behind the stream_end frame.
func (c *baseGRPCConn) endStream() {
	if !c.closeOnStreamEnd {
		c.sendStreamEnd()
		return
	}
	c.CloseAfterSend(websocket.CloseNormalClosure, TypeStreamEnd, c.closeGrace,
		ControlMessage{Type: TypeStreamEnd})
}

// OnStart initializes the base connection
func (c *baseGRPCConn) OnStart(conn *websocket.Conn) error {
	return c.BaseConn.OnStart(conn)
}

// SendPing sends a ping using the gRPC-WS ControlMessage envelope format.
// This overrides BaseConn.SendPing to use the proper envelope protocol.
func (c *baseGRPCConn) SendPing() error {
	c.SendOutput(ControlMessage{
		Type:   TypePing,
		PingID: c.PingID.Add(1),
	})
	return nil
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
