package http

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type hubSSEConn struct {
	BaseSSEConn[any]
	hub    *SSEHub[any]
	joined chan struct{}
}

func (c *hubSSEConn) OnStart(w http.ResponseWriter, r *http.Request) error {
	if err := c.BaseSSEConn.OnStart(w, r); err != nil {
		return err
	}
	c.hub.Register(&c.BaseSSEConn)
	c.joined <- struct{}{}
	return nil
}

func (c *hubSSEConn) OnClose() {
	c.hub.Unregister(c.ConnID())
	c.BaseSSEConn.OnClose()
}

type hubSSEHandler struct {
	hub    *SSEHub[any]
	joined chan struct{}
}

func (h *hubSSEHandler) Validate(w http.ResponseWriter, r *http.Request) (*hubSSEConn, bool) {
	return &hubSSEConn{
		BaseSSEConn: BaseSSEConn[any]{Codec: &JSONCodec{}},
		hub:         h.hub,
		joined:      h.joined,
	}, true
}

// sseClient counts the data events on one stream until its context ends.
func sseClient(ctx context.Context, url string, events *atomic.Int64) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data: ") {
				events.Add(1)
			}
		}
	}()
	return nil
}

func TestSSELoad_100Connections(t *testing.T) {
	skipIfShort(t)
	const clients, events = 100, 50
	hub := NewSSEHub[any]()
	h := &hubSSEHandler{hub: hub, joined: make(chan struct{}, clients)}
	config := DefaultSSEConnConfig()
	config.KeepalivePeriod = 0
	server := httptest.NewServer(SSEServe[any](h, config))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	counts := make([]atomic.Int64, clients)
	for i := range counts {
		if err := sseClient(ctx, server.URL, &counts[i]); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < clients; i++ {
		select {
		case <-h.joined:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d SSE connections started", i, clients)
		}
	}

	for i := range events {
		hub.Broadcast(map[string]any{"i": i})
	}
	deadline := time.Now().Add(10 * time.Second)
	for i := range counts {
		for counts[i].Load() < events {
			if time.Now().After(deadline) {
				t.Fatalf("client %d got %d of %d events", i, counts[i].Load(), events)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// Connections join and leave the hub while it broadcasts. The point is -race
// and no deadlock; there is nothing to count.
func TestSSELoad_HubChurn(t *testing.T) {
	skipIfShort(t)
	hub := NewSSEHub[any]()
	h := &hubSSEHandler{hub: hub, joined: make(chan struct{}, 1000)}
	config := DefaultSSEConnConfig()
	config.KeepalivePeriod = 0
	server := httptest.NewServer(SSEServe[any](h, config))
	defer server.Close()

	stop := make(chan struct{})
	var broadcasts sync.WaitGroup
	broadcasts.Add(1)
	go func() {
		defer broadcasts.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				hub.Broadcast(i)
			}
		}
	}()

	// At most 32 dials in flight, under the default listen backlog on macOS.
	sem := make(chan struct{}, 32)
	var clients sync.WaitGroup
	for range 200 {
		clients.Add(1)
		sem <- struct{}{}
		go func() {
			defer clients.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithCancel(context.Background())
			var n atomic.Int64
			if err := sseClient(ctx, server.URL, &n); err != nil {
				t.Error(err)
			}
			time.Sleep(5 * time.Millisecond)
			cancel()
		}()
	}
	clients.Wait()
	close(stop)
	broadcasts.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for hub.Count() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d connections still registered after every client left", hub.Count())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
