package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/panyam/servicekit/internal/loadtest"
)

// Load tests for issue 13. They skip under -short and must pass with
// -race -count=5 (make test-race).

func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("load test; skipped with -short")
	}
}

// room is a broadcast server: every connection it accepts gets every
// message sent to the room, and echoes back whatever it reads.
type room struct {
	mu    sync.Mutex
	conns map[*roomConn]bool
	// joined is signalled once per connection that has started.
	joined chan struct{}

	writeTimeout time.Duration
	observer     ConnObserver
}

type roomConn struct {
	JSONConn
	room *room
}

func (c *roomConn) OnStart(conn *websocket.Conn) error {
	if err := c.JSONConn.OnStart(conn); err != nil {
		return err
	}
	c.room.mu.Lock()
	c.room.conns[c] = true
	c.room.mu.Unlock()
	c.room.joined <- struct{}{}
	return nil
}

func (c *roomConn) HandleMessage(msg any) error {
	c.SendOutput(msg)
	return nil
}

func (c *roomConn) OnClose() {
	c.room.mu.Lock()
	delete(c.room.conns, c)
	c.room.mu.Unlock()
	c.JSONConn.OnClose()
}

func (r *room) Validate(w http.ResponseWriter, req *http.Request) (*roomConn, bool) {
	c := &roomConn{room: r}
	c.Codec = &JSONCodec{}
	c.WriteTimeout = r.writeTimeout
	c.Observer = r.observer
	return c, true
}

func (r *room) broadcast(msg any) {
	r.mu.Lock()
	conns := make([]*roomConn, 0, len(r.conns))
	for c := range r.conns {
		conns = append(conns, c)
	}
	r.mu.Unlock()
	for _, c := range conns {
		c.SendOutput(msg)
	}
}

// broadcastEach sends to every connection from its own goroutine, the way a
// relay fans out, so one blocked client holds up only its own sends.
func (r *room) broadcastEach(msgs []any) *sync.WaitGroup {
	r.mu.Lock()
	defer r.mu.Unlock()
	var wg sync.WaitGroup
	for c := range r.conns {
		wg.Add(1)
		go func(c *roomConn) {
			defer wg.Done()
			for _, m := range msgs {
				c.SendOutput(m)
			}
		}(c)
	}
	return &wg
}

func newRoom() *room {
	return &room{conns: map[*roomConn]bool{}, joined: make(chan struct{}, 1000)}
}

func serveRoom(t *testing.T, r *room) string {
	t.Helper()
	server := httptest.NewServer(WSServe(r, nil))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func dialPool(t *testing.T, r *room, url string, clients, stuck int) *loadtest.Pool {
	t.Helper()
	pool, err := loadtest.Dial(context.Background(), url, loadtest.Options{Clients: clients, Stuck: stuck})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for i := 0; i < clients; i++ {
		select {
		case <-r.joined:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d connections started", i, clients)
		}
	}
	return pool
}

// waitGoroutines waits for the goroutine count to come back near base.
// Connections finish closing on server goroutines after the client side is
// gone, so the count takes a moment to settle.
func waitGoroutines(t *testing.T, base int) {
	t.Helper()
	const slack = 10
	deadline := time.Now().Add(5 * time.Second)
	for {
		n := runtime.NumGoroutine()
		if n <= base+slack {
			return
		}
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			buf = buf[:runtime.Stack(buf, true)]
			t.Fatalf("goroutines: %d now, %d before\n%s", n, base, buf)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWSLoad_100Connections(t *testing.T) {
	skipIfShort(t)
	r := newRoom()
	pool := dialPool(t, r, serveRoom(t, r), 100, 0)

	if err := pool.SendAll([]byte(`{"n":1}`), 10, false); err != nil {
		t.Fatal(err)
	}
	if err := pool.WaitEach(10, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := pool.Received(); got != 1000 {
		t.Fatalf("received %d echoes, want 1000", got)
	}
}

func TestWSLoad_RapidConnectDisconnect(t *testing.T) {
	skipIfShort(t)
	obs := newRecordingObserver()
	obs.closedCh = make(chan struct{}, 1000)
	r := newRoom()
	r.observer = obs
	url := serveRoom(t, r)
	base := runtime.NumGoroutine()

	const cycles = 500
	for i := 0; i < cycles; i += 50 {
		pool := dialPool(t, r, url, 50, 0)
		pool.Close()
	}
	for i := 0; i < cycles; i++ {
		select {
		case <-obs.closedCh:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d connections reported closed", i, cycles)
		}
	}
	waitGoroutines(t, base)
	obs.mu.Lock()
	defer obs.mu.Unlock()
	if len(obs.opened) != cycles {
		t.Fatalf("opened %d, want %d", len(obs.opened), cycles)
	}
}

func TestWSLoad_ConcurrentBroadcast(t *testing.T) {
	skipIfShort(t)
	r := newRoom()
	pool := dialPool(t, r, serveRoom(t, r), 50, 0)

	for i := range 100 {
		r.broadcast(map[string]any{"i": i})
	}
	if err := pool.WaitEach(100, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := pool.Received(); got != 50*100 {
		t.Fatalf("received %d, want %d", got, 50*100)
	}
}

// One client stops reading while the room broadcasts large frames. With a
// WriteTimeout it is dropped and reported as write_timeout, and the others
// get every message.
func TestWSLoad_SlowConsumer(t *testing.T) {
	skipIfShort(t)
	obs := newRecordingObserver()
	r := newRoom()
	r.writeTimeout = 300 * time.Millisecond
	r.observer = obs
	pool := dialPool(t, r, serveRoom(t, r), 10, 1)

	// 40 x 256KB is far more than loopback buffers hold for a client that
	// never reads (see the stuck-client gotcha in CLAUDE.md).
	big := strings.Repeat("x", 256<<10)
	msgs := make([]any, 40)
	for i := range msgs {
		msgs[i] = fmt.Sprintf("%d:%s", i, big)
	}
	start := time.Now()
	sent := make(chan struct{})
	go func() { r.broadcastEach(msgs).Wait(); close(sent) }()
	select {
	case <-sent:
	case <-time.After(10 * time.Second):
		t.Fatal("broadcast to the stuck client never returned")
	}

	if err := pool.WaitEach(int64(len(msgs)), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	got := obs.waitClose(t)
	if len(got) != 1 || got[0].reason != CloseWriteTimeout {
		t.Fatalf("closes = %+v, want one write_timeout", got)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("broadcast took %v; the stuck client held up the others", elapsed)
	}
}
