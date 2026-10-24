// grpcws-demo demonstrates the grpcws package for gRPC-over-WebSocket streaming.
//
// This demo shows multiplayer game sync where multiple browser windows
// connected to the same gameId see each other's actions in real-time.
//
// Run: go run ./cmd/grpcws-demo
// Test: Open http://localhost:8080/game123 in multiple browser windows
// Metrics: http://localhost:8080/metrics
// Exercise: make exercise (go run ./cmd/grpcws-demo -exercise)
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/panyam/servicekit/cmd/grpcws-demo/gen"
	"github.com/panyam/servicekit/grpcws"
	gohttp "github.com/panyam/servicekit/http"
	"github.com/panyam/servicekit/prom"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

//go:embed index.html
var indexHTML string

// ============================================================================
// GameHub - Manages game rooms and broadcasts events to all players
// ============================================================================

type GameHub struct {
	mu    sync.RWMutex
	games map[string]*GameRoom
}

type GameRoom struct {
	mu         sync.RWMutex
	gameId     string
	players    map[string]*PlayerConn
	eventNum   int
	stateNum   int
	gameState  map[string]*gen.PlayerState
	eventsChan chan *gen.GameEvent
}

type PlayerConn struct {
	playerId string
	events   chan *gen.GameEvent
	states   chan *gen.GameState
}

var hub = &GameHub{
	games: make(map[string]*GameRoom),
}

func (h *GameHub) GetOrCreateRoom(gameId string) *GameRoom {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, exists := h.games[gameId]
	if !exists {
		room = &GameRoom{
			gameId:     gameId,
			players:    make(map[string]*PlayerConn),
			gameState:  make(map[string]*gen.PlayerState),
			eventsChan: make(chan *gen.GameEvent, 100),
		}
		h.games[gameId] = room

		// Start broadcasting events to all players
		go room.broadcastLoop()
	}
	return room
}

func (r *GameRoom) broadcastLoop() {
	for event := range r.eventsChan {
		r.mu.RLock()
		for _, player := range r.players {
			select {
			case player.events <- event:
			default:
				// Skip if player's channel is full
			}
		}
		r.mu.RUnlock()
	}
}

// AddPlayer, RemovePlayer and ProcessAction queue their event after
// unlocking: broadcastLoop takes the lock to drain eventsChan, so sending
// while holding it deadlocks once the channel fills (many players leaving
// at once).
func (r *GameRoom) AddPlayer(playerId string) *PlayerConn {
	r.mu.Lock()

	conn := &PlayerConn{
		playerId: playerId,
		events:   make(chan *gen.GameEvent, 10),
		states:   make(chan *gen.GameState, 10),
	}
	r.players[playerId] = conn

	// Initialize player state
	r.gameState[playerId] = &gen.PlayerState{
		PlayerId: playerId,
		Health:   100,
		Score:    0,
	}

	// Broadcast join event
	r.eventNum++
	event := &gen.GameEvent{
		EventId:   fmt.Sprintf("evt-%d", r.eventNum),
		EventType: "player_joined",
		Timestamp: time.Now().UnixMilli(),
		GameId:    r.gameId,
		Payload: &gen.GameEvent_PlayerJoined{
			PlayerJoined: &gen.PlayerJoined{
				PlayerId:    playerId,
				PlayerCount: int32(len(r.players)),
			},
		},
	}
	log.Printf("[%s] Player %s joined (total: %d)", r.gameId, playerId, len(r.players))
	r.mu.Unlock()

	r.eventsChan <- event
	return conn
}

func (r *GameRoom) RemovePlayer(playerId string) {
	r.mu.Lock()
	var event *gen.GameEvent
	defer func() {
		r.mu.Unlock()
		if event != nil {
			r.eventsChan <- event
		}
	}()

	if conn, exists := r.players[playerId]; exists {
		close(conn.events)
		close(conn.states)
		delete(r.players, playerId)
		delete(r.gameState, playerId)

		// Broadcast leave event
		r.eventNum++
		event = &gen.GameEvent{
			EventId:   fmt.Sprintf("evt-%d", r.eventNum),
			EventType: "player_left",
			Timestamp: time.Now().UnixMilli(),
			GameId:    r.gameId,
			Payload: &gen.GameEvent_PlayerLeft{
				PlayerLeft: &gen.PlayerLeft{
					PlayerId:    playerId,
					PlayerCount: int32(len(r.players)),
				},
			},
		}

		log.Printf("[%s] Player %s left (remaining: %d)", r.gameId, playerId, len(r.players))
	}
}

