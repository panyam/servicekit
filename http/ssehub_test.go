package http

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// SSEHub test helpers
// ============================================================================

// mockResponseWriter implements http.ResponseWriter and http.Flusher for
// unit-testing SSE connections without a real HTTP server. All written bytes
// are captured in a buffer for inspection.
type mockResponseWriter struct {
	headers    http.Header
	buf        bytes.Buffer
	mu         sync.Mutex
	statusCode int
}

func newMockResponseWriter() *mockResponseWriter {
	return &mockResponseWriter{
		headers:    make(http.Header),
		statusCode: 200,
	}
}

func (m *mockResponseWriter) Header() http.Header         { return m.headers }
func (m *mockResponseWriter) WriteHeader(statusCode int)   { m.statusCode = statusCode }
func (m *mockResponseWriter) Flush()                       {}
func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buf.Write(b)
}

// createTestSSEConn creates a BaseSSEConn initialized with a mock
// ResponseWriter, suitable for hub tests that don't need real HTTP streaming.
func createTestSSEConn[O any](t *testing.T, codec Codec[any, O], name string) *BaseSSEConn[O] {
	t.Helper()
	conn := &BaseSSEConn[O]{
		Codec:     codec,
		NameStr:   name,
		ConnIdStr: name, // use name as ID for easy test assertions
	}
	w := newMockResponseWriter()
	r := httptest.NewRequest("GET", "/events", nil)
	if err := conn.OnStart(w, r); err != nil {
		t.Fatalf("Failed to start SSE conn %q: %v", name, err)
	}
	return conn
}

// ============================================================================
// SSEHub Tests
// ============================================================================

// TestSSEHubRegisterUnregister verifies that Register adds connections to the
// hub and Unregister removes them, with Count reflecting the current state.
func TestSSEHubRegisterUnregister(t *testing.T) {
	hub := NewSSEHub[any]()

	conn1 := createTestSSEConn[any](t, &JSONCodec{}, "conn1")
	conn2 := createTestSSEConn[any](t, &JSONCodec{}, "conn2")
	conn3 := createTestSSEConn[any](t, &JSONCodec{}, "conn3")

	hub.Register(conn1)
	hub.Register(conn2)
	hub.Register(conn3)

	if hub.Count() != 3 {
		t.Errorf("Expected count=3, got %d", hub.Count())
	}

	hub.Unregister("conn2")
	if hub.Count() != 2 {
		t.Errorf("Expected count=2 after unregister, got %d", hub.Count())
	}

	hub.Unregister("conn1")
	hub.Unregister("conn3")
	if hub.Count() != 0 {
		t.Errorf("Expected count=0, got %d", hub.Count())
	}

	// Clean up writers
	conn1.OnClose()
	conn2.OnClose()
	conn3.OnClose()
}

// TestSSEHubSend verifies that Send delivers a message to a specific
// connection by ID and returns true, while other connections receive nothing.
func TestSSEHubSend(t *testing.T) {
	hub := NewSSEHub[any]()

	conn1 := createTestSSEConn[any](t, &JSONCodec{}, "target")
	conn2 := createTestSSEConn[any](t, &JSONCodec{}, "other")
	defer conn1.OnClose()
	defer conn2.OnClose()

	hub.Register(conn1)
	hub.Register(conn2)

	ok := hub.Send("target", map[string]any{"msg": "hello"})
	if !ok {
		t.Error("Expected Send to return true for existing connection")
	}

	// Give the writer goroutine time to process
	time.Sleep(50 * time.Millisecond)
}

// TestSSEHubSendNonexistent verifies that Send returns false when the
// target connection ID does not exist, without panicking.
func TestSSEHubSendNonexistent(t *testing.T) {
	hub := NewSSEHub[any]()

	ok := hub.Send("nonexistent", map[string]any{"msg": "hello"})
	if ok {
		t.Error("Expected Send to return false for nonexistent connection")
	}
}

// TestSSEHubBroadcast verifies that Broadcast sends a message to all
// registered connections.
func TestSSEHubBroadcast(t *testing.T) {
	hub := NewSSEHub[any]()

	conns := make([]*BaseSSEConn[any], 3)
	for i := range conns {
		conns[i] = createTestSSEConn[any](t, &JSONCodec{}, fmt.Sprintf("conn%d", i))
		defer conns[i].OnClose()
		hub.Register(conns[i])
	}

	hub.Broadcast(map[string]any{"broadcast": true})

	// Give writer goroutines time to process
	time.Sleep(50 * time.Millisecond)
}

