package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// closeTestConn is a JSONConn that reports when OnClose runs, so tests can
// tell that WSHandleConn has exited.
type closeTestConn struct {
	JSONConn
	swallowErrors bool
	started       chan struct{}
	closed        chan struct{}
}

func (c *closeTestConn) OnStart(conn *websocket.Conn) error {
	err := c.JSONConn.OnStart(conn)
	close(c.started)
	return err
}

func (c *closeTestConn) OnError(err error) error {
	if c.swallowErrors {
		return nil
	}
	return err
}

func (c *closeTestConn) OnClose() {
	c.JSONConn.OnClose()
	close(c.closed)
}

type closeTestHandler struct {
	conn *closeTestConn
}

func (h *closeTestHandler) Validate(w http.ResponseWriter, r *http.Request) (*closeTestConn, bool) {
	return h.conn, true
}

func newCloseTestServer(t *testing.T, conn *closeTestConn) (*websocket.Conn, func()) {
	t.Helper()
	conn.Codec = &JSONCodec{}
	conn.NameStr = "closeTestConn"
	conn.started = make(chan struct{})
	conn.closed = make(chan struct{})
	server := httptest.NewServer(WSServe(&closeTestHandler{conn: conn}, nil))
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial: %v", err)
	}
	select {
	case <-conn.started:
	case <-time.After(2 * time.Second):
		t.Fatal("OnStart never ran")
	}
	return client, func() {
		client.Close()
		server.Close()
	}
}

func waitClosed(t *testing.T, conn *closeTestConn, timeout time.Duration) {
	t.Helper()
	select {
	case <-conn.closed:
	case <-time.After(timeout):
		t.Fatalf("OnClose did not run within %v", timeout)
	}
}

// floodUntilClosed sends 1MB messages until the server conn closes. A client
// that never reads fills the TCP window within a few messages, after which
// the Writer is blocked inside a write.
func floodUntilClosed(conn *closeTestConn) {
	big := strings.Repeat("x", 1<<20)
	go func() {
		for {
			select {
			case <-conn.closed:
				return
			default:
				conn.SendOutput(any(big))
			}
		}
	}()
}

func TestCloseAfterSend_DeliversLastMessagesThenNormalClose(t *testing.T) {
	conn := &closeTestConn{}
	client, cleanup := newCloseTestServer(t, conn)
	defer cleanup()

	conn.CloseAfterSend(websocket.CloseNormalClosure, "done", 2*time.Second, any("a"), any("b"))

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	for _, want := range []string{"a", "b"} {
		var got string
		if err := client.ReadJSON(&got); err != nil {
			t.Fatalf("reading %q: %v", want, err)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	_, _, err := client.ReadMessage()
	var ce *websocket.CloseError
	if !errors.As(err, &ce) {
		t.Fatalf("expected a close frame after the last message, got %v", err)
	}
	if ce.Code != websocket.CloseNormalClosure || ce.Text != "done" {
		t.Fatalf("got close %d %q, want 1000 \"done\"", ce.Code, ce.Text)
	}
	waitClosed(t, conn, 2*time.Second)
}

func TestCloseAfterSend_HardClosesStuckClientAfterGrace(t *testing.T) {
	conn := &closeTestConn{}
	_, cleanup := newCloseTestServer(t, conn)
	defer cleanup()

	floodUntilClosed(conn)
	time.Sleep(300 * time.Millisecond)

	go conn.CloseAfterSend(websocket.CloseNormalClosure, "done", 200*time.Millisecond, any("last"))
	waitClosed(t, conn, 3*time.Second)
}

func TestWriteTimeout_ClosesStuckClient(t *testing.T) {
	conn := &closeTestConn{}
	conn.WriteTimeout = 200 * time.Millisecond
	_, cleanup := newCloseTestServer(t, conn)
	defer cleanup()

	floodUntilClosed(conn)
	waitClosed(t, conn, 3*time.Second)
}

// A clean close from the client ends the connection right away, including
// when OnError swallows errors, since there is nothing left to read.
func TestWSHandleConn_ClientCloseEndsConn(t *testing.T) {
	for _, swallow := range []bool{false, true} {
		t.Run(fmt.Sprintf("swallowErrors=%v", swallow), func(t *testing.T) {
			conn := &closeTestConn{swallowErrors: swallow}
			client, cleanup := newCloseTestServer(t, conn)
			defer cleanup()

			msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")
			if err := client.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second)); err != nil {
				t.Fatalf("write close: %v", err)
			}
			waitClosed(t, conn, 2*time.Second)
		})
	}
}
