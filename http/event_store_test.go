package http

import (
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// MemoryEventStore Tests
// ============================================================================

// TestMemoryEventStoreStoreAndReplay verifies the basic round-trip: storing
// events and replaying them from the beginning (empty lastEventID triggers
// the "unknown ID" fallback which returns all events).
func TestMemoryEventStoreStoreAndReplay(t *testing.T) {
	store := NewMemoryEventStore(100)

	store.Store("stream1", StoredEvent{ID: "1", Event: "message", Data: []byte(`{"a":1}`)})
	store.Store("stream1", StoredEvent{ID: "2", Event: "message", Data: []byte(`{"a":2}`)})
	store.Store("stream1", StoredEvent{ID: "3", Event: "message", Data: []byte(`{"a":3}`)})

	// Replay from unknown ID returns all events
	events, err := ReplayAll(store, "stream1", "unknown")
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}
	if events[0].ID != "1" || events[1].ID != "2" || events[2].ID != "3" {
		t.Errorf("Unexpected event IDs: %v, %v, %v", events[0].ID, events[1].ID, events[2].ID)
	}
}

// TestMemoryEventStoreReplayFromMidStream verifies that Replay returns only
// events after the specified lastEventID, not the anchor event itself.
func TestMemoryEventStoreReplayFromMidStream(t *testing.T) {
	store := NewMemoryEventStore(100)

	for i := 1; i <= 5; i++ {
		store.Store("s1", StoredEvent{
			ID:    fmt.Sprintf("%d", i),
			Event: "message",
			Data:  []byte(fmt.Sprintf(`{"seq":%d}`, i)),
		})
	}

	// Replay from event "2" should return events 3, 4, 5
	events, err := ReplayAll(store, "s1", "2")
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("Expected 3 events after ID '2', got %d", len(events))
	}
	if events[0].ID != "3" {
		t.Errorf("First replayed event should be ID '3', got %q", events[0].ID)
	}
	if events[2].ID != "5" {
		t.Errorf("Last replayed event should be ID '5', got %q", events[2].ID)
	}
}

// TestMemoryEventStoreReplayFromLastEvent verifies that replaying from the
// most recent event returns an empty slice (nothing was missed).
func TestMemoryEventStoreReplayFromLastEvent(t *testing.T) {
	store := NewMemoryEventStore(100)

	store.Store("s1", StoredEvent{ID: "1", Event: "message", Data: []byte(`{}`)})
	store.Store("s1", StoredEvent{ID: "2", Event: "message", Data: []byte(`{}`)})

	events, err := ReplayAll(store, "s1", "2")
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Expected 0 events when replaying from last event, got %d", len(events))
	}
}

// TestMemoryEventStoreReplayNonexistentStream verifies that replaying from
// a stream that doesn't exist returns nil without error.
func TestMemoryEventStoreReplayNonexistentStream(t *testing.T) {
	store := NewMemoryEventStore(100)

	events, err := ReplayAll(store, "nonexistent", "1")
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if events != nil {
		t.Errorf("Expected nil for nonexistent stream, got %v", events)
	}
}

// TestMemoryEventStoreMaxEventsEviction verifies that when a stream exceeds
// maxPerStream, the oldest events are dropped (FIFO eviction).
func TestMemoryEventStoreMaxEventsEviction(t *testing.T) {
	store := NewMemoryEventStore(3) // only keep 3 events

	for i := 1; i <= 5; i++ {
		store.Store("s1", StoredEvent{
			ID:    fmt.Sprintf("%d", i),
			Event: "message",
			Data:  []byte(fmt.Sprintf(`%d`, i)),
		})
	}

	// Should only have events 3, 4, 5 (oldest 1, 2 evicted)
	events, err := ReplayAll(store, "s1", "unknown")
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("Expected 3 events (max), got %d", len(events))
	}
	if events[0].ID != "3" {
		t.Errorf("Oldest surviving event should be ID '3', got %q", events[0].ID)
	}
	if events[2].ID != "5" {
		t.Errorf("Newest event should be ID '5', got %q", events[2].ID)
	}
}

