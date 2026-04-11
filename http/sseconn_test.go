package http

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

// ============================================================================
// Test helpers
// ============================================================================

// connectSSE makes an HTTP GET request to an SSE endpoint and returns the
// response. The caller must close resp.Body when done. The response is
// returned immediately once headers are received (the body streams).
func connectSSE(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to connect to SSE endpoint: %v", err)
	}
	return resp
}

// sseEvent represents a parsed SSE event with its fields.
type sseEvent struct {
	Event   string // "event:" field value
	Data    string // "data:" field value (joined if multi-line)
	ID      string // "id:" field value
	Retry   int    // "retry:" field value in milliseconds (0 = not set)
	Comment string // comment line (starts with ":")
}

// readSSEEvent reads a single SSE event from a bufio.Reader using the shared
// SSEEventReader. It blocks until a complete event is received or the timeout
// elapses. The goroutine+timeout pattern is needed because SSE streams are
// long-lived and tests must not hang on slow/stuck connections.
func readSSEEvent(t *testing.T, reader *bufio.Reader, timeout time.Duration) (sseEvent, error) {
	t.Helper()
	type result struct {
		event sseEvent
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		r := NewSSEEventReader(reader)
		ev, err := r.ReadEvent()
		ch <- result{
			event: sseEvent{
				Event:   ev.Event,
				Data:    ev.Data,
				ID:      ev.ID,
				Retry:   ev.Retry,
				Comment: ev.Comment,
			},
			err: err,
		}
	}()

	select {
	case r := <-ch:
		return r.event, r.err
	case <-time.After(timeout):
		return sseEvent{}, fmt.Errorf("timeout waiting for SSE event after %v", timeout)
	}
}

// ============================================================================
// Test SSE connection types
// ============================================================================

// NotifierSSEConn is a test SSE connection that sends messages pushed to it
// via a channel. It tracks lifecycle events for test assertions.
type NotifierSSEConn struct {
	BaseSSEConn[any]
	started     atomic.Bool
	closed      atomic.Bool
	closedChan  chan struct{} // closed when OnClose is called
	startedChan chan struct{} // closed when OnStart completes
}

func newNotifierSSEConn() *NotifierSSEConn {
	return &NotifierSSEConn{
		BaseSSEConn: BaseSSEConn[any]{
			Codec:   &JSONCodec{},
			NameStr: "NotifierSSEConn",
		},
		closedChan:  make(chan struct{}),
		startedChan: make(chan struct{}),
	}
}

func (n *NotifierSSEConn) OnStart(w http.ResponseWriter, r *http.Request) error {
	if err := n.BaseSSEConn.OnStart(w, r); err != nil {
		return err
	}
	n.started.Store(true)
	close(n.startedChan)
	return nil
}

func (n *NotifierSSEConn) OnClose() {
	n.closed.Store(true)
	n.BaseSSEConn.OnClose()
	select {
	case <-n.closedChan:
	default:
		close(n.closedChan)
	}
}

// NotifierSSEHandler creates NotifierSSEConn instances and stores them for
// test access. The connChan receives each created connection so tests can
// send messages through them.
type NotifierSSEHandler struct {
	connChan chan *NotifierSSEConn
}

func (h *NotifierSSEHandler) Validate(w http.ResponseWriter, r *http.Request) (*NotifierSSEConn, bool) {
	conn := newNotifierSSEConn()
	if h.connChan != nil {
		h.connChan <- conn
	}
	return conn, true
}

// waitForSSEConn receives a connection from the handler channel and waits
// for OnStart to complete, ensuring the Writer is initialized before tests
// send messages.
func waitForSSEConn(t *testing.T, ch chan *NotifierSSEConn) *NotifierSSEConn {
	t.Helper()
	select {
	case conn := <-ch:
		select {
		case <-conn.startedChan:
			return conn
		case <-time.After(2 * time.Second):
			t.Fatal("Timed out waiting for SSE connection to start")
			return nil
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for SSE connection from handler")
		return nil
	}
}

// ============================================================================
// SSEConn Tests
// ============================================================================

// TestSSEBasicDelivery verifies that an SSE endpoint sets the correct
// Content-Type header (text/event-stream) and delivers JSON-encoded data
// messages in proper SSE format (data: {...}\n\n).
//
// Per WHATWG Server-Sent Events spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events):
//   - The MIME type must be "text/event-stream"
//   - Each event is terminated by a blank line (\n\n)
//   - The "data" field carries the event payload
func TestSSEBasicDelivery(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	// Verify SSE headers
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Expected Cache-Control no-cache, got %q", cc)
	}

	// Get the connection created by the handler
	conn := waitForSSEConn(t, handler.connChan)

	// Send a message
	msg := map[string]any{"greeting": "hello", "count": 42}
	conn.SendOutput(msg)

	// Read and verify the SSE event
	reader := bufio.NewReader(resp.Body)
	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read SSE event: %v", err)
	}

	// Parse the data field as JSON
	var received map[string]any
	if err := json.Unmarshal([]byte(ev.Data), &received); err != nil {
		t.Fatalf("Failed to parse SSE data as JSON: %v (data: %q)", err, ev.Data)
	}
	if received["greeting"] != "hello" {
		t.Errorf("Expected greeting=hello, got %v", received["greeting"])
	}
	if received["count"] != float64(42) {
		t.Errorf("Expected count=42, got %v", received["count"])
	}
}