func (r *GameRoom) ProcessAction(action *gen.PlayerAction) *gen.GameState {
	r.mu.Lock()
	var event *gen.GameEvent
	defer func() {
		r.mu.Unlock()
		r.eventsChan <- event
	}()

	// Update player state
	player, exists := r.gameState[action.PlayerId]
	if !exists {
		player = &gen.PlayerState{
			PlayerId: action.PlayerId,
			Health:   100,
			Score:    0,
		}
		r.gameState[action.PlayerId] = player
	}

	actionType := "unknown"
	switch a := action.Action.(type) {
	case *gen.PlayerAction_Move:
		player.X = a.Move.X
		player.Y = a.Move.Y
		player.Z = a.Move.Z
		actionType = "move"
	case *gen.PlayerAction_Attack:
		player.Score += 10
		actionType = "attack"
	case *gen.PlayerAction_UseItem:
		player.Health = min(100, player.Health+20)
		actionType = "use_item"
	}

	// Broadcast action event to all players
	r.eventNum++
	event = &gen.GameEvent{
		EventId:   fmt.Sprintf("evt-%d", r.eventNum),
		EventType: "player_action",
		Timestamp: time.Now().UnixMilli(),
		GameId:    r.gameId,
		Payload: &gen.GameEvent_PlayerAction{
			PlayerAction: &gen.PlayerActionEvent{
				PlayerId:   action.PlayerId,
				ActionType: actionType,
				ActionId:   action.ActionId,
			},
		},
	}

	// Build current game state
	r.stateNum++
	var playerStates []*gen.PlayerState
	for _, p := range r.gameState {
		// Make a copy
		playerStates = append(playerStates, &gen.PlayerState{
			PlayerId: p.PlayerId,
			X:        p.X,
			Y:        p.Y,
			Z:        p.Z,
			Health:   p.Health,
			Score:    p.Score,
		})
	}

	state := &gen.GameState{
		StateId:   fmt.Sprintf("state-%d", r.stateNum),
		Timestamp: time.Now().UnixMilli(),
		Players:   playerStates,
	}

	// Send state to all players' state channels
	for _, conn := range r.players {
		select {
		case conn.states <- state:
		default:
		}
	}

	return state
}

// demoConfig is what main's flags and the exercise set.
type demoConfig struct {
	// ConnOptions applies to all three stream handlers.
	grpcws.ConnOptions
	// Metrics, if set, observes every connection and request; /metrics
	// serves Registry.
	Metrics  *prom.ConnMetrics
	HTTP     *prom.HTTPMetrics
	Registry *prometheus.Registry
}

func newDemoConfig(opts grpcws.ConnOptions) demoConfig {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	cfg := demoConfig{
		ConnOptions: opts,
		Metrics:     prom.NewConnMetrics(reg),
		HTTP:        prom.NewHTTPMetrics(reg),
		Registry:    reg,
	}
	cfg.ConnOptions.Observer = cfg.Metrics
	return cfg
}

