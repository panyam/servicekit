package http

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type closeEvent struct {
	info   ConnInfo
	reason CloseReason
}

type recordingObserver struct {
	mu       sync.Mutex
	opened   []ConnInfo
	closed   []closeEvent
	sent     map[string]int
	received int
	closedCh chan struct{}
}

func newRecordingObserver() *recordingObserver {
	return &recordingObserver{sent: map[string]int{}, closedCh: make(chan struct{}, 100)}
}

func (o *recordingObserver) ConnOpened(info ConnInfo) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opened = append(o.opened, info)
}

func (o *recordingObserver) ConnClosed(info ConnInfo, reason CloseReason, lifetime time.Duration) {
	o.mu.Lock()
	o.closed = append(o.closed, closeEvent{info, reason})
	o.mu.Unlock()
	o.closedCh <- struct{}{}
}

func (o *recordingObserver) MessageSent(info ConnInfo, kind string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent[kind]++
}

func (o *recordingObserver) MessageReceived(info ConnInfo) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.received++
}

// waitClose waits for one ConnClosed and returns every close seen so far.
func (o *recordingObserver) waitClose(t *testing.T) []closeEvent {
	t.Helper()
	select {
	case <-o.closedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("ConnClosed was not called")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]closeEvent(nil), o.closed...)
}

func expectOneClose(t *testing.T, o *recordingObserver, want closeEvent) {
	t.Helper()
	got := o.waitClose(t)
	time.Sleep(50 * time.Millisecond) // a second ConnClosed would land here
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.closed) != 1 || got[0] != want {
		t.Fatalf("closes = %+v, want exactly [%+v]", o.closed, want)
	}
	if len(o.opened) != 1 || o.opened[0] != want.info {
		t.Fatalf("opens = %+v, want exactly [%+v]", o.opened, want.info)
	}
}

func TestObserver_WSClientCloseCountsFrames(t *testing.T) {
	obs := newRecordingObserver()
	conn := &closeTestConn{}
	conn.Observer = obs
	client, cleanup := newCloseTestServer(t, conn)
	defer cleanup()

	if err := client.WriteJSON(map[string]any{"hi": 1}); err != nil {
		t.Fatal(err)
	}
	conn.SendOutput(any("hello"))
	if _, _, err := client.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	client.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))

	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "ws"}, CloseClient})
	obs.mu.Lock()
	defer obs.mu.Unlock()
	if obs.received != 1 || obs.sent["data"] != 1 {
		t.Fatalf("received=%d sent=%v, want 1 received and 1 data sent", obs.received, obs.sent)
	}
}

func TestObserver_WriteTimeoutReportsWriteTimeout(t *testing.T) {
	obs := newRecordingObserver()
	conn := &closeTestConn{}
	conn.Observer = obs
	conn.WriteTimeout = 200 * time.Millisecond
	_, cleanup := newCloseTestServer(t, conn)
	defer cleanup()

	floodUntilClosed(conn)
	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "ws"}, CloseWriteTimeout})
}

func TestObserver_CloseAfterSendReportsServerClose(t *testing.T) {
	obs := newRecordingObserver()
	conn := &closeTestConn{}
	conn.Observer = obs
	conn.Info = ConnInfo{Transport: "grpcws", Mode: "bidi"}
	client, cleanup := newCloseTestServer(t, conn)
	defer cleanup()

	conn.CloseAfterSend(websocket.CloseNormalClosure, "done", time.Second, any("last"))
	for {
		if _, _, err := client.ReadMessage(); err != nil {
			break
		}
	}
	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "grpcws", Mode: "bidi"}, CloseServer})
	obs.mu.Lock()
	defer obs.mu.Unlock()
	if obs.sent["close"] != 1 {
		t.Fatalf("sent = %v, want one close frame", obs.sent)
	}
}

func TestObserver_PongTimeout(t *testing.T) {
	obs := newRecordingObserver()
	conn := &closeTestConn{}
	conn.Codec = &JSONCodec{}
	conn.started = make(chan struct{})
	conn.closed = make(chan struct{})
	config := DefaultWSConnConfig()
	config.BiDirStreamConfig = &BiDirStreamConfig{PingPeriod: time.Hour, PongPeriod: 100 * time.Millisecond}
	config.Observer = obs
	server := httptest.NewServer(WSServe(&closeTestHandler{conn: conn}, config))
	defer server.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "ws"}, ClosePongTimeout})
}

// The config-level observer reaches connections that set none, and does not
// replace one a connection set itself.
func TestObserver_ConfigObserverIsADefault(t *testing.T) {
	for _, own := range []bool{false, true} {
		cfgObs, connObs := newRecordingObserver(), newRecordingObserver()
		conn := &closeTestConn{}
		conn.Codec = &JSONCodec{}
		conn.started = make(chan struct{})
		conn.closed = make(chan struct{})
		if own {
			conn.Observer = connObs
		}
		config := DefaultWSConnConfig()
		config.Observer = cfgObs
		server := httptest.NewServer(WSServe(&closeTestHandler{conn: conn}, config))
		client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		client.Close()
		want, other := cfgObs, connObs
		if own {
			want, other = connObs, cfgObs
		}
		expectOneClose(t, want, closeEvent{ConnInfo{Transport: "ws"}, CloseClient})
		if len(other.opened) != 0 {
			t.Fatalf("own=%v: the other observer saw %d opens", own, len(other.opened))
		}
		server.Close()
	}
}

func TestObserver_StartFailedIsReported(t *testing.T) {
	obs := newRecordingObserver()
	conn := &failingStartConn{}
	conn.Codec = &JSONCodec{}
	conn.Observer = obs
	server := httptest.NewServer(WSServe(&staticHandler[*failingStartConn]{conn}, nil))
	defer server.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "ws"}, CloseStartFailed})
}