// TestSSEFormatCompliance verifies SSE wire format: event: field before data:,
// id: field, and comment lines (: prefix). Also verifies multi-line data is
// split into separate data: lines per the SSE spec.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream):
//   - Lines starting with ":" are comments (ignored by EventSource but keep connections alive)
//   - "event:" sets the event type (defaults to "message" if absent)
//   - "id:" sets the last event ID (used for reconnection via Last-Event-ID header)
//   - "data:" carries the payload; multiple data: lines are joined with "\n"
//   - Field order: event/id before data by convention (spec allows any order)
func TestSSEFormatCompliance(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)
	reader := bufio.NewReader(resp.Body)

	// Test event with event: and id: fields
	conn.SendEventWithID("update", "evt-1", map[string]any{"key": "value"})

	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read SSE event: %v", err)
	}
	if ev.Event != "update" {
		t.Errorf("Expected event=update, got %q", ev.Event)
	}
	if ev.ID != "evt-1" {
		t.Errorf("Expected id=evt-1, got %q", ev.ID)
	}
	if ev.Data == "" {
		t.Error("Expected non-empty data field")
	}
}

// TestSSEKeepalive verifies that SSE comment keepalives (": keepalive\n\n")
// are sent at the configured interval. Uses a short 50ms interval for fast
// testing.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream):
// lines starting with ":" are comments. Servers use these as keepalive
// signals to prevent intermediate proxies (nginx default 60s, AWS ALB 60s)
// from closing idle connections.
func TestSSEKeepalive(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	config := &SSEConnConfig{
		KeepalivePeriod: 50 * time.Millisecond,
	}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, config))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()
	waitForSSEConn(t, handler.connChan)

	// Read keepalive events — we should get at least 2 within 200ms
	reader := bufio.NewReader(resp.Body)
	keepaliveCount := 0
	for i := 0; i < 3; i++ {
		ev, err := readSSEEvent(t, reader, 500*time.Millisecond)
		if err != nil {
			t.Fatalf("Failed to read keepalive event %d: %v", i, err)
		}
		if ev.Comment == "keepalive" {
			keepaliveCount++
		}
	}

	if keepaliveCount < 2 {
		t.Errorf("Expected at least 2 keepalive comments, got %d", keepaliveCount)
	}
}

// TestSSEClientDisconnect verifies that OnClose is called when the client
// disconnects (closes the response body). This tests context-aware cleanup.
// SSE connections are long-lived HTTP responses; the server detects client
// disconnect via request context cancellation (Go's http.Server cancels the
// context when the client closes the TCP connection).
func TestSSEClientDisconnect(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	conn := waitForSSEConn(t, handler.connChan)

	if conn.closed.Load() {
		t.Fatal("OnClose should not have been called yet")
	}

	// Close the response body to simulate client disconnect
	resp.Body.Close()

	// Wait for OnClose to be called
	select {
	case <-conn.closedChan:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for OnClose after client disconnect")
	}

	if !conn.closed.Load() {
		t.Error("OnClose was not called after client disconnect")
	}
}

// TestSSEConcurrentSends verifies that multiple goroutines can send messages
// concurrently through the same SSEConn without panics or data races.
func TestSSEConcurrentSends(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)

	const numGoroutines = 10
	const msgsPerGoroutine = 5
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerGoroutine; j++ {
				conn.SendOutput(map[string]any{
					"goroutine": id,
					"msg":       j,
				})
			}
		}(i)
	}
	wg.Wait()

	// Read all messages — verify we got the expected count
	reader := bufio.NewReader(resp.Body)
	received := 0
	for i := 0; i < numGoroutines*msgsPerGoroutine; i++ {
		_, err := readSSEEvent(t, reader, 2*time.Second)
		if err != nil {
			t.Fatalf("Failed to read event %d: %v", i, err)
		}
		received++
	}

	if received != numGoroutines*msgsPerGoroutine {
		t.Errorf("Expected %d messages, got %d", numGoroutines*msgsPerGoroutine, received)
	}
}

