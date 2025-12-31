package grpcws

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
// Server Streaming Connection
// ============================================================================

// ServerStreamConn handles server-streaming RPCs over WebSocket.
// The server sends multiple messages; client receives and can send control messages.
//
// RPC pattern: rpc Subscribe(Request) returns (stream Response)
type ServerStreamConn[Req proto.Message, Resp proto.Message, Stream ServerStream[Resp]] struct {
	baseGRPCConn

	stream Stream
	codec  *GRPCWSCodec[Req, Resp]
}

// OnStart initializes the connection and starts forwarding gRPC responses
func (c *ServerStreamConn[Req, Resp, Stream]) OnStart(conn *websocket.Conn) error {
	if err := c.baseGRPCConn.OnStart(conn); err != nil {
		return err
	}

	// Start goroutine to forward gRPC responses to WebSocket
	go c.forwardResponses()

	return nil
}

// forwardResponses reads from the gRPC stream and forwards to WebSocket
func (c *ServerStreamConn[Req, Resp, Stream]) forwardResponses() {
	for {
		resp, err := c.stream.Recv()
		if err == io.EOF {
			c.sendStreamEnd()
			return
		}
		if err != nil {
			select {
			case <-c.streamCtx.Done():
				// Context cancelled, exit silently
				return
			default:
				c.sendError(err.Error())
				return
			}
		}

		// Encode and send the response
		dataMsg, err := c.codec.EncodeData(resp)
		if err != nil {
			c.sendError(err.Error())
			return
		}

		c.metrics.IncrementSent()
		c.SendOutput(dataMsg)
	}
}

// HandleMessage processes control messages from the client
func (c *ServerStreamConn[Req, Resp, Stream]) HandleMessage(msg ControlMessage) error {
	switch msg.Type {
	case TypePong:
		// Heartbeat response - connection is alive
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

// OnClose cancels the gRPC stream and cleans up
func (c *ServerStreamConn[Req, Resp, Stream]) OnClose() {
	c.baseGRPCConn.OnClose()
	log.Printf("ServerStreamConn %s closed: sent %d messages", c.ConnId(), c.metrics.MsgsSent)
}

// ============================================================================
// Server Streaming Handler
// ============================================================================

// ServerStreamHandler creates ServerStreamConn instances for incoming connections.
type ServerStreamHandler[Req proto.Message, Resp proto.Message, Stream ServerStream[Resp]] struct {
	// CreateStream creates the gRPC stream from a request
	CreateStream func(ctx context.Context, req Req) (Stream, error)

	// ParseRequest extracts the initial request from the HTTP request
	ParseRequest func(r *http.Request) (Req, error)

	// MarshalOptions for protojson encoding (optional)
	MarshalOptions protojson.MarshalOptions

	// UnmarshalOptions for protojson decoding (optional)
	UnmarshalOptions protojson.UnmarshalOptions
}

// Validate implements WSHandler. Parses the request and creates the gRPC stream.
func (h *ServerStreamHandler[Req, Resp, Stream]) Validate(
	w http.ResponseWriter,
	r *http.Request,
) (*ServerStreamConn[Req, Resp, Stream], bool) {

	// Parse the request
	req, err := h.ParseRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(r.Context())

	// Create the gRPC stream
	stream, err := h.CreateStream(ctx, req)
	if err != nil {
		cancel()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}

	// Create codec
	codec := &GRPCWSCodec[Req, Resp]{
		MarshalOptions:   h.MarshalOptions,
		UnmarshalOptions: h.UnmarshalOptions,
	}

	conn := &ServerStreamConn[Req, Resp, Stream]{
		baseGRPCConn: baseGRPCConn{
			BaseConn: gohttp.BaseConn[ControlMessage, ControlMessage]{
				Codec:   codec,
				NameStr: "ServerStreamConn",
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

// NewServerStreamHandler creates a handler for server-streaming RPCs.
//
// Example:
//
//	handler := grpcws.NewServerStreamHandler(
//	    func(ctx context.Context, req *pb.SubscribeRequest) (pb.GameService_SubscribeClient, error) {
//	        return client.Subscribe(ctx, req)
//	    },
//	    func(r *http.Request) (*pb.SubscribeRequest, error) {
//	        vars := mux.Vars(r)
//	        return &pb.SubscribeRequest{GameId: vars["game_id"]}, nil
//	    },
//	)
func NewServerStreamHandler[Req proto.Message, Resp proto.Message, Stream ServerStream[Resp]](
	createStream func(ctx context.Context, req Req) (Stream, error),
	parseRequest func(r *http.Request) (Req, error),
) *ServerStreamHandler[Req, Resp, Stream] {
	return &ServerStreamHandler[Req, Resp, Stream]{
		CreateStream: createStream,
		ParseRequest: parseRequest,
	}
}
