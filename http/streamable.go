package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ============================================================================
// Streamable HTTP Types
// ============================================================================

// StreamableResponse is the return type for StreamableHandlerFunc.
// It is a sealed interface — only SingleResponse and StreamResponse
// are valid implementations.
//
// This pattern enables the "POST-that-optionally-streams" transport
// introduced by MCP 2025-03-26, where a single endpoint returns either
// a synchronous JSON response or an SSE event stream.
type StreamableResponse interface {
	isStreamable() // sealed
}

// SingleResponse sends a single JSON response. This is the synchronous
// path — the server marshals Body as JSON and returns it with
// Content-Type: application/json.
//
// If StatusCode is 0, it defaults to 200.
type SingleResponse struct {
	StatusCode int // HTTP status code (default 200)
	Body       any // JSON-marshaled response body
}

func (SingleResponse) isStreamable() {}

// StreamResponse streams SSE events from the Events channel. This is the
// asynchronous path — the server sets Content-Type: text/event-stream and
// writes each event from the channel until it is closed or the client
// disconnects.
//
// The handler creates the channel, spawns a goroutine to write events,
// and closes the channel when done. StreamableServe handles SSE formatting,
// flushing, and client disconnect detection.
//
// Per WHATWG SSE spec: https://html.spec.whatwg.org/multipage/server-sent-events.html
type StreamResponse struct {
	Events <-chan SSEEvent
}

func (StreamResponse) isStreamable() {}

// SSEEvent represents a single Server-Sent Event to be streamed.
// Used by StreamResponse to describe each event in the stream.
//
// Per WHATWG SSE spec:
//   - Event sets the "event:" field (clients listen via addEventListener)
//   - Data is JSON-marshaled into the "data:" field
//   - ID sets the "id:" field (enables reconnection via Last-Event-ID)
type SSEEvent struct {
	// Event is the optional SSE event type ("event:" field).
	// If empty, the event type defaults to "message" on the client side.
	Event string

	// Data is the event payload. JSON-marshaled into the "data:" field.
	Data any

	// ID is the optional SSE event ID ("id:" field).
	// Enables client reconnection via the Last-Event-ID header.
	ID string
}

// ============================================================================
// StreamableHandlerFunc
// ============================================================================

// StreamableHandlerFunc processes an HTTP request and returns either a
// SingleResponse (synchronous JSON) or StreamResponse (SSE event stream).
//
// The handler receives the request context and the original request. It
// decides whether to respond synchronously or stream based on the request
// content, Accept header, or application logic.
//
// Example:
//
//	func myHandler(ctx context.Context, r *http.Request) gohttp.StreamableResponse {
//	    if wantsStreaming(r) {
//	        ch := make(chan gohttp.SSEEvent)
//	        go func() {
//	            defer close(ch)
//	            ch <- gohttp.SSEEvent{Data: map[string]any{"progress": 50}}
//	            ch <- gohttp.SSEEvent{Data: map[string]any{"progress": 100}}
//	        }()
//	        return gohttp.StreamResponse{Events: ch}
//	    }
//	    return gohttp.SingleResponse{Body: map[string]any{"result": "ok"}}
//	}
type StreamableHandlerFunc func(ctx context.Context, r *http.Request) StreamableResponse

// ============================================================================
// StreamableConfig
// ============================================================================

// StreamableConfig controls StreamableServe behavior.
type StreamableConfig struct {
	// Codec for serializing SSE event Data fields.
	// Only Encode() is used. Default: JSONCodec.
	Codec Codec[any, any]
}

// DefaultStreamableConfig returns a StreamableConfig with sensible defaults.
func DefaultStreamableConfig() *StreamableConfig {
	return &StreamableConfig{
		Codec: &JSONCodec{},
	}
}

// ============================================================================
// StreamableServe
// ============================================================================

// StreamableServe creates an http.HandlerFunc that supports both synchronous
// JSON responses and SSE event streaming from a single endpoint. This is the
// "POST-that-optionally-streams" pattern used by MCP 2025-03-26 Streamable HTTP.
//
// The handler function decides per-request whether to return a SingleResponse
// (JSON) or StreamResponse (SSE stream). StreamableServe handles content-type
// negotiation, SSE formatting, flushing, and client disconnect detection.
//
// Example:
//
//	router.HandleFunc("/rpc", gohttp.StreamableServe(myHandler, nil))
//
// For SSE streaming, set http.Server.WriteTimeout = 0 to prevent the server
// from closing long-lived connections.
//
// Per WHATWG SSE spec: https://html.spec.whatwg.org/multipage/server-sent-events.html
func StreamableServe(handler StreamableHandlerFunc, config *StreamableConfig) http.HandlerFunc {
	if config == nil {
		config = DefaultStreamableConfig()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		resp := handler(r.Context(), r)

		switch v := resp.(type) {
		case SingleResponse:
			writeSingleResponse(w, v)
		case StreamResponse:
			writeStreamResponse(w, r, v, config.Codec)
		default:
			http.Error(w, "internal error: unknown response type", http.StatusInternalServerError)
		}
	}
}

// writeSingleResponse marshals the body as JSON and writes it with
// Content-Type: application/json.
func writeSingleResponse(w http.ResponseWriter, resp SingleResponse) {
	w.Header().Set("Content-Type", "application/json")
	status := resp.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if resp.Body != nil {
		json.NewEncoder(w).Encode(resp.Body)
	}
}

// writeStreamResponse sets SSE headers and streams events from the channel
// until it is closed or the client disconnects.
func writeStreamResponse(w http.ResponseWriter, r *http.Request, resp StreamResponse, codec Codec[any, any]) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-resp.Events:
			if !ok {
				// Channel closed — stream complete
				return
			}
			writeSSEEvent(w, flusher, event, codec)
		}
	}
}

// writeSSEEvent formats and writes a single SSE event to the ResponseWriter.
// Uses the same wire format as BaseSSEConn.OnStart callback.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event SSEEvent, codec Codec[any, any]) {
	if event.Event != "" {
		fmt.Fprintf(w, "event: %s\n", event.Event)
	}
	if event.ID != "" {
		fmt.Fprintf(w, "id: %s\n", event.ID)
	}
	if event.Data != nil {
		data, _, err := codec.Encode(event.Data)
		if err != nil {
			// Fall back to json.Marshal if codec fails
			data, _ = json.Marshal(event.Data)
		}
		for _, line := range bytes.Split(data, []byte("\n")) {
			fmt.Fprintf(w, "data: %s\n", line)
		}
	}
	fmt.Fprint(w, "\n") // blank line terminates the event
	flusher.Flush()
}