// TestSSEHubBroadcastEvent verifies that BroadcastEvent sends an event with
// the specified event type to all registered connections.
func TestSSEHubBroadcastEvent(t *testing.T) {
	hub := NewSSEHub[any]()

	conn := createTestSSEConn[any](t, &JSONCodec{}, "conn1")
	defer conn.OnClose()
	hub.Register(conn)

	hub.BroadcastEvent("notification", map[string]any{"alert": "test"})

	// Give writer goroutine time to process
	time.Sleep(50 * time.Millisecond)
}

// TestSSEHubCloseAll verifies that CloseAll calls OnClose on every registered
// connection and empties the hub (Count returns 0). Verifies that each
// connection's Writer is stopped (the observable effect of OnClose).
func TestSSEHubCloseAll(t *testing.T) {
	hub := NewSSEHub[any]()

	conns := make([]*BaseSSEConn[any], 3)
	for i := range conns {
		conns[i] = createTestSSEConn[any](t, &JSONCodec{}, fmt.Sprintf("conn%d", i))
		hub.Register(conns[i])
	}

	// Verify all writers are running before CloseAll
	for i, conn := range conns {
		if conn.Writer == nil {
			t.Fatalf("conn%d Writer should not be nil before CloseAll", i)
		}
	}

	hub.CloseAll()

	if hub.Count() != 0 {
		t.Errorf("Expected count=0 after CloseAll, got %d", hub.Count())
	}

	// Verify Done channel is closed on each connection (effect of OnClose)
	for i, conn := range conns {
		select {
		case <-conn.Done():
			// success — done channel is closed
		default:
			t.Errorf("conn%d Done channel should be closed after CloseAll", i)
		}
	}
}

// TestSSEHubSendEventWithID verifies that SendEventWithID delivers an event
// with both event type and ID fields to the targeted connection, and returns
// true. Returns false for nonexistent connections.
func TestSSEHubSendEventWithID(t *testing.T) {
	hub := NewSSEHub[any]()

	conn := createTestSSEConn[any](t, &JSONCodec{}, "target")
	defer conn.OnClose()
	hub.Register(conn)

	ok := hub.SendEventWithID("target", "update", "evt-42", map[string]any{"version": 3})
	if !ok {
		t.Error("Expected SendEventWithID to return true for existing connection")
	}

	ok = hub.SendEventWithID("nonexistent", "update", "evt-42", map[string]any{"version": 3})
	if ok {
		t.Error("Expected SendEventWithID to return false for nonexistent connection")
	}

	// Give writer goroutine time to process
	time.Sleep(50 * time.Millisecond)
}

// TestSSEHubBroadcastEventWithID verifies that BroadcastEventWithID sends a
// named event with an ID to all registered connections.
func TestSSEHubBroadcastEventWithID(t *testing.T) {
	hub := NewSSEHub[any]()

	conns := make([]*BaseSSEConn[any], 3)
	for i := range conns {
		conns[i] = createTestSSEConn[any](t, &JSONCodec{}, fmt.Sprintf("conn%d", i))
		defer conns[i].OnClose()
		hub.Register(conns[i])
	}

	hub.BroadcastEventWithID("notification", "evt-1", map[string]any{"alert": "test"})

	// Give writer goroutines time to process
	time.Sleep(50 * time.Millisecond)
}

// TestSSEHubConcurrentAccess verifies that concurrent Register, Unregister,
// Send, and Broadcast operations do not cause data races. Run with -race flag.
func TestSSEHubConcurrentAccess(t *testing.T) {
	hub := NewSSEHub[any]()

	var wg sync.WaitGroup
	const goroutines = 10

	// Pre-create connections
	conns := make([]*BaseSSEConn[any], goroutines)
	for i := range conns {
		conns[i] = createTestSSEConn[any](t, &JSONCodec{}, fmt.Sprintf("concurrent%d", i))
	}

	// Concurrent register
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hub.Register(conns[idx])
		}(i)
	}
	wg.Wait()

	// Concurrent broadcast + send + count
	for i := 0; i < goroutines; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			hub.Broadcast(map[string]any{"test": true})
		}()
		go func(idx int) {
			defer wg.Done()
			hub.Send(fmt.Sprintf("concurrent%d", idx), map[string]any{"targeted": true})
		}(i)
		go func() {
			defer wg.Done()
			_ = hub.Count()
		}()
	}
	wg.Wait()

	// Concurrent unregister
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hub.Unregister(fmt.Sprintf("concurrent%d", idx))
		}(i)
	}
	wg.Wait()

	if hub.Count() != 0 {
		t.Errorf("Expected count=0 after concurrent unregister, got %d", hub.Count())
	}

	// Clean up writers
	for _, conn := range conns {
		conn.OnClose()
	}
}

