// grpcws-demo demonstrates the grpcws package for gRPC-over-WebSocket streaming.
//
// This demo shows all three gRPC streaming patterns using real proto messages:
// - Server streaming: Subscribe to game events
// - Client streaming: Send commands, get summary
// - Bidirectional streaming: Real-time game sync
//
// Run: go run ./cmd/grpcws-demo
// Test: Open http://localhost:8080 in browser
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/panyam/servicekit/cmd/grpcws-demo/gen"
	"github.com/panyam/servicekit/grpcws"
	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

//go:embed index.html
var indexHTML string

func main() {
	router := mux.NewRouter()

	// Serve the test UI
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(indexHTML))
	})

	// WebSocket endpoints using grpcws handlers
	wsRouter := router.PathPrefix("/ws").Subrouter()

	// Server streaming: Subscribe to game events
	wsRouter.HandleFunc("/v1/subscribe", gohttp.WSServe(
		grpcws.NewServerStreamHandler(
			func(ctx context.Context, req *gen.SubscribeRequest) (*MockServerStream, error) {
				return NewMockServerStream(ctx, req), nil
			},
			func(r *http.Request) (*gen.SubscribeRequest, error) {
				// Parse request from query params or body
				return &gen.SubscribeRequest{
					GameId:   r.URL.Query().Get("game_id"),
					PlayerId: r.URL.Query().Get("player_id"),
				}, nil
			},
		),
		nil,
	))

	// Client streaming: Send commands
	wsRouter.HandleFunc("/v1/commands", gohttp.WSServe(
		grpcws.NewClientStreamHandler(
			func(ctx context.Context) (*MockClientStream, error) {
				return NewMockClientStream(ctx), nil
			},
			func() *gen.GameCommand { return &gen.GameCommand{} },
		),
		nil,
	))

	// Bidirectional streaming: Game sync
	wsRouter.HandleFunc("/v1/sync", gohttp.WSServe(
		grpcws.NewBidiStreamHandler(
			func(ctx context.Context) (*MockBidiStream, error) {
				return NewMockBidiStream(ctx), nil
			},
			func() *gen.PlayerAction { return &gen.PlayerAction{} },
		),
		nil,
	))

	log.Println("Starting grpcws-demo server on :8080")
	log.Println("Open http://localhost:8080 in browser to test")
	log.Println("")
	log.Println("WebSocket endpoints:")
	log.Println("  ws://localhost:8080/ws/v1/subscribe  - Server streaming (game events)")
	log.Println("  ws://localhost:8080/ws/v1/commands   - Client streaming (send commands)")
	log.Println("  ws://localhost:8080/ws/v1/sync       - Bidi streaming (game sync)")

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal(err)
	}
}

// ============================================================================
// Mock Streams - Simulate gRPC streaming behavior
// ============================================================================

// MockServerStream simulates a server-streaming gRPC client
type MockServerStream struct {
	ctx      context.Context
	cancel   context.CancelFunc
	req      *gen.SubscribeRequest
	eventNum int
	ticker   *time.Ticker
	mu       sync.Mutex
}

func NewMockServerStream(parent context.Context, req *gen.SubscribeRequest) *MockServerStream {
	ctx, cancel := context.WithCancel(parent)
	return &MockServerStream{
		ctx:    ctx,
		cancel: cancel,
		req:    req,
		ticker: time.NewTicker(2 * time.Second),
	}
}

func (s *MockServerStream) Recv() (*gen.GameEvent, error) {
	select {
	case <-s.ctx.Done():
		return nil, io.EOF
	case <-s.ticker.C:
		s.mu.Lock()
		s.eventNum++
		num := s.eventNum
		s.mu.Unlock()

		event := &gen.GameEvent{
			EventId:   fmt.Sprintf("evt-%d", num),
			EventType: "score_update",
			Timestamp: time.Now().UnixMilli(),
			GameId:    s.req.GameId,
			Payload: &gen.GameEvent_ScoreUpdate{
				ScoreUpdate: &gen.ScoreUpdate{
					PlayerId: s.req.PlayerId,
					OldScore: int32((num - 1) * 10),
					NewScore: int32(num * 10),
				},
			},
		}
		return event, nil
	}
}

// grpc.ClientStream implementation
func (s *MockServerStream) Header() (metadata.MD, error) { return nil, nil }
func (s *MockServerStream) Trailer() metadata.MD         { return nil }
func (s *MockServerStream) CloseSend() error             { return nil }
func (s *MockServerStream) Context() context.Context     { return s.ctx }
func (s *MockServerStream) SendMsg(m any) error          { return nil }
func (s *MockServerStream) RecvMsg(m any) error          { return nil }

// Verify interface compliance
var _ grpcws.ServerStream[*gen.GameEvent] = (*MockServerStream)(nil)

