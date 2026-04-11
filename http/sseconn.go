package http

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	conc "github.com/panyam/gocurrent"
	gut "github.com/panyam/goutils/utils"
)

// ============================================================================
// SSE Configuration
// ============================================================================

// SSEConnConfig controls the behavior of SSE connections.
type SSEConnConfig struct {
	// KeepalivePeriod specifies how often to send SSE comment keepalives.
	// SSE comments (lines starting with ":") are ignored by EventSource clients
	// but keep the TCP connection alive through proxies (nginx default idle
	// timeout: 60s, AWS ALB: 60s).
	//
	// Set to 0 to disable keepalives. Default: 30 seconds.
	//
	// See WHATWG SSE spec: https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
	KeepalivePeriod time.Duration
}

// DefaultSSEConnConfig returns an SSEConnConfig with sensible defaults:
//   - KeepalivePeriod: 30 seconds
//
// These defaults keep connections alive through most reverse proxies.
// For SSE endpoints behind proxies with shorter idle timeouts, reduce the
// keepalive period accordingly.
func DefaultSSEConnConfig() *SSEConnConfig {
	return &SSEConnConfig{
		KeepalivePeriod: 30 * time.Second,
	}
}

// ============================================================================
// SSE Message Types
// ============================================================================

// SSEOutgoingMessage represents a message to be sent over an SSE connection.
// This union type allows data messages and keepalive comments to go through
// the same Writer, avoiding concurrent write issues.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream):
//   - "event:" sets the event type (clients listen via addEventListener)
//   - "data:" carries the payload (multiple lines joined with "\n")
//   - "id:" sets the last event ID (used for reconnection via Last-Event-ID header)
//   - "retry:" sets the client's reconnection delay in milliseconds
//   - Lines starting with ":" are comments (used for keepalive)
type SSEOutgoingMessage[O any] struct {
	// Data is a regular output message. Encoded via Codec, sent as SSE "data:" field.
	// Mutually exclusive with Comment.
	Data *O

	// Event is the SSE event type ("event:" field). Only used with Data.
	// If empty, the event type defaults to "message" on the client side.
	Event string

	// ID is the SSE event ID ("id:" field). Only used with Data.
	// Enables client reconnection via the Last-Event-ID header.
	ID string

	// Retry is the reconnection delay hint in milliseconds, emitted as the
	// SSE "retry:" field. When non-zero, the client uses this value as its
	// next reconnection delay if the connection drops. Zero or negative
	// values are ignored (no retry line written).
	//
	// Can be sent alongside Data (as a combined event+hint) or as a bare
	// hint via SendRetry (no Data, just the retry line).
	Retry int

	// Comment is an SSE comment line (for keepalive). Mutually exclusive with Data.
	// Sent as ": {comment}\n\n".
	Comment string
}

// ============================================================================
// SSEConn Interface
// ============================================================================

// SSEConn represents a server-sent events connection. It is the SSE counterpart
// to WSConn — write-only (server to client), with lifecycle hooks for setup
// and teardown.
//
// Implementations typically embed BaseSSEConn[O] and may override OnStart/OnClose
// for custom initialization and cleanup logic.
//
// SSE connections are unidirectional (server → client) per the WHATWG spec:
// https://html.spec.whatwg.org/multipage/server-sent-events.html
type SSEConn[O any] interface {
	// Name returns a human-readable name for this connection type.
	// Used for logging and debugging.
	Name() string

	// ConnId returns a unique identifier for this connection instance.
	// Used for tracking in SSEHub and logging.
	ConnId() string

	// OnStart is called when the SSE connection is established.
	// Receives the ResponseWriter (which must implement http.Flusher) and
	// the Request (whose Context signals client disconnect).
	// Return an error to reject the connection.
	OnStart(w http.ResponseWriter, r *http.Request) error

	// OnClose is called when the connection ends (client disconnect or server close).
	// Use this to clean up resources.
	OnClose()

	// SendKeepalive sends an SSE comment as a keepalive signal.
	// Called automatically by SSEServe at the configured interval.
	SendKeepalive()

	// Done returns a channel that is closed when the connection should terminate.
	// SSEServe monitors this channel to detect programmatic close requests.
	Done() <-chan struct{}
}

// ============================================================================
// BaseSSEConn — Embeddable SSE connection
// ============================================================================

