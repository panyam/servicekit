package http

import (
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	conc "github.com/panyam/gocurrent"
)

// WSConn represents a bidirectional WebSocket connection that can handle
// typed messages of type I. It extends BiDirStreamConn with WebSocket-specific
// functionality for reading messages and connection initialization.
//
// Implementations typically embed BaseConn[I, O] and override HandleMessage.
// The type parameter I represents the input message type received from clients.
type WSConn[I any] interface {
	BiDirStreamConn[I]

	// ReadMessage reads and decodes the next message from the WebSocket connection.
	// This is called in a loop by WSHandleConn to process incoming messages.
	// Returns the decoded message or an error (including io.EOF on close).
	ReadMessage(w *websocket.Conn) (I, error)

	// OnStart is called when the WebSocket connection is established.
	// Use this to initialize the connection (e.g., set up writers, start goroutines).
	// Return an error to reject and close the connection.
	OnStart(conn *websocket.Conn) error
}

// WSHandler validates HTTP requests and creates WebSocket connections.
// It acts as a factory for WSConn instances, typically performing authentication
// and authorization before allowing the upgrade.
//
// Type parameters:
//   - I: The input message type that the connection will handle
//   - S: The specific WSConn implementation type (must implement WSConn[I])
type WSHandler[I any, S WSConn[I]] interface {
	// Validate checks if the HTTP request should be upgraded to a WebSocket.
	// Return (connection, true) to proceed with the upgrade.
	// Return (nil, false) to reject (the handler should write the error response).
	Validate(w http.ResponseWriter, r *http.Request) (S, bool)
}

// WSConnConfig combines BiDirStreamConfig with WebSocket-specific settings.
// It controls connection upgrade behavior and lifecycle timing.
type WSConnConfig struct {
	*BiDirStreamConfig
	// Upgrader handles the HTTP to WebSocket protocol upgrade.
	// Configure ReadBufferSize, WriteBufferSize, and CheckOrigin as needed.
	Upgrader websocket.Upgrader
}

// DefaultWSConnConfig returns a WSConnConfig with sensible defaults:
//   - ReadBufferSize: 1024 bytes
//   - WriteBufferSize: 1024 bytes
//   - CheckOrigin: allows all origins (configure for production!)
//   - PingPeriod: 30 seconds
//   - PongPeriod: 300 seconds (5 minutes)
func DefaultWSConnConfig() *WSConnConfig {
	return &WSConnConfig{
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		BiDirStreamConfig: DefaultBiDirStreamConfig(),
	}
}

// WSServe creates an http.HandlerFunc that upgrades HTTP requests to WebSocket
// connections and manages their lifecycle. This is the primary entry point for
// creating WebSocket endpoints.
//
// The handler validates incoming requests and creates connection instances.
// The config controls upgrade behavior and timing; if nil, DefaultWSConnConfig is used.
//
// Example:
//
//	router.HandleFunc("/ws", gohttp.WSServe(&MyHandler{}, nil))
//
// The lifecycle is:
//  1. handler.Validate() is called to check the request
//  2. If valid, the connection is upgraded to WebSocket
//  3. conn.OnStart() is called to initialize the connection
//  4. Messages are read and passed to conn.HandleMessage()
//  5. On close, conn.OnClose() is called for cleanup
func WSServe[I any, S WSConn[I]](handler WSHandler[I, S], config *WSConnConfig) http.HandlerFunc {
	if config == nil {
		config = DefaultWSConnConfig()
	}
	return func(rw http.ResponseWriter, req *http.Request) {
		ctx, isValid := handler.Validate(rw, req)
		if !isValid {
			return
		}

		// Standard upgrade to WS .....
		conn, err := config.Upgrader.Upgrade(rw, req, nil)
		if err != nil {
			http.Error(rw, "WS Upgrade failed", 400)
			log.Println("WS upgrade failed: ", err)
			return
		}
		defer conn.Close()

		log.Println("Start Handling Conn with: ", ctx)
		WSHandleConn(conn, ctx, config)
	}
}

// WSHandleConn manages the lifecycle of an established WebSocket connection.
// It handles:
//   - Periodic ping messages for connection health checks
//   - Timeout detection when no data is received within PongPeriod
//   - Message reading and dispatching to ctx.HandleMessage()
//   - Error handling via ctx.OnError()
//   - Clean shutdown via ctx.OnClose()
//
// This function is called automatically by WSServe, but can also be used directly
// when you have an established WebSocket connection from another source.
//
// The function blocks until the connection is closed or an unrecoverable error occurs.
func WSHandleConn[I any, S WSConn[I]](conn *websocket.Conn, ctx S, config *WSConnConfig) {
	if config == nil {
		config = DefaultWSConnConfig()
	}
	reader := conc.NewReader(func() (I, error) {
		res, err := ctx.ReadMessage(conn)
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
			return res, net.ErrClosed
		}
		return res, err
	})
	defer reader.Stop()

	lastReadAt := time.Now()
	pingTimer := time.NewTicker(config.PingPeriod)
	pongChecker := time.NewTicker(config.PongPeriod)
	defer pingTimer.Stop()
	defer pongChecker.Stop()

	defer ctx.OnClose()
	err := ctx.OnStart(conn)
	if err != nil {
		return
	}

	conn.SetReadDeadline(time.Now().Add(config.PongPeriod))
	for {
		select {
		case <-pingTimer.C:
			ctx.SendPing()
			break
		case <-pongChecker.C:
			hb_delta := time.Now().Sub(lastReadAt).Seconds()
			if hb_delta > config.PongPeriod.Seconds() {
				// Lost connection with conn so can drop off?
				if ctx.OnTimeout() {
					log.Printf("Last heart beat more than %d seconds ago.  Killing connection", int(hb_delta))
					return
				}
			}
			break
		case result := <-reader.OutputChan():
			conn.SetReadDeadline(time.Now().Add(config.PongPeriod))
			lastReadAt = time.Now()
			if result.Error != nil {
				if result.Error != io.EOF {
					if ce, ok := result.Error.(*websocket.CloseError); ok {
						log.Println("WebSocket Closed: ", ce)
						switch ce.Code {
						case websocket.CloseAbnormalClosure:
						case websocket.CloseNormalClosure:
						case websocket.CloseGoingAway:
							return
						}
					}
					if ctx.OnError(result.Error) != nil {
						log.Println("Closing due to Error: ", result.Error)
						return
					}
				}
			} else {
				// we have an actual message being sent on this channel - typically
				// dont need to do anything as we are using these for outbound connections
				// only to write to a listening agent FE so can just log and drop any
				// thing sent by agent FE here - this can change later
				ctx.HandleMessage(result.Value)
			}
			break
		}
	}
}

// JSONConn is an alias for BaseConn with untyped JSON messages.
// This provides a simple way to handle dynamic JSON messages.
//
// For typed messages, use BaseConn[YourInputType, YourOutputType] directly
// with an appropriate codec.
type JSONConn = BaseConn[any, any]

// NewJSONConn creates a new JSONConn with the default JSON codec.
func NewJSONConn() *JSONConn {
	return &JSONConn{
		Codec:   &JSONCodec{},
		NameStr: "JSONConn",
	}
}

// JSONHandler is a simple handler that creates JSONConn instances.
// All connections are accepted (no validation).
type JSONHandler struct{}

// Validate implements WSHandler. Accepts all connections.
func (j *JSONHandler) Validate(w http.ResponseWriter, r *http.Request) (*JSONConn, bool) {
	return NewJSONConn(), true
}
