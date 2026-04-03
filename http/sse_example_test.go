package http_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	gohttp "github.com/panyam/servicekit/http"
)

// ExampleSSEServe demonstrates setting up an SSE endpoint using SSEServe.
// The handler creates a connection that pushes events to the client.
// SSE endpoints require http.Server.WriteTimeout = 0 for long-lived streams.
func ExampleSSEServe() {
	router := mux.NewRouter()
	router.HandleFunc("/events", gohttp.SSEServe[any](&gohttp.JSONSSEHandler{}, nil))

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		WriteTimeout: 0, // Required for SSE!
	}
	log.Fatal(srv.ListenAndServe())
}

// ExampleSSEHub demonstrates using SSEHub for broadcasting events to
// multiple SSE connections. The hub tracks connections by ID and supports
// targeted delivery and broadcast.
func ExampleSSEHub() {
	hub := gohttp.NewSSEHub[any]()

	// Broadcast to all connected clients
	hub.Broadcast(map[string]any{"type": "alert", "msg": "hello"})

	// Send to a specific client
	hub.Send("session-123", map[string]any{"type": "direct"})

	// Send a named event
	hub.BroadcastEvent("notification", map[string]any{"text": "update"})

	// On shutdown, close all connections
	hub.CloseAll()

	fmt.Println("Hub operations complete")
	// Output: Hub operations complete
}

// ExampleListenAndServeGraceful demonstrates graceful shutdown with
// signal handling and SSEHub integration. OnShutdown callbacks run
// before drain, so they can send goodbye events while connections
// are still open.
func ExampleListenAndServeGraceful() {
	hub := gohttp.NewSSEHub[any]()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: http.DefaultServeMux,
	}

	// Blocks until SIGTERM/SIGINT, then:
	// 1. Calls hub.CloseAll() (closes SSE connections)
	// 2. Drains HTTP requests for up to 10s
	// 3. Returns nil
	err := gohttp.ListenAndServeGraceful(srv,
		gohttp.WithDrainTimeout(10*time.Second),
		gohttp.WithOnShutdown(hub.CloseAll),
	)
	if err != nil {
		log.Fatal(err)
	}
}

// ExampleStreamableServe demonstrates the POST-that-optionally-streams
// pattern from MCP 2025-03-26. The handler decides per-request whether
// to return a JSON response or an SSE event stream.
func ExampleStreamableServe() {
	handler := gohttp.StreamableServe(
		func(ctx context.Context, r *http.Request) gohttp.StreamableResponse {
			// Check if client wants streaming
			if r.Header.Get("Accept") == "text/event-stream" {
				ch := make(chan gohttp.SSEEvent)
				go func() {
					defer close(ch)
					ch <- gohttp.SSEEvent{Event: "progress", Data: map[string]any{"pct": 50}}
					ch <- gohttp.SSEEvent{Event: "result", Data: map[string]any{"done": true}}
				}()
				return gohttp.StreamResponse{Events: ch}
			}
			// Default: synchronous JSON response
			return gohttp.SingleResponse{Body: map[string]any{"result": "ok"}}
		},
		nil, // default config (JSONCodec)
	)

	http.HandleFunc("/rpc", handler)
	fmt.Println("Streamable handler registered")
	// Output: Streamable handler registered
}