// BaseSSEConn is a generic SSE connection that handles message serialization
// and thread-safe writes. It mirrors BaseConn[I, O] but for write-only SSE
// streams.
//
// Type parameter O is the output message type sent to clients.
//
// Usage:
//
//	type MySSEConn struct {
//	    gohttp.BaseSSEConn[MyEvent]
//	}
//
//	func (c *MySSEConn) OnStart(w http.ResponseWriter, r *http.Request) error {
//	    if err := c.BaseSSEConn.OnStart(w, r); err != nil {
//	        return err
//	    }
//	    // custom initialization
//	    return nil
//	}
//
// Important: For SSE endpoints, set http.Server.WriteTimeout = 0 to prevent
// the server from closing long-lived SSE connections. See middleware.ApplyDefaults
// documentation for details.
type BaseSSEConn[O any] struct {
	// Codec handles output message serialization.
	// Only Encode() is used (SSE has no input decoding).
	// Must be set before the connection is used.
	Codec Codec[any, O]

	// Writer serializes all outgoing messages through a single goroutine.
	// Initialized in OnStart. Use SendOutput/SendEvent instead of direct access.
	Writer *conc.Writer[SSEOutgoingMessage[O]]

	// NameStr is an optional human-readable name for this connection.
	NameStr string

	// ConnIdStr is a unique identifier for this connection.
	// Auto-generated if not set.
	ConnIdStr string

	// ready is closed when OnStart completes and the Writer is initialized.
	// Initialized eagerly via initReady(). Use Ready() to wait.
	ready     chan struct{}
	readyOnce sync.Once

	// done is closed when the connection should terminate.
	// Used by SSEServe to detect programmatic close via Close().
	done     chan struct{}
	doneOnce sync.Once
}

// Name returns the connection name.
func (b *BaseSSEConn[O]) Name() string {
	if b.NameStr == "" {
		b.NameStr = "SSEConn"
	}
	return b.NameStr
}

// ConnId returns the connection ID, generating one if not set.
func (b *BaseSSEConn[O]) ConnId() string {
	if b.ConnIdStr == "" {
		b.ConnIdStr = gut.RandString(10, "")
	}
	return b.ConnIdStr
}

// OnStart initializes the SSE connection. It asserts that the ResponseWriter
// supports http.Flusher (required for streaming), then creates the Writer
// with SSE-format dispatch.
//
// The Writer callback formats each message according to the SSE wire protocol:
//   - Comments: ": {text}\n\n"
//   - Data events: "event: {type}\nid: {id}\ndata: {json}\n\n"
//
// Per WHATWG SSE spec, multi-line data is split into separate "data:" lines.
func (b *BaseSSEConn[O]) OnStart(w http.ResponseWriter, r *http.Request) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported: ResponseWriter does not implement http.Flusher")
	}

	log.Printf("Starting %s SSE connection: %s", b.Name(), b.ConnId())

	// Flush headers immediately so the client receives them before any
	// data events. This must happen before creating the Writer goroutine
	// to avoid a concurrent write race on the ResponseWriter.
	flusher.Flush()

	b.initReady()
	b.done = make(chan struct{})
	b.Writer = conc.NewWriter(func(msg SSEOutgoingMessage[O]) error {
		// Handle keepalive comments
		if msg.Comment != "" {
			fmt.Fprintf(w, ": %s\n\n", msg.Comment)
			flusher.Flush()
			return nil
		}

		// Handle bare retry hint (no data). Used by SendRetry to change the
		// client's reconnection delay without delivering application data.
		if msg.Data == nil && msg.Retry > 0 {
			fmt.Fprintf(w, "retry: %d\n\n", msg.Retry)
			flusher.Flush()
			return nil
		}

		// Handle data messages
		if msg.Data != nil {
			data, _, err := b.Codec.Encode(*msg.Data)
			if err != nil {
				return err
			}

			if msg.Event != "" {
				fmt.Fprintf(w, "event: %s\n", msg.Event)
			}
			if msg.ID != "" {
				fmt.Fprintf(w, "id: %s\n", msg.ID)
			}
			// Retry is emitted before data so clients that parse
			// field-by-field see the hint alongside the event payload.
			// Per SSE spec, negative or zero values are dropped.
			if msg.Retry > 0 {
				fmt.Fprintf(w, "retry: %d\n", msg.Retry)
			}

			// Per SSE spec, multi-line data must be split into separate data: lines
			for _, line := range bytes.Split(data, []byte("\n")) {
				fmt.Fprintf(w, "data: %s\n", line)
			}
			fmt.Fprint(w, "\n") // blank line terminates the event
			flusher.Flush()
		}
		return nil
	})

	close(b.ready)
	return nil
}

