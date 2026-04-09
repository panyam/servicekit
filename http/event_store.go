package http

import (
	"sync"
)

// ============================================================================
// EventStore — SSE event persistence for stream resumption
// ============================================================================

// StoredEvent represents a single SSE event that has been persisted for
// potential replay. It captures the three fields needed to reconstruct
// the original SSE wire format: event type, event ID, and raw data.
type StoredEvent struct {
	// ID is the SSE event ID ("id:" field). Used by clients to resume
	// via the Last-Event-ID header on reconnection.
	ID string

	// Event is the SSE event type ("event:" field). If empty, clients
	// receive the event via the default "message" handler.
	Event string

	// Data is the raw event payload (pre-serialized bytes). Stored as-is
	// to avoid re-serialization on replay.
	Data []byte
}

// EventStore persists SSE events for stream resumption. When a client
// reconnects with a Last-Event-ID header, the server replays missed
// events from the store.
//
// Implementations must be safe for concurrent use by multiple goroutines.
//
// The streamID parameter groups events by logical stream (e.g., session ID).
// Event IDs are opaque strings — ordering is determined by insertion order,
// not by parsing the ID value.
type EventStore interface {
	// Store persists an event for the given stream. Events are ordered by
	// insertion time within a stream.
	Store(streamID string, event StoredEvent) error

	// Replay returns all events stored after the event with the given
	// lastEventID. If lastEventID is not found in the stream, all stored
	// events are returned (conservative fallback — the client may have
	// been disconnected long enough for the anchor event to be evicted).
	//
	// Returns an empty slice if the stream does not exist or if
	// lastEventID is the most recent event.
	Replay(streamID string, lastEventID string) ([]StoredEvent, error)

	// Trim removes all stored events for the given stream. Call this
	// when a session is destroyed to prevent unbounded memory growth.
	Trim(streamID string) error
}

// ============================================================================
// MemoryEventStore — bounded in-memory implementation
// ============================================================================

// MemoryEventStore is an in-memory EventStore backed by bounded per-stream
// slices. When a stream exceeds maxPerStream events, the oldest events are
// dropped (FIFO eviction).
//
// This implementation is suitable for single-process deployments. For
// multi-process or persistent resumption, use a Redis or database-backed
// EventStore.
//
// Thread-safe: all methods are protected by a sync.RWMutex.
type MemoryEventStore struct {
	mu           sync.RWMutex
	streams      map[string][]StoredEvent
	maxPerStream int
}

// NewMemoryEventStore creates a MemoryEventStore with the given maximum
// events per stream. If maxPerStream <= 0, it defaults to 1000.
func NewMemoryEventStore(maxPerStream int) *MemoryEventStore {
	if maxPerStream <= 0 {
		maxPerStream = 1000
	}
	return &MemoryEventStore{
		streams:      make(map[string][]StoredEvent),
		maxPerStream: maxPerStream,
	}
}

// Store appends an event to the stream. If the stream exceeds maxPerStream,
// the oldest event is dropped.
func (s *MemoryEventStore) Store(streamID string, event StoredEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := s.streams[streamID]
	events = append(events, event)

	// Evict oldest if over capacity
	if len(events) > s.maxPerStream {
		// Drop the oldest events to stay within bounds.
		// Copy to avoid holding references to the old backing array.
		excess := len(events) - s.maxPerStream
		trimmed := make([]StoredEvent, s.maxPerStream)
		copy(trimmed, events[excess:])
		events = trimmed
	}

	s.streams[streamID] = events
	return nil
}

// Replay returns all events after lastEventID by scanning for it in the
// stream's insertion-ordered slice. If lastEventID is not found (e.g.,
// evicted), all stored events are returned as a conservative fallback.
func (s *MemoryEventStore) Replay(streamID string, lastEventID string) ([]StoredEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.streams[streamID]
	if len(events) == 0 {
		return nil, nil
	}

	// Scan for the anchor event
	for i, ev := range events {
		if ev.ID == lastEventID {
			// Return everything after the anchor
			if i+1 >= len(events) {
				return nil, nil // anchor is the last event
			}
			result := make([]StoredEvent, len(events)-i-1)
			copy(result, events[i+1:])
			return result, nil
		}
	}

	// Anchor not found — return all events (conservative fallback)
	result := make([]StoredEvent, len(events))
	copy(result, events)
	return result, nil
}

// Trim removes all stored events for the given stream.
func (s *MemoryEventStore) Trim(streamID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streams, streamID)
	return nil
}

// ============================================================================
// Compile-time interface compliance
// ============================================================================

var _ EventStore = (*MemoryEventStore)(nil)
