package http

import (
	"iter"
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

	// Replay yields, in insertion order, the events stored after the event
	// with the given lastEventID. If lastEventID is not in the stream (never
	// stored, or already evicted), it yields every stored event, since the
	// client may have been away long enough for its anchor to be dropped.
	// It yields nothing for an unknown stream or when lastEventID is the most
	// recent event.
	//
	// A backend that fails part-way yields a zero StoredEvent with the error
	// and stops. Breaking out of the loop early must release any resources
	// the iterator holds. Use ReplayAll to collect the events into a slice.
	//
	// The iterator is consumed as the events are written to a client, which
	// can be slow, so it must not block Store on the same stream while the
	// caller is between events.
	Replay(streamID string, lastEventID string) iter.Seq2[StoredEvent, error]

	// Trim removes all stored events for the given stream. Call this
	// when a session is destroyed to prevent unbounded memory growth.
	Trim(streamID string) error
}

// ReplayAll collects store.Replay into a slice. It returns the events read
// before the first error along with that error.
func ReplayAll(store EventStore, streamID string, lastEventID string) ([]StoredEvent, error) {
	var out []StoredEvent
	for ev, err := range store.Replay(streamID, lastEventID) {
		if err != nil {
			return out, err
		}
		out = append(out, ev)
	}
	return out, nil
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

// Replay yields the events after lastEventID without copying them, and
// without holding the lock while it yields.
//
// It takes the stream's slice header under the read lock and iterates it after
// releasing the lock. That snapshot stays valid: Store only appends past its
// length or swaps in a new slice on eviction, and never writes to an element
// the snapshot can see. Events stored after Replay starts aren't yielded.
func (s *MemoryEventStore) Replay(streamID string, lastEventID string) iter.Seq2[StoredEvent, error] {
	return func(yield func(StoredEvent, error) bool) {
		s.mu.RLock()
		events := s.streams[streamID]
		s.mu.RUnlock()
		start := 0
		for i, ev := range events {
			if ev.ID == lastEventID {
				start = i + 1
				break
			}
		}
		for _, ev := range events[start:] {
			if !yield(ev, nil) {
				return
			}
		}
	}
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