func newRouter(cfg demoConfig) *mux.Router {
	router := mux.NewRouter()
	router.Use(cfg.HTTP.Middleware)
	router.Handle("/metrics", promhttp.HandlerFor(cfg.Registry, promhttp.HandlerOpts{}))

	// Serve the client library
	router.HandleFunc("/servicekit-client.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "clients/typescript/dist/servicekit-client.browser.js")
	})

	// WebSocket endpoints with gameId
	wsRouter := router.PathPrefix("/ws").Subrouter()

	// Server streaming: Subscribe to game events (broadcasts to all in same game)
	subscribe := grpcws.NewServerStreamHandler(
		func(ctx context.Context, req *gen.SubscribeRequest) (*SharedServerStream, error) {
			room := hub.GetOrCreateRoom(req.GameId)
			return NewSharedServerStream(ctx, room, req), nil
		},
		func(r *http.Request) (*gen.SubscribeRequest, error) {
			vars := mux.Vars(r)
			playerId := r.URL.Query().Get("player_id")
			if playerId == "" {
				playerId = fmt.Sprintf("player-%d", rand.Intn(10000))
			}
			return &gen.SubscribeRequest{
				GameId:   vars["gameId"],
				PlayerId: playerId,
			}, nil
		},
	)
	subscribe.ConnOptions = cfg.ConnOptions
	wsRouter.HandleFunc("/v1/{gameId}/subscribe", gohttp.WSServe(subscribe, nil))

	// Client streaming: Send commands
	commands := grpcws.NewClientStreamHandler(
		func(ctx context.Context) (*MockClientStream, error) {
			return NewMockClientStream(ctx), nil
		},
		func() *gen.GameCommand { return &gen.GameCommand{} },
	)
	commands.ConnOptions = cfg.ConnOptions
	wsRouter.HandleFunc("/v1/{gameId}/commands", gohttp.WSServe(commands, nil))

	// Bidirectional streaming: Game sync (shared state)
	syncGame := grpcws.NewBidiStreamHandler(
		func(ctx context.Context) (*SharedBidiStream, error) {
			// We'll get gameId from the first action's playerId context
			return NewSharedBidiStream(ctx), nil
		},
		func() *gen.PlayerAction { return &gen.PlayerAction{} },
	)
	syncGame.ConnOptions = cfg.ConnOptions
	wsRouter.HandleFunc("/v1/{gameId}/sync", gohttp.WSServe(syncGame, nil))

	// Serve the test UI at /{gameId}
	router.HandleFunc("/{gameId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(indexHTML))
	})

	// Redirect root to a default game
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/game-"+fmt.Sprintf("%d", rand.Intn(1000)), http.StatusTemporaryRedirect)
	})
	return router
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	writeTimeout := flag.Duration("write-timeout", 10*time.Second, "drop a client whose write takes longer than this (0: never)")
	closeOnEnd := flag.Bool("close-on-stream-end", true, "close the WebSocket when its stream ends")
	exercise := flag.Bool("exercise", false, "run the observable_relay exercise (issue 51) and exit")
	ex := exerciseFlags()
	flag.Parse()

	if *exercise {
		log.SetOutput(io.Discard)
		if err := runExercise(*ex); err != nil {
			fmt.Fprintln(os.Stderr, "exercise:", err)
			os.Exit(1)
		}
		return
	}

	cfg := newDemoConfig(grpcws.ConnOptions{CloseOnStreamEnd: *closeOnEnd, WriteTimeout: *writeTimeout})
	router := newRouter(cfg)

	log.Printf("Starting grpcws-demo server on %s", *addr)
	log.Println("")
	log.Println("Open http://localhost:8080/my-game in MULTIPLE browser windows")
	log.Println("All windows with the same game ID will see each other's actions!")
	log.Println("")
	log.Println("WebSocket endpoints:")
	log.Println("  ws://localhost:8080/ws/v1/{gameId}/subscribe  - Server streaming (shared events)")
	log.Println("  ws://localhost:8080/ws/v1/{gameId}/commands   - Client streaming")
	log.Println("  ws://localhost:8080/ws/v1/{gameId}/sync       - Bidi streaming (shared state)")
	log.Println("  http://localhost:8080/metrics                 - Prometheus metrics")

	if err := http.ListenAndServe(*addr, router); err != nil {
		log.Fatal(err)
	}
}

// ============================================================================
// SharedServerStream - Server stream that broadcasts to all players in a game
// ============================================================================

type SharedServerStream struct {
	ctx      context.Context
	cancel   context.CancelFunc
	room     *GameRoom
	playerId string
	conn     *PlayerConn
}

func NewSharedServerStream(parent context.Context, room *GameRoom, req *gen.SubscribeRequest) *SharedServerStream {
	ctx, cancel := context.WithCancel(parent)
	conn := room.AddPlayer(req.PlayerId)

	return &SharedServerStream{
		ctx:      ctx,
		cancel:   cancel,
		room:     room,
		playerId: req.PlayerId,
		conn:     conn,
	}
}