// initReady ensures the ready channel is created exactly once.
func (b *BaseSSEConn[O]) initReady() {
	b.readyOnce.Do(func() {
		b.ready = make(chan struct{})
	})
}

// Ready returns a channel that is closed when OnStart completes and the
// Writer is initialized. Safe to call before OnStart (the channel is
// created lazily). Use this to wait before sending messages:
//
//	<-conn.Ready()
//	conn.SendOutput(msg)
func (b *BaseSSEConn[O]) Ready() <-chan struct{} {
	b.initReady()
	return b.ready
}

// OnClose cleans up the SSE connection by stopping the Writer and closing
// the done channel. Always call the parent OnClose when overriding:
//
//	func (c *MySSEConn) OnClose() {
//	    // custom cleanup
//	    c.BaseSSEConn.OnClose()
//	}
func (b *BaseSSEConn[O]) OnClose() {
	if b.Writer != nil {
		b.Writer.Stop()
	}
	b.doneOnce.Do(func() {
		if b.done != nil {
			close(b.done)
		}
	})
	log.Printf("Closed %s SSE connection: %s", b.Name(), b.ConnId())
}

// SendOutput sends a data message to the client. The message is serialized
// using the configured Codec and formatted as an SSE data event.
//
// This is a convenience method for sending events without a named type.
// For named events, use SendEvent or SendEventWithID.
func (b *BaseSSEConn[O]) SendOutput(msg O) {
	if b.Writer != nil {
		b.Writer.Send(SSEOutgoingMessage[O]{Data: &msg})
	}
}

// SendEvent sends a named event to the client. The event type is set via
// the SSE "event:" field, allowing clients to listen with addEventListener.
//
// Per WHATWG SSE spec, if no event type is specified, clients receive the
// event via the "message" event handler.
func (b *BaseSSEConn[O]) SendEvent(event string, msg O) {
	if b.Writer != nil {
		b.Writer.Send(SSEOutgoingMessage[O]{Data: &msg, Event: event})
	}
}

// SendEventWithID sends a named event with an ID. The ID is set via the
// SSE "id:" field, enabling client-side reconnection via the Last-Event-ID
// header.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#the-last-event-id-string):
// on reconnection, the browser includes "Last-Event-ID: {id}" so the server
// can resume the event stream from the correct position.
func (b *BaseSSEConn[O]) SendEventWithID(event string, id string, msg O) {
	if b.Writer != nil {
		b.Writer.Send(SSEOutgoingMessage[O]{Data: &msg, Event: event, ID: id})
	}
}

// SendKeepalive sends an SSE comment as a keepalive signal. The comment
// format (": keepalive\n\n") is ignored by EventSource clients but prevents
// intermediate proxies from closing idle connections.
func (b *BaseSSEConn[O]) SendKeepalive() {
	if b.Writer != nil {
		b.Writer.Send(SSEOutgoingMessage[O]{Comment: "keepalive"})
	}
}

// SendRetry emits a bare SSE "retry:" field to change the client's
// reconnection delay. Used for server-initiated disconnect hints — e.g., a
// long-running tool handler tells the client to back off for N milliseconds
// before reconnecting. Has no effect if ms <= 0 or the Writer is nil.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream):
// the retry field sets the client's reconnection time in integer
// milliseconds. Clients that do not support the field ignore it.
//
// To combine a retry hint with a data delivery in one event, set Retry on
// an SSEOutgoingMessage that also has Data set, and pass it to the Writer
// directly.
func (b *BaseSSEConn[O]) SendRetry(ms int) {
	if b.Writer != nil && ms > 0 {
		b.Writer.Send(SSEOutgoingMessage[O]{Retry: ms})
	}
}

// Done returns a channel that is closed when the connection should terminate.
// Use this to signal SSEServe to exit the event loop from application code.
func (b *BaseSSEConn[O]) Done() <-chan struct{} {
	return b.done
}

// Close signals the connection to terminate. SSEServe will detect this via
// the Done channel and call OnClose.
func (b *BaseSSEConn[O]) Close() {
	b.doneOnce.Do(func() {
		if b.done != nil {
			close(b.done)
		}
	})
}