// TestMemoryEventStoreEvictionReplayFromEvictedID verifies that when the
// anchor event has been evicted, Replay returns all remaining events
// (conservative fallback).
func TestMemoryEventStoreEvictionReplayFromEvictedID(t *testing.T) {
	store := NewMemoryEventStore(3)

	for i := 1; i <= 5; i++ {
		store.Store("s1", StoredEvent{ID: fmt.Sprintf("%d", i), Event: "message", Data: []byte(`{}`)})
	}

	// Event "1" was evicted — fallback returns all 3 remaining events
	events, err := ReplayAll(store, "s1", "1")
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("Expected 3 events (all remaining) for evicted anchor, got %d", len(events))
	}
	if events[0].ID != "3" {
		t.Errorf("First event should be ID '3', got %q", events[0].ID)
	}
}

// TestMemoryEventStoreTrim verifies that Trim removes all events for a
// stream and subsequent Replay returns nil.
func TestMemoryEventStoreTrim(t *testing.T) {
	store := NewMemoryEventStore(100)

	store.Store("s1", StoredEvent{ID: "1", Event: "message", Data: []byte(`{}`)})
	store.Store("s1", StoredEvent{ID: "2", Event: "message", Data: []byte(`{}`)})
	store.Store("s2", StoredEvent{ID: "1", Event: "message", Data: []byte(`{}`)})

	err := store.Trim("s1")
	if err != nil {
		t.Fatalf("Trim returned error: %v", err)
	}

	// s1 should be empty
	events, _ := ReplayAll(store, "s1", "unknown")
	if events != nil {
		t.Errorf("Expected nil after Trim, got %d events", len(events))
	}

	// s2 should be unaffected
	events, _ = ReplayAll(store, "s2", "unknown")
	if len(events) != 1 {
		t.Errorf("Expected s2 to have 1 event, got %d", len(events))
	}
}

// TestMemoryEventStoreTrimNonexistent verifies that trimming a nonexistent
// stream is a no-op without error.
func TestMemoryEventStoreTrimNonexistent(t *testing.T) {
	store := NewMemoryEventStore(100)
	err := store.Trim("nonexistent")
	if err != nil {
		t.Fatalf("Trim returned error for nonexistent stream: %v", err)
	}
}

// TestMemoryEventStoreIsolation verifies that events in different streams
// are independent — storing to one stream does not affect another.
func TestMemoryEventStoreIsolation(t *testing.T) {
	store := NewMemoryEventStore(100)

	store.Store("s1", StoredEvent{ID: "1", Event: "message", Data: []byte(`{"stream":"s1"}`)})
	store.Store("s2", StoredEvent{ID: "1", Event: "message", Data: []byte(`{"stream":"s2"}`)})
	store.Store("s2", StoredEvent{ID: "2", Event: "message", Data: []byte(`{"stream":"s2"}`)})

	s1Events, _ := ReplayAll(store, "s1", "unknown")
	s2Events, _ := ReplayAll(store, "s2", "unknown")

	if len(s1Events) != 1 {
		t.Errorf("s1 should have 1 event, got %d", len(s1Events))
	}
	if len(s2Events) != 2 {
		t.Errorf("s2 should have 2 events, got %d", len(s2Events))
	}
}

// TestMemoryEventStorePreservesEventFields verifies that all StoredEvent
// fields (ID, Event, Data) are preserved through Store and Replay.
func TestMemoryEventStorePreservesEventFields(t *testing.T) {
	store := NewMemoryEventStore(100)

	original := StoredEvent{
		ID:    "evt-42",
		Event: "notification",
		Data:  []byte(`{"key":"value","nested":{"x":1}}`),
	}
	store.Store("s1", original)

	events, _ := ReplayAll(store, "s1", "unknown")
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	got := events[0]
	if got.ID != original.ID {
		t.Errorf("ID: got %q, want %q", got.ID, original.ID)
	}
	if got.Event != original.Event {
		t.Errorf("Event: got %q, want %q", got.Event, original.Event)
	}
	if string(got.Data) != string(original.Data) {
		t.Errorf("Data: got %q, want %q", got.Data, original.Data)
	}
}

// TestMemoryEventStoreDefaultMaxPerStream verifies that passing 0 or negative
// maxPerStream defaults to 1000.
func TestMemoryEventStoreDefaultMaxPerStream(t *testing.T) {
	store := NewMemoryEventStore(0)
	if store.maxPerStream != 1000 {
		t.Errorf("Expected default maxPerStream=1000, got %d", store.maxPerStream)
	}

	store = NewMemoryEventStore(-5)
	if store.maxPerStream != 1000 {
		t.Errorf("Expected default maxPerStream=1000 for negative input, got %d", store.maxPerStream)
	}
}