func (s *SharedServerStream) Recv() (*gen.GameEvent, error) {
	select {
	case <-s.ctx.Done():
		s.room.RemovePlayer(s.playerId)
		return nil, io.EOF
	case event, ok := <-s.conn.events:
		if !ok {
			return nil, io.EOF
		}
		return event, nil
	}
}

func (s *SharedServerStream) Header() (metadata.MD, error) { return nil, nil }
func (s *SharedServerStream) Trailer() metadata.MD         { return nil }
func (s *SharedServerStream) CloseSend() error {
	s.room.RemovePlayer(s.playerId)
	return nil
}
func (s *SharedServerStream) Context() context.Context { return s.ctx }
func (s *SharedServerStream) SendMsg(m any) error      { return nil }
func (s *SharedServerStream) RecvMsg(m any) error      { return nil }

var _ grpcws.ServerStream[*gen.GameEvent] = (*SharedServerStream)(nil)

// ============================================================================
// SharedBidiStream - Bidi stream with shared game state
// ============================================================================

type SharedBidiStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	room      *GameRoom
	playerId  string
	conn      *PlayerConn
	closed    bool
	mu        sync.Mutex
	gameIdSet bool
}

func NewSharedBidiStream(parent context.Context) *SharedBidiStream {
	ctx, cancel := context.WithCancel(parent)
	return &SharedBidiStream{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *SharedBidiStream) Send(action *gen.PlayerAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("stream closed")
	}

	// First action sets the game context
	if !s.gameIdSet {
		// Extract gameId from playerId format: "gameId:playerId"
		// Or use a default game
		gameId := "default"
		if action.PlayerId != "" {
			// For demo, use first 8 chars as gameId
			gameId = "shared-game"
		}
		s.room = hub.GetOrCreateRoom(gameId)
		s.playerId = action.PlayerId
		s.conn = s.room.AddPlayer(action.PlayerId)
		s.gameIdSet = true
	}

	// Process action in the shared room
	s.room.ProcessAction(action)
	return nil
}

func (s *SharedBidiStream) Recv() (*gen.GameState, error) {
	// Wait for game to be set up
	for !s.gameIdSet {
		select {
		case <-s.ctx.Done():
			return nil, io.EOF
		case <-time.After(100 * time.Millisecond):
		}
	}

	select {
	case state, ok := <-s.conn.states:
		if !ok {
			return nil, io.EOF
		}
		return state, nil
	case <-s.ctx.Done():
		return nil, io.EOF
	}
}

func (s *SharedBidiStream) CloseSend() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		s.closed = true
		if s.room != nil && s.playerId != "" {
			s.room.RemovePlayer(s.playerId)
		}
	}
	return nil
}

func (s *SharedBidiStream) Header() (metadata.MD, error) { return nil, nil }
func (s *SharedBidiStream) Trailer() metadata.MD         { return nil }
func (s *SharedBidiStream) Context() context.Context     { return s.ctx }
func (s *SharedBidiStream) SendMsg(m any) error          { return nil }
func (s *SharedBidiStream) RecvMsg(m any) error          { return nil }

var _ grpcws.BidiStream[*gen.PlayerAction, *gen.GameState] = (*SharedBidiStream)(nil)

// ============================================================================
// MockClientStream - Same as before (not shared)
// ============================================================================

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

func (s *MockClientStream) Header() (metadata.MD, error) { return nil, nil }
func (s *MockClientStream) Trailer() metadata.MD         { return nil }
func (s *MockClientStream) CloseSend() error             { return nil }
func (s *MockClientStream) Context() context.Context     { return s.ctx }
func (s *MockClientStream) SendMsg(m any) error          { return nil }
func (s *MockClientStream) RecvMsg(m any) error          { return nil }

var _ grpcws.ClientStream[*gen.GameCommand, *gen.CommandSummary] = (*MockClientStream)(nil)
var _ grpc.ClientStream = (*SharedServerStream)(nil)
var _ grpc.ClientStream = (*SharedBidiStream)(nil)
var _ grpc.ClientStream = (*MockClientStream)(nil)