type failingStartConn struct{ JSONConn }

func (c *failingStartConn) OnStart(conn *websocket.Conn) error {
	c.JSONConn.OnStart(conn)
	return errors.New("no")
}

type staticHandler[S any] struct{ conn S }

func (h *staticHandler[S]) Validate(w http.ResponseWriter, r *http.Request) (S, bool) {
	return h.conn, true
}

// A write failure outranks a server close that was already under way: a
// client stuck behind CloseAfterSend is a dropped client.
func TestCloseReasonRanking(t *testing.T) {
	cases := []struct {
		notes []CloseReason
		want  CloseReason
	}{
		{nil, CloseClient},
		{[]CloseReason{CloseServer}, CloseServer},
		{[]CloseReason{CloseServer, CloseWriteTimeout}, CloseWriteTimeout},
		{[]CloseReason{CloseWriteTimeout, CloseServer}, CloseWriteTimeout},
		{[]CloseReason{ClosePongTimeout, CloseServer}, CloseServer},
		{[]CloseReason{CloseWriteError, CloseWriteTimeout}, CloseWriteError},
	}
	for _, c := range cases {
		obs := newRecordingObserver()
		var s connStats
		s.open(obs, ConnInfo{})
		for _, r := range c.notes {
			s.noteReason(r)
		}
		s.close(obs, ConnInfo{}, CloseClient)
		s.close(obs, ConnInfo{}, CloseClient)
		if len(obs.closed) != 1 || obs.closed[0].reason != c.want {
			t.Errorf("notes %v: closes = %+v, want one %q", c.notes, obs.closed, c.want)
		}
	}
}

func serveObservedSSE(t *testing.T, obs ConnObserver) (*NotifierSSEConn, *http.Response, context.CancelFunc) {
	t.Helper()
	connCh := make(chan *NotifierSSEConn, 1)
	config := DefaultSSEConnConfig()
	config.KeepalivePeriod = 0
	config.Observer = obs
	server := httptest.NewServer(SSEServe[any](&NotifierSSEHandler{connChan: connCh}, config))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return waitForSSEConn(t, connCh), resp, cancel
}

func TestObserver_SSEClientDisconnect(t *testing.T) {
	obs := newRecordingObserver()
	conn, resp, cancel := serveObservedSSE(t, obs)
	conn.SendEvent("tick", any(1))
	conn.SendKeepalive()
	reader := bufio.NewReader(resp.Body)
	if _, err := readSSEEvent(t, reader, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	cancel()
	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "sse"}, CloseClient})
	obs.mu.Lock()
	defer obs.mu.Unlock()
	if obs.sent["data"] != 1 || obs.sent["comment"] != 1 {
		t.Fatalf("sent = %v, want one data and one comment", obs.sent)
	}
}

func TestObserver_SSEServerClose(t *testing.T) {
	obs := newRecordingObserver()
	conn, _, cancel := serveObservedSSE(t, obs)
	defer cancel()
	conn.Close()
	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "sse"}, CloseServer})
}

type failingStartSSEConn struct{ BaseSSEConn[any] }

func (c *failingStartSSEConn) OnStart(w http.ResponseWriter, r *http.Request) error {
	c.BaseSSEConn.OnStart(w, r)
	return errors.New("no")
}

// SSEServe skips OnClose when OnStart fails, so the observer would otherwise
// count the connection as open forever.
func TestObserver_SSEStartFailedIsReported(t *testing.T) {
	obs := newRecordingObserver()
	conn := &failingStartSSEConn{BaseSSEConn[any]{Codec: &JSONCodec{}, Observer: obs}}
	server := httptest.NewServer(SSEServe[any](&staticHandler[*failingStartSSEConn]{conn}, nil))
	defer server.Close()
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "sse"}, CloseStartFailed})
}

// closeOnCloseConn starts a server close from OnClose, the way cancelling a
// grpcws stream ends it and calls CloseAfterSend.
type closeOnCloseConn struct{ closeTestConn }

func (c *closeOnCloseConn) OnClose() {
	c.CloseAfterSend(websocket.CloseNormalClosure, "late", time.Second)
	c.closeTestConn.OnClose()
}

// A close set off by tearing the connection down must not replace the
// reason the read loop ended with.
func TestObserver_CloseDuringTeardownKeepsClientReason(t *testing.T) {
	obs := newRecordingObserver()
	conn := &closeOnCloseConn{}
	conn.Observer = obs
	conn.Codec = &JSONCodec{}
	conn.started = make(chan struct{})
	conn.closed = make(chan struct{})
	server := httptest.NewServer(WSServe(&staticHandler[*closeOnCloseConn]{conn}, nil))
	defer server.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	<-conn.started
	client.Close()
	expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "ws"}, CloseClient})
}

// A client that closes while the server is busy writing to it closed the
// connection. Writes that fail with ErrCloseSent after gorilla answered the
// close frame are not write errors.
func TestObserver_ClientCloseDuringFloodIsClientClose(t *testing.T) {
	for i := 0; i < 5; i++ {
		obs := newRecordingObserver()
		conn := &closeTestConn{}
		conn.Observer = obs
		client, cleanup := newCloseTestServer(t, conn)
		go func() {
			for {
				select {
				case <-conn.closed:
					return
				default:
					conn.SendOutput(any("tick"))
				}
			}
		}()
		client.ReadMessage()
		client.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				break
			}
		}
		expectOneClose(t, obs, closeEvent{ConnInfo{Transport: "ws"}, CloseClient})
		cleanup()
	}
}