// TestMemoryEventStoreConcurrentAccess verifies that concurrent Store, Replay,
// and Trim operations do not cause data races. Run with -race flag.
func TestMemoryEventStoreConcurrentAccess(t *testing.T) {
	store := NewMemoryEventStore(100)
	const goroutines = 20

	var wg sync.WaitGroup

	// Concurrent stores to same stream
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.Store("shared", StoredEvent{
				ID:    fmt.Sprintf("%d", idx),
				Event: "message",
				Data:  []byte(fmt.Sprintf(`{"idx":%d}`, idx)),
			})
		}(i)
	}

	// Concurrent replays
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ReplayAll(store, "shared", "unknown")
		}()
	}

	// Concurrent stores to different streams
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			streamID := fmt.Sprintf("stream-%d", idx)
			store.Store(streamID, StoredEvent{ID: "1", Event: "message", Data: []byte(`{}`)})
			ReplayAll(store, streamID, "unknown")
			store.Trim(streamID)
		}(i)
	}

	wg.Wait()
}

// TestMemoryEventStoreReplayReturnsCopy verifies that the slice returned by
// Replay is a copy — mutating it does not affect the store's internal state.
func TestMemoryEventStoreReplayReturnsCopy(t *testing.T) {
	store := NewMemoryEventStore(100)

	store.Store("s1", StoredEvent{ID: "1", Event: "message", Data: []byte(`{"a":1}`)})
	store.Store("s1", StoredEvent{ID: "2", Event: "message", Data: []byte(`{"a":2}`)})

	events, _ := ReplayAll(store, "s1", "unknown")
	// Mutate the returned slice
	events[0].ID = "mutated"

	// Re-replay should be unaffected
	events2, _ := ReplayAll(store, "s1", "unknown")
	if events2[0].ID != "1" {
		t.Errorf("Store internal state was mutated via Replay result: got ID %q, want '1'", events2[0].ID)
	}
}

func storeN(t *testing.T, store EventStore, stream string, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if err := store.Store(stream, StoredEvent{ID: fmt.Sprint(i), Data: []byte(fmt.Sprint(i))}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMemoryEventStoreReplayStopsWhenConsumerBreaks(t *testing.T) {
	store := NewMemoryEventStore(10)
	storeN(t, store, "s1", 5)
	var got []string
	for ev, err := range store.Replay("s1", "unknown") {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ev.ID)
		if len(got) == 2 {
			break
		}
	}
	if fmt.Sprint(got) != "[1 2]" {
		t.Fatalf("got %v, want [1 2]", got)
	}
}

// A slow consumer (an SSE write per event) must not block Store on the same
// stream, so Replay can't hold the store's lock while it yields.
func TestMemoryEventStoreReplayDoesNotBlockStore(t *testing.T) {
	store := NewMemoryEventStore(10)
	storeN(t, store, "s1", 3)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range store.Replay("s1", "unknown") {
			store.Store("s1", StoredEvent{ID: "late"})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Store blocked while Replay was yielding")
	}
}

func TestMemoryEventStoreReplayIsASnapshot(t *testing.T) {
	store := NewMemoryEventStore(10)
	storeN(t, store, "s1", 3)
	var got []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range store.Replay("s1", "1") {
			got = append(got, ev.ID)
			store.Store("s1", StoredEvent{ID: "late-" + ev.ID})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Store blocked while Replay was yielding")
	}
	if fmt.Sprint(got) != "[2 3]" {
		t.Fatalf("got %v, want [2 3] (events stored during replay are not yielded)", got)
	}
}

type failingStore struct{ EventStore }

func (failingStore) Replay(string, string) iter.Seq2[StoredEvent, error] {
	return func(yield func(StoredEvent, error) bool) {
		if !yield(StoredEvent{ID: "1"}, nil) {
			return
		}
		yield(StoredEvent{}, errors.New("backend down"))
	}
}

func TestReplayAllReturnsTheFirstError(t *testing.T) {
	events, err := ReplayAll(failingStore{}, "s1", "")
	if err == nil || err.Error() != "backend down" {
		t.Fatalf("got err %v, want backend down", err)
	}
	if len(events) != 1 || events[0].ID != "1" {
		t.Fatalf("got %v, want the event read before the error", events)
	}
}
