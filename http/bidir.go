package http

import "time"

// BiDirStreamConfig provides configuration for bidirectional stream connections.
// It controls the timing of health checks and connection timeout detection.
type BiDirStreamConfig struct {
	// PingPeriod specifies how often to send ping messages to the remote peer.
	// Pings are used as heartbeat messages to verify the connection is alive.
	// Default: 30 seconds.
	PingPeriod time.Duration

	// PongPeriod specifies the maximum time to wait for any data (ping, pong, or
	// regular messages) from the remote peer before considering the connection dead.
	// If no data is received within this duration, OnTimeout() is called.
	// Default: 300 seconds (5 minutes).
	PongPeriod time.Duration
}

// DefaultBiDirStreamConfig returns a BiDirStreamConfig with sensible defaults:
//   - PingPeriod: 30 seconds
//   - PongPeriod: 300 seconds (5 minutes)
//
// These defaults are suitable for most production deployments. For development
// or testing, you may want shorter periods for faster feedback.
func DefaultBiDirStreamConfig() *BiDirStreamConfig {
	return &BiDirStreamConfig{
		PingPeriod: time.Second * 30,
		PongPeriod: time.Second * 300,
	}
}

// BiDirStreamConn defines the lifecycle and message handling interface for
// bidirectional stream connections. Implementations handle messages of type I
// and manage connection state through lifecycle hooks.
//
// The typical lifecycle is:
//  1. Connection established, OnStart() called (not part of this interface)
//  2. Messages received and processed via HandleMessage()
//  3. Periodic SendPing() calls to maintain connection health
//  4. On errors, OnError() is called to determine if connection should continue
//  5. On timeout (no data received within PongPeriod), OnTimeout() is called
//  6. Connection ends, OnClose() is called for cleanup
type BiDirStreamConn[I any] interface {
	// SendPing sends a heartbeat ping message to the remote peer.
	// Called periodically based on BiDirStreamConfig.PingPeriod.
	// The implementation should encode and send an appropriate ping message.
	SendPing() error

	// Name returns an optional human-readable name for this connection type.
	// Used for logging and debugging purposes.
	Name() string

	// ConnId returns a unique identifier for this specific connection instance.
	// Useful for tracking individual connections in logs and metrics.
	ConnId() string

	// HandleMessage processes an incoming message of type I.
	// Called for each message received from the remote peer.
	// Return an error to signal that the connection should be closed.
	HandleMessage(msg I) error

	// OnError is called when an error occurs during connection operation.
	// Return nil to suppress the error and continue the connection.
	// Return the error (or a different error) to close the connection.
	OnError(err error) error

	// OnClose is called when the connection is closing for any reason.
	// Use this hook to clean up resources (timers, goroutines, etc.)
	// and perform any final actions (logging, metrics, etc.).
	OnClose()

	// OnTimeout is called when no data has been received within the PongPeriod.
	// Return true to close the connection, false to continue waiting.
	// Typically returns true, but may return false for connections that
	// expect long periods of silence.
	OnTimeout() bool
}
