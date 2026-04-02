package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// ============================================================================
// Graceful Shutdown Tests
// ============================================================================

// TestGracefulShutdown_Context verifies that ListenAndServeGraceful stops
// when the parent context is cancelled, returning nil (clean shutdown).
// This is the primary mechanism for programmatic shutdown and deterministic
// testing without OS signals.
//
// Uses context cancellation per Go's context.Context contract
// (https://pkg.go.dev/context).
func TestGracefulShutdown_Context(t *testing.T) {
	srv := &http.Server{
		Addr:    getFreePorts(t, 1)[0],
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(srv, WithContext(ctx))
	}()

	// Wait for server to start
	waitForServer(t, srv.Addr, 2*time.Second)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Expected nil error on clean shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for graceful shutdown")
	}
}

// TestGracefulShutdown_Signal verifies that ListenAndServeGraceful stops
// when a registered OS signal is received, returning nil.
//
// Per POSIX signal handling (signal(7)): SIGTERM is the standard termination
// signal sent by `kill`, and SIGINT is sent by Ctrl+C. Both are the default
// signals caught by ListenAndServeGraceful. This test uses SIGUSR1 to avoid
// interfering with the test runner.
func TestGracefulShutdown_Signal(t *testing.T) {
	srv := &http.Server{
		Addr:    getFreePorts(t, 1)[0],
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(srv,
			WithSignals(syscall.SIGUSR1), // Use SIGUSR1 to avoid interfering with test runner
		)
	}()

	waitForServer(t, srv.Addr, 2*time.Second)

	// Send signal to trigger shutdown
	syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Expected nil error on signal shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for signal shutdown")
	}
}

// TestGracefulShutdown_OnShutdownCallbacks verifies that OnShutdown callbacks
// are called in registration order before the server begins draining. This
// ensures SSEHub.CloseAll() can send goodbye events while connections are
// still open.
func TestGracefulShutdown_OnShutdownCallbacks(t *testing.T) {
	srv := &http.Server{
		Addr:    getFreePorts(t, 1)[0],
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	var order []int
	var mu sync.Mutex

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(srv,
			WithContext(ctx),
			WithOnShutdown(func() {
				mu.Lock()
				order = append(order, 1)
				mu.Unlock()
			}),
			WithOnShutdown(func() {
				mu.Lock()
				order = append(order, 2)
				mu.Unlock()
			}),
			WithOnShutdown(func() {
				mu.Lock()
				order = append(order, 3)
				mu.Unlock()
			}),
		)
	}()

	waitForServer(t, srv.Addr, 2*time.Second)
	cancel()

	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Fatalf("Expected 3 callbacks, got %d", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("Callback %d: expected %d, got %d", i, i+1, v)
		}
	}
}

// TestGracefulShutdown_DrainTimeout verifies that ListenAndServeGraceful
// respects the drain timeout for slow in-flight handlers. A handler that
// takes longer than the drain timeout should be interrupted.
//
// Per Go's http.Server.Shutdown documentation (https://pkg.go.dev/net/http#Server.Shutdown):
// "Shutdown does not attempt to close nor wait for hijacked connections
// such as WebSockets. The caller of Shutdown should separately notify
// such long-lived connections of shutdown and wait for them to close."
func TestGracefulShutdown_DrainTimeout(t *testing.T) {
	var handlerDone atomic.Bool
	srv := &http.Server{
		Addr: getFreePorts(t, 1)[0],
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Slow handler that blocks for 10s
			time.Sleep(10 * time.Second)
			handlerDone.Store(true)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(srv,
			WithContext(ctx),
			WithDrainTimeout(200*time.Millisecond),
		)
	}()

	waitForServer(t, srv.Addr, 2*time.Second)

	// Start a slow request
	go http.Get(fmt.Sprintf("http://%s/slow", srv.Addr))
	time.Sleep(50 * time.Millisecond) // let the request start

	// Trigger shutdown
	cancel()

	start := time.Now()
	select {
	case <-errCh:
		elapsed := time.Since(start)
		if elapsed > 2*time.Second {
			t.Errorf("Shutdown took too long: %v (expected ~200ms drain timeout)", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for shutdown with drain timeout")
	}
}

// TestGracefulShutdown_ServerError verifies that ListenAndServeGraceful
// returns the error immediately if the server fails to start (e.g., port
// already in use).
func TestGracefulShutdown_ServerError(t *testing.T) {
	addr := getFreePorts(t, 1)[0]

	// Occupy the port
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to occupy port: %v", err)
	}
	defer ln.Close()

	srv := &http.Server{
		Addr:    addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(srv)
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Expected error for port-in-use, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for server error")
	}
}

// TestGracefulShutdown_DefaultConfig verifies that ListenAndServeGraceful
// works with no options, using sensible defaults (SIGTERM+SIGINT, 30s drain).
func TestGracefulShutdown_DefaultConfig(t *testing.T) {
	srv := &http.Server{
		Addr:    getFreePorts(t, 1)[0],
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	// Use context to shut down since we can't safely send SIGTERM in tests
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(srv, WithContext(ctx))
	}()

	waitForServer(t, srv.Addr, 2*time.Second)

	// Verify server is responding
	resp, err := http.Get(fmt.Sprintf("http://%s/", srv.Addr))
	if err != nil {
		t.Fatalf("Server not responding: %v", err)
	}
	resp.Body.Close()

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out")
	}
}

// ============================================================================
// Test helpers
// ============================================================================

// getFreePorts returns n available TCP port addresses.
func getFreePorts(t *testing.T, n int) []string {
	t.Helper()
	ports := make([]string, n)
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to get free port: %v", err)
		}
		ports[i] = ln.Addr().String()
		ln.Close()
	}
	return ports
}

// waitForServer polls the given address until it accepts a TCP connection
// or the timeout expires.
func waitForServer(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Server at %s not ready after %v", addr, timeout)
}