// InputChan returns the Writer's input channel for use with FanOut.
func (b *BaseSSEConn[O]) InputChan() chan<- SSEOutgoingMessage[O] {
	if b.Writer != nil {
		return b.Writer.InputChan()
	}
	return nil
}

// ============================================================================
// SSEHandler — Factory interface
// ============================================================================

// SSEHandler validates HTTP requests and creates SSE connections.
// It acts as a factory for SSEConn instances, analogous to WSHandler for
// WebSocket connections.
//
// Type parameters:
//   - O: The output message type sent to the client
//   - S: The specific SSEConn implementation type
type SSEHandler[O any, S SSEConn[O]] interface {
	// Validate checks if the HTTP request should establish an SSE stream.
	// Return (connection, true) to proceed with the SSE connection.
	// Return (nil, false) to reject (the handler should write the error response).
	Validate(w http.ResponseWriter, r *http.Request) (S, bool)
}

// ============================================================================
// SSEServe — HTTP handler factory
// ============================================================================

// SSEServe creates an http.HandlerFunc that establishes SSE connections and
// manages their lifecycle. This is the primary entry point for creating SSE
// endpoints, analogous to WSServe for WebSocket endpoints.
//
// The handler validates incoming requests and creates connection instances.
// The config controls keepalive behavior; if nil, DefaultSSEConnConfig is used.
//
// Example:
//
//	router.HandleFunc("/events", gohttp.SSEServe[MyEvent](&MySSEHandler{}, nil))
//
// The lifecycle is:
//  1. handler.Validate() is called to check the request
//  2. SSE headers are set (Content-Type, Cache-Control, etc.)
//  3. conn.OnStart() is called to initialize the connection
//  4. Keepalive comments are sent at the configured interval
//  5. On client disconnect (context cancellation), conn.OnClose() is called
//
// Important: Set http.Server.WriteTimeout = 0 for SSE endpoints to prevent
// the server from closing long-lived connections. See middleware.ApplyDefaults.
//
// Per WHATWG SSE spec: https://html.spec.whatwg.org/multipage/server-sent-events.html
func SSEServe[O any, S SSEConn[O]](handler SSEHandler[O, S], config *SSEConnConfig) http.HandlerFunc {
	if config == nil {
		config = DefaultSSEConnConfig()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		conn, valid := handler.Validate(w, r)
		if !valid {
			return
		}

		// Set SSE response headers per the WHATWG spec
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// Disable proxy buffering (nginx, Varnish, etc.)
		w.Header().Set("X-Accel-Buffering", "no")

		if err := conn.OnStart(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.OnClose()

		// Note: header flush happens inside OnStart, before the Writer
		// goroutine is created, to avoid concurrent ResponseWriter access.

		// Start keepalive ticker if configured
		var keepaliveTicker *time.Ticker
		var keepaliveC <-chan time.Time
		if config.KeepalivePeriod > 0 {
			keepaliveTicker = time.NewTicker(config.KeepalivePeriod)
			keepaliveC = keepaliveTicker.C
			defer keepaliveTicker.Stop()
		}

		// Block until client disconnects or connection is programmatically closed
		for {
			select {
			case <-r.Context().Done():
				return
			case <-conn.Done():
				return
			case <-keepaliveC:
				conn.SendKeepalive()
			}
		}
	}
}

// ============================================================================
// Convenience types
// ============================================================================

// JSONSSEConn is an alias for BaseSSEConn with untyped JSON messages.
// This provides a simple way to send dynamic JSON data over SSE.
//
// For typed messages, use BaseSSEConn[YourOutputType] directly with an
// appropriate codec.
type JSONSSEConn = BaseSSEConn[any]

// JSONSSEHandler is a simple handler that creates JSONSSEConn instances.
// All connections are accepted (no validation).
type JSONSSEHandler struct{}

// Validate implements SSEHandler. Accepts all connections.
func (h *JSONSSEHandler) Validate(w http.ResponseWriter, r *http.Request) (*JSONSSEConn, bool) {
	return &JSONSSEConn{
		Codec:   &JSONCodec{},
		NameStr: "JSONSSEConn",
	}, true
}

// ============================================================================
// Compile-time interface compliance checks
// ============================================================================

var _ SSEConn[any] = (*BaseSSEConn[any])(nil)
