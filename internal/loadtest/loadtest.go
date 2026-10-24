// Package loadtest drives many WebSocket clients at a server, some of which
// stop reading, and counts what each one saw. The load tests in http/ and
// the observable_relay exercise (cmd/grpcws-demo -exercise) both use it.
//
// It speaks plain WebSocket and imports nothing from servicekit, so tests
// inside package http can use it without an import cycle.
package loadtest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Client is one connection in a Pool.
type Client struct {
	Conn *websocket.Conn

	// Stuck clients never read. Once their TCP window fills, the server's
	// writes to them block, which is what WriteTimeout exists for.
	Stuck bool

	received atomic.Int64
	count    func([]byte) bool
	ended    chan struct{}
	endErr   error
}

// Received is how many data frames this client has read (those that pass
// Options.Count, if set).
func (c *Client) Received() int64 { return c.received.Load() }

// Ended is closed when the client's read loop stops, because the server
// closed the connection or Pool.Close did. It is never closed for a stuck
// client, which has no read loop.
func (c *Client) Ended() <-chan struct{} { return c.ended }

// Pool is a set of clients dialled together.
type Pool struct {
	Clients []*Client
}

// Options configures Dial.
type Options struct {
	// Clients is how many connections to open, and Stuck how many of those
	// (the last ones) never read.
	Clients, Stuck int

	// First, if set, is written by every client right after it connects
	// (a grpcws request, a join message).
	First []byte

	// URLFor, if set, gives client i its own URL (a player id in the
	// query, say) instead of the one passed to Dial.
	URLFor func(i int) string

	// Count, if set, picks which frames Received counts. Nil counts all.
	Count func(frame []byte) bool

	// Parallel bounds concurrent dials. Zero means 32, which stays under
	// the default listen backlog on macOS.
	Parallel int

	Dialer *websocket.Dialer
}

// Dial opens the pool. On any dial error it closes what it opened and
// returns the error.
func Dial(ctx context.Context, url string, opts Options) (*Pool, error) {
	if opts.Stuck > opts.Clients {
		return nil, fmt.Errorf("stuck (%d) > clients (%d)", opts.Stuck, opts.Clients)
	}
	parallel := opts.Parallel
	if parallel <= 0 {
		parallel = 32
	}
	dialer := opts.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}

	p := &Pool{Clients: make([]*Client, opts.Clients)}
	sem := make(chan struct{}, parallel)
	errs := make(chan error, opts.Clients)
	var wg sync.WaitGroup
	for i := range p.Clients {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			u := url
			if opts.URLFor != nil {
				u = opts.URLFor(i)
			}
			conn, _, err := dialer.DialContext(ctx, u, nil)
			if err != nil {
				errs <- fmt.Errorf("client %d: %w", i, err)
				return
			}
			if opts.First != nil {
				if err := conn.WriteMessage(websocket.TextMessage, opts.First); err != nil {
					conn.Close()
					errs <- fmt.Errorf("client %d first message: %w", i, err)
					return
				}
			}
			c := &Client{Conn: conn, Stuck: i >= opts.Clients-opts.Stuck, count: opts.Count, ended: make(chan struct{})}
			if !c.Stuck {
				go c.readLoop()
			}
			p.Clients[i] = c
		}(i)
	}
	wg.Wait()
	close(errs)
	if err := <-errs; err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func (c *Client) readLoop() {
	defer close(c.ended)
	for {
		_, frame, err := c.Conn.ReadMessage()
		if err != nil {
			c.endErr = err
			return
		}
		if c.count == nil || c.count(frame) {
			c.received.Add(1)
		}
	}
}

// Readers returns the clients that read.
func (p *Pool) Readers() []*Client {
	var out []*Client
	for _, c := range p.Clients {
		if c != nil && !c.Stuck {
			out = append(out, c)
		}
	}
	return out
}

// Received is the total data frames read across the pool.
func (p *Pool) Received() int64 {
	var n int64
	for _, c := range p.Readers() {
		n += c.Received()
	}
	return n
}

// Slowest is the lowest Received among readers still connected, and false
// if none are.
func (p *Pool) Slowest() (int64, bool) {
	var low int64
	found := false
	for _, c := range p.Readers() {
		select {
		case <-c.ended:
			continue
		default:
		}
		if n := c.Received(); !found || n < low {
			low, found = n, true
		}
	}
	return low, found
}

// WaitEach waits until every reader has read at least n frames.
func (p *Pool) WaitEach(n int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		short := 0
		for _, c := range p.Readers() {
			if c.Received() < n {
				short++
			}
		}
		if short == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d of %d readers read fewer than %d frames", short, len(p.Readers()), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// SendAll writes msg from every reader, times times. Stuck clients write too
// unless skipStuck, since a client that stops reading may still be sending.
func (p *Pool) SendAll(msg []byte, times int, skipStuck bool) error {
	for _, c := range p.Clients {
		if c == nil || (skipStuck && c.Stuck) {
			continue
		}
		for range times {
			if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return err
			}
		}
	}
	return nil
}

// Close closes every client: readers send a normal close frame and wait
// briefly for the server's answer, stuck clients just drop the socket.
func (p *Pool) Close() {
	var wg sync.WaitGroup
	for _, c := range p.Clients {
		if c == nil {
			continue
		}
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			if !c.Stuck {
				msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
				if c.Conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second)) == nil {
					select {
					case <-c.ended:
					case <-time.After(time.Second):
					}
				}
			}
			c.Conn.Close()
		}(c)
	}
	wg.Wait()
}

// ServerClosed reports whether the server ended this reader's connection,
// as opposed to Pool.Close or a network error.
func (c *Client) ServerClosed() bool {
	select {
	case <-c.ended:
	default:
		return false
	}
	var ce *websocket.CloseError
	return errors.As(c.endErr, &ce)
}
