package http

import (
	"bufio"
	"encoding/json"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

// ============================================================================
// Streamable HTTP Tests
// ============================================================================

// TestStreamable_SingleResponse verifies that when the handler returns a
// SingleResponse, the server responds with application/json Content-Type
// and the JSON-encoded body. This is the non-streaming (synchronous) path.
//
// Per MCP 2025-03-26 Streamable HTTP spec (https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#streamable-http):
// "If the server's response is not streaming, it SHOULD use Content-Type: application/json"
func TestStreamable_SingleResponse(t *testing.T) {
	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		return SingleResponse{
			Body: map[string]any{"result": "ok", "code": 200},
		}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Expected application/json Content-Type, got %q", ct)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	if body["result"] != "ok" {
		t.Errorf("Expected result=ok, got %v", body["result"])
	}
}

// TestStreamable_SingleResponse_CustomStatus verifies that SingleResponse
// respects custom HTTP status codes (e.g., 201 Created, 202 Accepted).
//
// Per MCP 2025-03-26 spec, the server may return 202 Accepted for
// notifications/responses that require no reply.
func TestStreamable_SingleResponse_CustomStatus(t *testing.T) {
	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		return SingleResponse{
			StatusCode: 202,
			Body:       map[string]any{"status": "accepted"},
		}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		t.Errorf("Expected 202, got %d", resp.StatusCode)
	}
}

// TestStreamable_StreamResponse verifies that when the handler returns a
// StreamResponse, the server responds with text/event-stream Content-Type
// and streams SSE events from the Events channel.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html):
// each event is terminated by a blank line, with data in "data:" fields.
func TestStreamable_StreamResponse(t *testing.T) {
	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		ch := make(chan SSEEvent, 1)
		go func() {
			ch <- SSEEvent{Data: map[string]any{"msg": "hello"}}
			close(ch)
		}()
		return StreamResponse{Events: ch}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Expected text/event-stream, got %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read SSE event: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
		t.Fatalf("Failed to parse event data: %v", err)
	}
	if data["msg"] != "hello" {
		t.Errorf("Expected msg=hello, got %v", data["msg"])
	}
}

// TestStreamable_StreamResponse_MultipleEvents verifies that multiple SSE
// events are delivered in order when the handler sends them through the
// Events channel.
func TestStreamable_StreamResponse_MultipleEvents(t *testing.T) {
	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		ch := make(chan SSEEvent, 3)
		go func() {
			for i := 1; i <= 3; i++ {
				ch <- SSEEvent{Data: map[string]any{"seq": i}}
			}
			close(ch)
		}()
		return StreamResponse{Events: ch}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	for i := 1; i <= 3; i++ {
		ev, err := readSSEEvent(t, reader, 2*time.Second)
		if err != nil {
			t.Fatalf("Failed to read event %d: %v", i, err)
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
			t.Fatalf("Failed to parse event %d: %v", i, err)
		}
		if int(data["seq"].(float64)) != i {
			t.Errorf("Event %d: expected seq=%d, got %v", i, i, data["seq"])
		}
	}
}

// TestStreamable_StreamResponse_EventTypes verifies that SSE events with
// named event types have the "event:" field set correctly.
//
// Per WHATWG SSE spec, clients use EventSource.addEventListener(type, handler)
// to listen for named event types.
func TestStreamable_StreamResponse_EventTypes(t *testing.T) {
	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		ch := make(chan SSEEvent, 1)
		go func() {
			ch <- SSEEvent{Event: "progress", Data: map[string]any{"pct": 50}}
			close(ch)
		}()
		return StreamResponse{Events: ch}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read event: %v", err)
	}
	if ev.Event != "progress" {
		t.Errorf("Expected event=progress, got %q", ev.Event)
	}
}

// TestStreamable_StreamResponse_EventWithID verifies that SSE events with
// an ID have the "id:" field set correctly, enabling client-side
// reconnection via the Last-Event-ID header.
//
// Per WHATWG SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#the-last-event-id-string).
func TestStreamable_StreamResponse_EventWithID(t *testing.T) {
	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		ch := make(chan SSEEvent, 1)
		go func() {
			ch <- SSEEvent{Event: "result", ID: "req-42", Data: map[string]any{"done": true}}
			close(ch)
		}()
		return StreamResponse{Events: ch}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	ev, err := readSSEEvent(t, reader, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to read event: %v", err)
	}
	if ev.ID != "req-42" {
		t.Errorf("Expected id=req-42, got %q", ev.ID)
	}
}

// TestStreamable_StreamResponse_ClientDisconnect verifies that the handler's
// context is cancelled when the client disconnects (closes the response body).
// Go's net/http server cancels the request context when the client TCP
// connection closes (per net/http.Request.Context() documentation).
func TestStreamable_StreamResponse_ClientDisconnect(t *testing.T) {
	ctxCancelled := make(chan struct{})

	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		ch := make(chan SSEEvent)
		go func() {
			// Block until context is cancelled (client disconnect)
			<-ctx.Done()
			close(ctxCancelled)
			close(ch)
		}()
		return StreamResponse{Events: ch}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Close body to simulate client disconnect
	resp.Body.Close()

	select {
	case <-ctxCancelled:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for context cancellation on client disconnect")
	}
}

// TestStreamable_NilConfig verifies that StreamableServe works with nil
// config, using the default JSONCodec for serialization.
func TestStreamable_NilConfig(t *testing.T) {
	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		return SingleResponse{Body: map[string]any{"ok": true}}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/rpc")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("Expected body to contain 'ok', got: %s", body)
	}
}

// TestStreamable_POSTMethod verifies that StreamableServe works correctly
// with POST requests, the primary method for the Streamable HTTP pattern
// (MCP 2025-03-26).
func TestStreamable_POSTMethod(t *testing.T) {
	handler := StreamableServe(func(ctx context.Context, r *http.Request) StreamableResponse {
		return SingleResponse{
			Body: map[string]any{"method": r.Method},
		}
	}, nil)

	router := mux.NewRouter()
	router.HandleFunc("/rpc", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{"jsonrpc":"2.0"}`))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["method"] != "POST" {
		t.Errorf("Expected method=POST, got %v", body["method"])
	}
}
