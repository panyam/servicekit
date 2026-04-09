package http

import (
	"log"
	"sync"
)

// SSEHub manages a collection of SSE connections, providing session tracking,
// targeted delivery, and broadcast capabilities. It is the SSE counterpart to
// the WebSocket hub/room pattern used in the grpcws-demo's GameHub.
//
// SSEHub is instance-based (not a package-level global) for testability and
// to support multiple independent hubs in the same process.
//
// Type parameter O is the output message type sent to clients.
//
// Usage:
//
//	hub := gohttp.NewSSEHub[MyEvent]()
//
//	// In your SSEConn.OnStart:
//	hub.Register(conn)
//
//	// In your SSEConn.OnClose:
//	hub.Unregister(conn.ConnId())
//
//	// From application code:
//	hub.Broadcast(MyEvent{Type: "update", Data: ...})
//	hub.Send(sessionId, MyEvent{Type: "direct", Data: ...})
//
//	// On graceful shutdown:
//	hub.CloseAll()
type SSEHub[O any] struct {
	mu    sync.RWMutex
	conns map[string]*BaseSSEConn[O]
}

// NewSSEHub creates a new SSEHub for managing SSE connections.
func NewSSEHub[O any]() *SSEHub[O] {
	return &SSEHub[O]{
		conns: make(map[string]*BaseSSEConn[O]),
	}
}

// Register adds an SSE connection to the hub, keyed by its ConnId.
// If a connection with the same ID already exists, it is replaced (the old
// connection is NOT closed — the caller is responsible for lifecycle management).
func (h *SSEHub[O]) Register(conn *BaseSSEConn[O]) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[conn.ConnId()] = conn
	log.Printf("SSEHub: registered connection %s (total: %d)", conn.ConnId(), len(h.conns))
}

// Unregister removes an SSE connection from the hub by its ConnId.
// The connection's OnClose is NOT called — the caller manages the connection
// lifecycle (typically SSEServe handles OnClose via defer).
//
// Unregistering a nonexistent ID is a no-op.
func (h *SSEHub[O]) Unregister(connId string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, connId)
	log.Printf("SSEHub: unregistered connection %s (total: %d)", connId, len(h.conns))
}

// Send delivers a message to a specific connection by ID.
// Returns true if the connection was found and the message was queued.
// Returns false if the connection ID does not exist (no error, no panic).
func (h *SSEHub[O]) Send(connId string, msg O) bool {
	h.mu.RLock()
	conn, ok := h.conns[connId]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	conn.SendOutput(msg)
	return true
}

// SendEvent delivers a named event to a specific connection by ID.
// The event type is set via the SSE "event:" field.
// Returns true if the connection was found and the message was queued.
// Returns false if the connection ID does not exist.
func (h *SSEHub[O]) SendEvent(connId string, event string, msg O) bool {
	h.mu.RLock()
	conn, ok := h.conns[connId]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	conn.SendEvent(event, msg)
	return true
}

// Broadcast sends a message to all registered connections. The message is
// queued to each connection's Writer independently, so a slow connection
// does not block others.
func (h *SSEHub[O]) Broadcast(msg O) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.conns {
		conn.SendOutput(msg)
	}
}

// BroadcastEvent sends a named event to all registered connections.
// The event type is set via the SSE "event:" field.
func (h *SSEHub[O]) BroadcastEvent(event string, msg O) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.conns {
		conn.SendEvent(event, msg)
	}
}

// SendEventWithID delivers a named event with an ID to a specific connection.
// The ID is set via the SSE "id:" field, enabling client reconnection via the
// Last-Event-ID header.
// Returns true if the connection was found and the message was queued.
// Returns false if the connection ID does not exist.
func (h *SSEHub[O]) SendEventWithID(connId, event, id string, msg O) bool {
	h.mu.RLock()
	conn, ok := h.conns[connId]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	conn.SendEventWithID(event, id, msg)
	return true
}

// BroadcastEventWithID sends a named event with an ID to all registered
// connections. The ID is set via the SSE "id:" field.
func (h *SSEHub[O]) BroadcastEventWithID(event, id string, msg O) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.conns {
		conn.SendEventWithID(event, id, msg)
	}
}

// Count returns the number of currently registered connections.
func (h *SSEHub[O]) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// CloseAll closes all registered connections by calling OnClose on each,
// then clears the connection map. Use this for graceful shutdown.
//
// After CloseAll, the hub is empty and can be reused.
func (h *SSEHub[O]) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, conn := range h.conns {
		conn.OnClose()
		delete(h.conns, id)
	}
	log.Printf("SSEHub: closed all connections")
}
