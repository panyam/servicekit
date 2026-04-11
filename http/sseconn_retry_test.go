package http

import (
	"bufio"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

// TestSSERetryFieldWritten verifies that an SSE message with a non-zero
// Retry value causes the writer to emit a `retry: <ms>\n` field in the
// wire output. This is the round-trip guarantee: writer emits, reader
// parses back.
//
// Per WHATWG Server-Sent Events spec, the retry field sets the client's
// reconnection delay in milliseconds. Servers use this to hint clients
// when to reconnect — e.g., "this long-running tool will take 30s, back
// off and come back". Without writer support, servicekit's SSE hub could
// only send data/event/id but not this reconnection hint.
func TestSSERetryFieldWritten(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)
	reader := bufio.NewReader(resp.Body)

	// Send a message with Retry set. The data payload carries the event.
	var msg any = map[string]any{"ok": true}
	conn.Writer.Send(SSEOutgoingMessage[any]{
		Data:  &msg,
		Retry: 5000,
	})

	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Retry != 5000 {
		t.Errorf("Retry = %d, want 5000", ev.Retry)
	}
	if ev.Data == "" {
		t.Errorf("expected data alongside retry, got empty")
	}
}

// TestSSERetryFieldOrder verifies that retry is emitted BEFORE data lines
// so multi-line data parsing cannot accidentally consume it. The spec
// doesn't technically require ordering, but the reader relies on the fact
// that field lines come before the blank-line terminator and are
// independently parsed — still, emitting retry before data keeps the wire
// output readable by humans (retry is a per-event hint, logically a prefix).
func TestSSERetryFieldOrder(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)
	reader := bufio.NewReader(resp.Body)

	var msg any = map[string]any{"x": 1}
	conn.Writer.Send(SSEOutgoingMessage[any]{
		Data:  &msg,
		Event: "hint",
		ID:    "e1",
		Retry: 2500,
	})

	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Event != "hint" {
		t.Errorf("Event = %q, want hint", ev.Event)
	}
	if ev.ID != "e1" {
		t.Errorf("ID = %q, want e1", ev.ID)
	}
	if ev.Retry != 2500 {
		t.Errorf("Retry = %d, want 2500", ev.Retry)
	}
	if ev.Data == "" {
		t.Errorf("expected non-empty data")
	}
}

// TestSSESendRetryBare verifies that the SendRetry convenience method
// emits a bare `retry: N` event with no data payload. This is the
// "pure hint" form where the server wants to change the client's
// reconnection delay without sending any application data.
//
// The reader should return an event with Retry set and Data empty.
func TestSSESendRetryBare(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)
	reader := bufio.NewReader(resp.Body)

	conn.SendRetry(3000)

	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Retry != 3000 {
		t.Errorf("Retry = %d, want 3000", ev.Retry)
	}
	if ev.Data != "" {
		t.Errorf("Data = %q, want empty (bare retry should carry no data)", ev.Data)
	}
	if ev.Event != "" {
		t.Errorf("Event = %q, want empty", ev.Event)
	}
}

// TestSSERetryZeroOmitted verifies that when Retry is 0, no `retry:` line
// is emitted in the wire output. The reader's zero value must not round-trip
// back as retry: 0 (which would tell clients to reconnect immediately and
// cause a thundering herd on long-running servers).
//
// This guards against a regression where a default zero-value leaks through
// the writer and the reader reports Retry=0 but the wire actually had the
// literal line `retry: 0` (which is valid SSE but semantically harmful).
func TestSSERetryZeroOmitted(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)
	reader := bufio.NewReader(resp.Body)

	// Normal SendOutput leaves Retry unset (zero).
	msg := map[string]any{"plain": true}
	conn.SendOutput(msg)

	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Retry != 0 {
		t.Errorf("Retry = %d, want 0 (no retry field should have been written)", ev.Retry)
	}
}

// TestSSERetryNegativeIgnored verifies that a negative Retry value is
// treated as "not set" and no retry line is emitted. The SSE spec defines
// retry as a non-negative integer; callers passing a negative by mistake
// should not produce malformed output.
func TestSSERetryNegativeIgnored(t *testing.T) {
	handler := &NotifierSSEHandler{connChan: make(chan *NotifierSSEConn, 1)}
	router := mux.NewRouter()
	router.HandleFunc("/events", SSEServe[any](handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	resp := connectSSE(t, server.URL+"/events")
	defer resp.Body.Close()

	conn := waitForSSEConn(t, handler.connChan)
	reader := bufio.NewReader(resp.Body)

	var msg any = map[string]any{"x": 1}
	conn.Writer.Send(SSEOutgoingMessage[any]{
		Data:  &msg,
		Retry: -100,
	})

	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Retry != 0 {
		t.Errorf("Retry = %d, want 0 (negative values must be dropped)", ev.Retry)
	}
}
