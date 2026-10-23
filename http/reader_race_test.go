package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// TestWebSocketReaderRace_RapidConnectDisconnect rapidly connects and
// disconnects WebSocket clients to stress the conc.Reader teardown path.
// Regression test for https://github.com/panyam/gocurrent/issues/4, where
// Reader.cleanup() raced the reader goroutine on its message channel during
// connection close. Fixed in gocurrent v0.0.12; CI runs this under -race.
func TestWebSocketReaderRace_RapidConnectDisconnect(t *testing.T) {
	router := mux.NewRouter()
	handler := &EchoHandler{}

	// Use short ping/pong intervals to increase teardown activity
	config := &WSConnConfig{
		BiDirStreamConfig: &BiDirStreamConfig{
			PingPeriod: 10 * time.Millisecond,
			PongPeriod: 50 * time.Millisecond,
		},
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	router.HandleFunc("/ws", WSServe(handler, config))
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Rapidly connect, send a message, and disconnect
	const iterations = 50
	for i := 0; i < iterations; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Iteration %d: failed to connect: %v", i, err)
		}

		// Send a message to ensure the reader goroutine is active
		conn.WriteJSON(map[string]any{"type": "test", "i": i})

		// Read the echo response (or timeout)
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		var msg map[string]any
		conn.ReadJSON(&msg) // ignore error — we just want the reader active

		// Close immediately — this triggers Reader.Stop() racing with
		// the reader goroutine's channel send
		conn.Close()
	}
}