// MockClientStream simulates a client-streaming gRPC client
type MockClientStream struct {
	ctx      context.Context
	cancel   context.CancelFunc
	commands []*gen.GameCommand
	mu       sync.Mutex
}

func NewMockClientStream(parent context.Context) *MockClientStream {
	ctx, cancel := context.WithCancel(parent)
	return &MockClientStream{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *MockClientStream) Send(cmd *gen.GameCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, cmd)
	log.Printf("Received command: %s (%s)", cmd.CommandId, cmd.CommandType)
	return nil
}

func (s *MockClientStream) CloseAndRecv() (*gen.CommandSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Simulate some commands failing
	failed := 0
	var errs []string
	for _, cmd := range s.commands {
		if rand.Float32() < 0.1 {
			failed++
			errs = append(errs, fmt.Sprintf("command %s failed", cmd.CommandId))
		}
	}

	return &gen.CommandSummary{
		CommandsReceived: int32(len(s.commands)),
		CommandsExecuted: int32(len(s.commands) - failed),
		CommandsFailed:   int32(failed),
		Errors:           errs,
	}, nil
}

// grpc.ClientStream implementation
func (s *MockClientStream) Header() (metadata.MD, error) { return nil, nil }
func (s *MockClientStream) Trailer() metadata.MD         { return nil }
func (s *MockClientStream) CloseSend() error             { return nil }
func (s *MockClientStream) Context() context.Context     { return s.ctx }
func (s *MockClientStream) SendMsg(m any) error          { return nil }
func (s *MockClientStream) RecvMsg(m any) error          { return nil }

// Verify interface compliance
var _ grpcws.ClientStream[*gen.GameCommand, *gen.CommandSummary] = (*MockClientStream)(nil)

// MockBidiStream simulates a bidirectional streaming gRPC client
type MockBidiStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	actions   chan *gen.PlayerAction
	responses chan *gen.GameState
	closed    bool
	mu        sync.Mutex
}

func NewMockBidiStream(parent context.Context) *MockBidiStream {
	ctx, cancel := context.WithCancel(parent)
	s := &MockBidiStream{
		ctx:       ctx,
		cancel:    cancel,
		actions:   make(chan *gen.PlayerAction, 10),
		responses: make(chan *gen.GameState, 10),
	}

	// Start a goroutine to process actions and generate game state updates
	go s.processActions()

	return s
}

func (s *MockBidiStream) processActions() {
	stateNum := 0
	players := make(map[string]*gen.PlayerState)

	for {
		select {
		case <-s.ctx.Done():
			return
		case action, ok := <-s.actions:
			if !ok {
				return
			}

			// Update player state based on action
			player, exists := players[action.PlayerId]
			if !exists {
				player = &gen.PlayerState{
					PlayerId: action.PlayerId,
					Health:   100,
					Score:    0,
				}
				players[action.PlayerId] = player
			}

			switch a := action.Action.(type) {
			case *gen.PlayerAction_Move:
				player.X = a.Move.X
				player.Y = a.Move.Y
				player.Z = a.Move.Z
			case *gen.PlayerAction_Attack:
				player.Score += 10
			case *gen.PlayerAction_UseItem:
				player.Health = min(100, player.Health+20)
			}

			// Send updated game state
			stateNum++
			var playerStates []*gen.PlayerState
			for _, p := range players {
				playerStates = append(playerStates, p)
			}

			select {
			case s.responses <- &gen.GameState{
				StateId:   fmt.Sprintf("state-%d", stateNum),
				Timestamp: time.Now().UnixMilli(),
				Players:   playerStates,
			}:
			case <-s.ctx.Done():
				return
			}
		}
	}
}

func (s *MockBidiStream) Send(action *gen.PlayerAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("stream closed")
	}

	select {
	case s.actions <- action:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *MockBidiStream) Recv() (*gen.GameState, error) {
	select {
	case state, ok := <-s.responses:
		if !ok {
			return nil, io.EOF
		}
		return state, nil
	case <-s.ctx.Done():
		return nil, io.EOF
	}
}

func (s *MockBidiStream) CloseSend() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		s.closed = true
		close(s.actions)
	}
	return nil
}

// grpc.ClientStream implementation
func (s *MockBidiStream) Header() (metadata.MD, error) { return nil, nil }
func (s *MockBidiStream) Trailer() metadata.MD         { return nil }
func (s *MockBidiStream) Context() context.Context     { return s.ctx }
func (s *MockBidiStream) SendMsg(m any) error          { return nil }
func (s *MockBidiStream) RecvMsg(m any) error          { return nil }

// Verify interface compliance
var _ grpcws.BidiStream[*gen.PlayerAction, *gen.GameState] = (*MockBidiStream)(nil)

// Compile-time check that mock streams implement grpc.ClientStream
var _ grpc.ClientStream = (*MockServerStream)(nil)
var _ grpc.ClientStream = (*MockClientStream)(nil)
var _ grpc.ClientStream = (*MockBidiStream)(nil)
