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
// Bidirectional Streaming Connection
// ============================================================================

// BidiStreamConn handles bidirectional streaming RPCs over WebSocket.
// Both client and server send multiple messages concurrently.
//
// RPC pattern: rpc SyncGame(stream Request) returns (stream Response)
type BidiStreamConn[Req proto.Message, Resp proto.Message, Stream BidiStream[Req, Resp]] struct {
	baseGRPCConn

	stream Stream
	codec  *GRPCWSCodec[Req, Resp]
}

// OnStart initializes the connection and starts forwarding responses
func (c *BidiStreamConn[Req, Resp, Stream]) OnStart(conn *websocket.Conn) error {
	if err := c.baseGRPCConn.OnStart(conn); err != nil {
		return err
	}

	// Start goroutine to forward gRPC responses to WebSocket
	go c.forwardResponses()

	return nil
}

// forwardResponses reads from the gRPC stream and forwards to WebSocket
func (c *BidiStreamConn[Req, Resp, Stream]) forwardResponses() {
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

// HandleMessage processes messages from the client
func (c *BidiStreamConn[Req, Resp, Stream]) HandleMessage(msg ControlMessage) error {
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
		// Client done sending (half-close)
		if err := c.stream.CloseSend(); err != nil {
			c.sendError(err.Error())
			return err
		}
		log.Printf("Client %s half-closed the stream", c.ConnId())

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
func (c *BidiStreamConn[Req, Resp, Stream]) OnClose() {
	c.baseGRPCConn.OnClose()
	log.Printf("BidiStreamConn %s closed: sent %d, received %d messages",
		c.ConnId(), c.metrics.MsgsSent, c.metrics.MsgsReceived)
}

// ============================================================================
// Bidirectional Streaming Handler
// ============================================================================

// BidiStreamHandler creates BidiStreamConn instances for incoming connections.
type BidiStreamHandler[Req proto.Message, Resp proto.Message, Stream BidiStream[Req, Resp]] struct {
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
func (h *BidiStreamHandler[Req, Resp, Stream]) Validate(
	w http.ResponseWriter,
	r *http.Request,
) (*BidiStreamConn[Req, Resp, Stream], bool) {

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

	conn := &BidiStreamConn[Req, Resp, Stream]{
		baseGRPCConn: baseGRPCConn{
			BaseConn: gohttp.BaseConn[ControlMessage, ControlMessage]{
				Codec:   codec,
				NameStr: "BidiStreamConn",
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

// NewBidiStreamHandler creates a handler for bidirectional streaming RPCs.
//
// Example:
//
//	handler := grpcws.NewBidiStreamHandler(
//	    func(ctx context.Context) (pb.GameService_SyncGameClient, error) {
//	        return client.SyncGame(ctx)
//	    },
//	    func() *pb.PlayerAction { return &pb.PlayerAction{} },
//	)
func NewBidiStreamHandler[Req proto.Message, Resp proto.Message, Stream BidiStream[Req, Resp]](
	createStream func(ctx context.Context) (Stream, error),
	newRequest func() Req,
) *BidiStreamHandler[Req, Resp, Stream] {
	return &BidiStreamHandler[Req, Resp, Stream]{
		CreateStream: createStream,
		NewRequest:   newRequest,
	}
}
