package grpcws

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
// Client Streaming Connection
// ============================================================================

// ClientStreamConn handles client-streaming RPCs over WebSocket.
// The client sends multiple messages; server responds once at the end.
//
// RPC pattern: rpc SendCommands(stream Request) returns (Response)
type ClientStreamConn[Req proto.Message, Resp proto.Message, Stream ClientStream[Req, Resp]] struct {
	baseGRPCConn

	stream Stream
	codec  *GRPCWSCodec[Req, Resp]
}

// OnStart initializes the connection
func (c *ClientStreamConn[Req, Resp, Stream]) OnStart(conn *websocket.Conn) error {
	return c.baseGRPCConn.OnStart(conn)
}

// HandleMessage processes messages from the client
func (c *ClientStreamConn[Req, Resp, Stream]) HandleMessage(msg ControlMessage) error {
	switch msg.Type {
	case TypeData:
		// Decode and forward to gRPC stream
		req, err := c.codec.DecodeRequest(msg)
		if err != nil {
			c.sendError(err.Error())
			return nil
		}

		if err := c.stream.Send(req); err != nil {
			c.sendError(err.Error())
			return err
		}

		c.metrics.IncrementReceived()

	case TypeEndSend:
		// Client done sending, get the response
		resp, err := c.stream.CloseAndRecv()
		if err != nil {
			c.sendError(err.Error())
			return err
		}

		// Send the final response
		dataMsg, err := c.codec.EncodeData(resp)
		if err != nil {
			c.sendError(err.Error())
			return err
		}

		c.metrics.IncrementSent()
		c.SendOutput(dataMsg)
		c.sendStreamEnd()

	case TypePong:
		// Heartbeat response
		log.Printf("Received pong from client %s", c.ConnId())

	case TypeCancel:
		// Client requested cancellation
		log.Printf("Client %s requested stream cancellation", c.ConnId())
		c.cancel()

	default:
		log.Printf("Unknown message type from client: %s", msg.Type)
	}

	return nil
}

// OnClose cancels the stream and cleans up
func (c *ClientStreamConn[Req, Resp, Stream]) OnClose() {
	c.baseGRPCConn.OnClose()
	log.Printf("ClientStreamConn %s closed: received %d messages", c.ConnId(), c.metrics.MsgsReceived)
}

// ============================================================================
// Client Streaming Handler
// ============================================================================

// ClientStreamHandler creates ClientStreamConn instances for incoming connections.
type ClientStreamHandler[Req proto.Message, Resp proto.Message, Stream ClientStream[Req, Resp]] struct {
	// CreateStream creates the gRPC stream
	CreateStream func(ctx context.Context) (Stream, error)

	// NewRequest creates a new request message instance (for decoding)
	NewRequest func() Req

	// MarshalOptions for protojson encoding (optional)
	MarshalOptions protojson.MarshalOptions

	// UnmarshalOptions for protojson decoding (optional)
	UnmarshalOptions protojson.UnmarshalOptions
}

// Validate implements WSHandler. Creates the gRPC stream.
func (h *ClientStreamHandler[Req, Resp, Stream]) Validate(
	w http.ResponseWriter,
	r *http.Request,
) (*ClientStreamConn[Req, Resp, Stream], bool) {

	// Create cancellable context
	ctx, cancel := context.WithCancel(r.Context())

	// Create the gRPC stream
	stream, err := h.CreateStream(ctx)
	if err != nil {
		cancel()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}

	// Create codec with request factory
	codec := &GRPCWSCodec[Req, Resp]{
		NewRequest:       h.NewRequest,
		MarshalOptions:   h.MarshalOptions,
		UnmarshalOptions: h.UnmarshalOptions,
	}

	conn := &ClientStreamConn[Req, Resp, Stream]{
		baseGRPCConn: baseGRPCConn{
			BaseConn: gohttp.BaseConn[ControlMessage, ControlMessage]{
				Codec:   codec,
				NameStr: "ClientStreamConn",
			},
		},
		stream: stream,
		codec:  codec,
	}

	conn.initContext(ctx)
	conn.cancelFunc = cancel

	return conn, true
}

// ============================================================================
// Factory Function
// ============================================================================

// NewClientStreamHandler creates a handler for client-streaming RPCs.
//
// Example:
//
//	handler := grpcws.NewClientStreamHandler(
//	    func(ctx context.Context) (pb.GameService_SendCommandsClient, error) {
//	        return client.SendCommands(ctx)
//	    },
//	    func() *pb.GameCommand { return &pb.GameCommand{} },
//	)
func NewClientStreamHandler[Req proto.Message, Resp proto.Message, Stream ClientStream[Req, Resp]](
	createStream func(ctx context.Context) (Stream, error),
	newRequest func() Req,
) *ClientStreamHandler[Req, Resp, Stream] {
	return &ClientStreamHandler[Req, Resp, Stream]{
		CreateStream: createStream,
		NewRequest:   newRequest,
	}
}