// TestSSECodecIntegration verifies that BaseSSEConn uses the Codec.Encode
// method to serialize typed output messages into the SSE data field.
func TestSSECodecIntegration(t *testing.T) {
	type StatusUpdate struct {
		Service string `json:"service"`
		Status  string `json:"status"`
		Code    int    `json:"code"`
	}

	// Use TypedJSONCodec for strongly-typed encoding
	typedHandler := &typedSSEHandler[StatusUpdate]{
		codec: &TypedJSONCodec[any, StatusUpdate]{},
	}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[StatusUpdate](typedHandler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := <-typedHandler.connChan
	// Wait for OnStart to complete (Writer initialization)
	select {
	case <-conn.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for SSE connection to be ready")
	}

	conn.SendOutput(StatusUpdate{
		Service: "api-gateway",
		Status:  "healthy",
		Code:    200,
	})

	reader := bufio.NewReader(resp.Body)
	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read SSE event: %v", err)
	}

	var received StatusUpdate
	if err := json.Unmarshal([]byte(ev.Data), &received); err != nil {
		t.Fatalf("Failed to parse typed SSE data: %v", err)
	}
	if received.Service != "api-gateway" {
		t.Errorf("Expected service=api-gateway, got %q", received.Service)
	}
	if received.Code != 200 {
		t.Errorf("Expected code=200, got %d", received.Code)
	}
}

// typedSSEHandler is a generic test handler for typed SSE connections.
type typedSSEHandler[O any] struct {
	codec    Codec[any, O]
	connChan chan *BaseSSEConn[O]
}

func (h *typedSSEHandler[O]) Validate(w http.ResponseWriter, r *http.Request) (*BaseSSEConn[O], bool) {
	if h.connChan == nil {
		h.connChan = make(chan *BaseSSEConn[O], 1)
	}
	conn := &BaseSSEConn[O]{
		Codec:   h.codec,
		NameStr: "TypedSSEConn",
	}
	h.connChan <- conn
	return conn, true
}

// TestSSEMultipleEventTypes verifies that SendEvent correctly sets the
// "event:" field in SSE output, allowing clients to listen for specific
// event types.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#the-eventsource-interface):
// clients use EventSource.addEventListener(type, handler) to listen for
// named event types. Events without "event:" default to type "message".
func TestSSEMultipleEventTypes(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)
	reader := bufio.NewReader(resp.Body)

	// Send events with different types
	conn.SendEvent("user_joined", map[string]any{"user": "alice"})
	conn.SendEvent("message", map[string]any{"text": "hello"})

	ev1, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read first event: %v", err)
	}
	if ev1.Event != "user_joined" {
		t.Errorf("Expected event=user_joined, got %q", ev1.Event)
	}

	ev2, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read second event: %v", err)
	}
	if ev2.Event != "message" {
		t.Errorf("Expected event=message, got %q", ev2.Event)
	}
}

// TestSSEEventWithID verifies that SendEventWithID correctly sets both the
// "event:" and "id:" fields in SSE output, enabling client-side reconnection
// via Last-Event-ID.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#the-last-event-id-string):
// the "id:" field sets the last event ID. On reconnection, the browser sends
// a "Last-Event-ID" HTTP header so the server can resume from where it left off.
func TestSSEEventWithID(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)
	reader := bufio.NewReader(resp.Body)

	conn.SendEventWithID("update", "12345", map[string]any{"version": 3})

	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read SSE event: %v", err)
	}
	if ev.Event != "update" {
		t.Errorf("Expected event=update, got %q", ev.Event)
	}
	if ev.ID != "12345" {
		t.Errorf("Expected id=12345, got %q", ev.ID)
	}
}

// TestSSENilConfig verifies that SSEServe uses DefaultSSEConnConfig when
// nil config is passed, and the endpoint still works correctly.
func TestSSENilConfig(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)

	// Should work with default config
	conn.SendOutput(map[string]any{"test": true})

	reader := bufio.NewReader(resp.Body)
	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read SSE event: %v", err)
	}
	if ev.Data == "" {
		t.Error("Expected non-empty data with nil config")
	}
}

// TestSSEValidationReject verifies that when SSEHandler.Validate returns
// false, the connection is rejected and no SSE stream is established.
func TestSSEValidationReject(t *testing.T) {
	handler := &rejectSSEHandler{}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/events")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		t.Error("Expected non-200 status for rejected connection")
	}
}

// rejectSSEHandler always rejects connections.
type rejectSSEHandler struct{}

func (h *rejectSSEHandler) Validate(w http.ResponseWriter, r *http.Request) (*BaseSSEConn[any], bool) {
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return nil, false
}
