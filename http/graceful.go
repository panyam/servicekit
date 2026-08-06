package http

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ============================================================================
// Graceful Shutdown Configuration
// ============================================================================

// GracefulOption configures ListenAndServeGraceful behavior.
type GracefulOption func(*gracefulConfig)

type gracefulConfig struct {
	drainTimeout time.Duration
	signals      []os.Signal
	onShutdown   []func()
	ctx          context.Context
	listener     net.Listener
}

func defaultGracefulConfig() *gracefulConfig {
	return &gracefulConfig{
		drainTimeout: 30 * time.Second,
		signals:      []os.Signal{syscall.SIGTERM, syscall.SIGINT},
	}
}

// WithDrainTimeout sets the maximum time to wait for in-flight requests
// to complete during shutdown. Default: 30 seconds.
//
// After this timeout expires, remaining connections are forcefully closed.
func WithDrainTimeout(d time.Duration) GracefulOption {
	return func(c *gracefulConfig) {
		c.drainTimeout = d
	}
}

// WithSignals sets the OS signals that trigger shutdown.
// Default: SIGTERM, SIGINT.
func WithSignals(sigs ...os.Signal) GracefulOption {
	return func(c *gracefulConfig) {
		c.signals = sigs
	}
}

// WithOnShutdown registers a callback to be invoked when shutdown is triggered,
// BEFORE the server begins draining connections. Multiple callbacks are called
// in registration order.
//
// Use this to notify long-lived connections before they are closed:
//   - SSEHub.CloseAll() — closes SSE Done channels, unblocking SSEServe handlers
//   - Custom WebSocket goodbye messages
//   - Flush logs, metrics, or other state
//
// Callbacks run while connections are still open, so they can send final
// messages to clients.
func WithOnShutdown(fn func()) GracefulOption {
	return func(c *gracefulConfig) {
		c.onShutdown = append(c.onShutdown, fn)
	}
}

// WithListener supplies an already-bound listener to serve on, instead of
// having ListenAndServeGraceful bind srv.Addr itself.
//
// This exists so callers can eliminate the start-up race: bind first, then
// hand the listener over. Once net.Listen has returned, the port is accepting
// connections, so a caller that starts the server in a goroutine no longer
// has to sleep-and-hope before dialing it.
//
// It also makes port :0 usable — the caller reads the kernel-assigned port off
// ln.Addr() before serving begins.
//
// srv.Addr is ignored when a listener is set. Closing the listener is handled
// by srv.Shutdown during the normal drain.
//
// Example:
//
//	ln, err := net.Listen("tcp", ":0")
//	if err != nil { return err }
//	go gohttp.ListenAndServeGraceful(srv, gohttp.WithListener(ln))
//	// ln.Addr() is bound and accepting right here — no sleep needed.
func WithListener(ln net.Listener) GracefulOption {
	return func(c *gracefulConfig) {
		c.listener = ln
	}
}

// WithContext sets a parent context. Shutdown is triggered when this context
// is cancelled, in addition to OS signals. This is useful for programmatic
// shutdown in tests or when embedding the server in a larger application.
func WithContext(ctx context.Context) GracefulOption {
	return func(c *gracefulConfig) {
		c.ctx = ctx
	}
}

// ============================================================================
// ListenAndServeGraceful
// ============================================================================

// ListenAndServeGraceful starts the HTTP server and handles graceful shutdown.
// It blocks until a shutdown signal is received (OS signal or context cancellation),
// then drains active connections within the configured timeout.
//
// By default it binds srv.Addr itself. Pass WithListener to serve on a
// listener you have already bound, which removes the start-up race for
// callers that run this in a goroutine.
//
// Shutdown lifecycle:
//  1. Start srv.ListenAndServe() — or srv.Serve(ln) with WithListener — in a goroutine
//  2. Block until: OS signal, parent context cancellation, or server error
//  3. Call OnShutdown callbacks (e.g. SSEHub.CloseAll()) — connections still open
//  4. Call srv.Shutdown() with drain timeout — waits for in-flight handlers
//  5. Return nil on clean shutdown, or the error
//
// Example:
//
//	srv := &http.Server{Addr: ":8080", Handler: mux}
//	middleware.ApplyDefaults(srv)
//	srv.WriteTimeout = 0 // required for SSE
//
//	hub := gohttp.NewSSEHub[any]()
//	err := gohttp.ListenAndServeGraceful(srv,
//	    gohttp.WithDrainTimeout(10*time.Second),
//	    gohttp.WithOnShutdown(hub.CloseAll),
//	)
func ListenAndServeGraceful(srv *http.Server, opts ...GracefulOption) error {
	cfg := defaultGracefulConfig()
	for _, o := range opts {
		o(cfg)
	}

	// Channel for server startup errors (port in use, etc.)
	serverErr := make(chan error, 1)
	go func() {
		// A caller-supplied listener is already bound, so Serve skips the
		// bind entirely — see WithListener.
		var err error
		if cfg.listener != nil {
			err = srv.Serve(cfg.listener)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, cfg.signals...)
	defer signal.Stop(sigChan)

	// Parent context channel (nil if no context provided)
	var ctxDone <-chan struct{}
	if cfg.ctx != nil {
		ctxDone = cfg.ctx.Done()
	}

	// Block until shutdown trigger or server error
	select {
	case err := <-serverErr:
		// Server failed to start
		return err
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
	case <-ctxDone:
		log.Println("Context cancelled, shutting down...")
	}

	// Run OnShutdown callbacks (connections still open)
	for _, fn := range cfg.onShutdown {
		fn()
	}

	// Drain active connections
	drainCtx, cancel := context.WithTimeout(context.Background(), cfg.drainTimeout)
	defer cancel()

	if err := srv.Shutdown(drainCtx); err != nil {
		log.Printf("Shutdown error: %v", err)
		return err
	}

	log.Println("Server shut down gracefully")
	return nil
}
