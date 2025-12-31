# ServiceKit Enhancement Plan

## Overview

This plan outlines the enhancement of ServiceKit to:
1. **Decouple message encoding from transport** via a Codec interface
2. **Add gRPC-over-WebSocket support** for robust streaming RPCs
3. **Support all gRPC streaming modes** (server, client, bidirectional)
4. **Provide type-safe generics** throughout the stack

## Motivation

### Current Limitations

1. **JSONConn couples encoding with transport** - Can't easily switch to protobuf, binary, etc.
2. **grpc-gateway streaming has robustness gaps**:
   - No heartbeat/ping-pong mechanism
   - No timeout detection for dead connections
   - No lifecycle hooks (OnStart, OnClose, OnError)
   - SSE is unidirectional (server→client only)
   - No connection tracking or graceful shutdown
3. **No type safety** - Current `any` type requires runtime casting

### Goals

- Swap encoding (JSON, ProtoJSON, Binary Proto) without changing connection logic
- Full WebSocket lifecycle management for gRPC streams
- Type-safe message handling via generics
- Support server, client, and bidirectional streaming modes
- Coexist with grpc-gateway (separate `/ws/` route prefix)

---

## Architecture

### New Package Structure

```
servicekit/
├── http/
│   ├── codec.go          # NEW: Codec interface + implementations
│   ├── baseconn.go       # NEW: Generic BaseConn[I, O]
│   ├── ws.go             # MODIFY: Update interfaces, remove JSONConn
│   ├── bidir.go          # UNCHANGED
│   ├── utils.go          # UNCHANGED
│   └── ...
├── grpcws/               # NEW PACKAGE
│   ├── doc.go            # Package documentation
│   ├── conn.go           # Base gRPC-WS connection
│   ├── server_stream.go  # Server streaming support
│   ├── client_stream.go  # Client streaming support
│   ├── bidi_stream.go    # Bidirectional streaming support
│   └── handler.go        # Handler factories
├── cmd/
│   ├── timews/           # UPDATE existing example
│   │   └── main.go
│   └── grpcws-demo/      # NEW example
│       └── main.go
└── README.md             # UPDATE documentation
```

---

## Phase 1: Codec Interface & BaseConn

### 1.1 Codec Interface (`http/codec.go`)

```go
package http

import "github.com/gorilla/websocket"

// MessageType for WebSocket frame type
type MessageType int

const (
    TextMessage   MessageType = websocket.TextMessage   // 1
    BinaryMessage MessageType = websocket.BinaryMessage // 2
)

// Codec handles encoding/decoding of messages over WebSocket
type Codec[I any, O any] interface {
    // Decode reads raw WebSocket data into typed input message
    Decode(data []byte, msgType MessageType) (I, error)

    // Encode converts typed output message to raw bytes
    Encode(msg O) ([]byte, MessageType, error)

    // EncodePing creates a ping message
    EncodePing(pingId int64, connId string, name string) ([]byte, MessageType, error)
}
```

### 1.2 Built-in Codecs

| Codec | Input Type | Output Type | Wire Format | Use Case |
|-------|------------|-------------|-------------|----------|
| `JSONCodec` | `any` | `any` | JSON text | Dynamic messages, debugging |
| `TypedJSONCodec[I, O]` | `I` | `O` | JSON text | Typed Go structs |
| `ProtoJSONCodec[I, O]` | `I proto.Message` | `O proto.Message` | JSON text | Proto messages, human-readable |
| `BinaryProtoCodec[I, O]` | `I proto.Message` | `O proto.Message` | Binary | Proto messages, max efficiency |

### 1.3 BaseConn (`http/baseconn.go`)

Replace `JSONConn` with generic `BaseConn[I, O]`:

```go
type BaseConn[I any, O any] struct {
    Codec     Codec[I, O]
    Writer    *conc.Writer[conc.Message[O]]
    NameStr   string
    ConnIdStr string
    PingId    int64
    wsConn    *websocket.Conn
}
```

Key methods:
- `ReadMessage(conn) (I, error)` - Uses Codec.Decode
- `OnStart(conn) error` - Creates Writer with Codec.Encode
- `SendPing() error` - Uses Codec.EncodePing
- `SendOutput(msg O)` - Helper to send typed messages
- `SendError(err error)` - Helper to send errors

### 1.4 Convenience Type Aliases

```go
// For untyped JSON (simple use cases)
type JSONConn = BaseConn[any, any]

func NewJSONConn() *JSONConn {
    return &JSONConn{Codec: &JSONCodec{}}
}
```

---

## Phase 2: gRPC-WebSocket Package (`grpcws/`)

### 2.1 Message Protocol

All gRPC-WS messages follow this envelope:

```json
// Server → Client
{"type": "data", "data": <proto-as-json>}
{"type": "error", "error": "message", "code": "GRPC_CODE"}
{"type": "stream_end"}
{"type": "ping", "pingId": 123, "connId": "abc", "name": "GameSync"}

// Client → Server
{"type": "data", "data": <proto-as-json>}
{"type": "pong", "pingId": 123}
{"type": "cancel"}
{"type": "end_send"}  // For client/bidi streaming: half-close
```

### 2.2 Server Streaming (`grpcws/server_stream.go`)

For RPCs like: `rpc Subscribe(Req) returns (stream Resp)`

```go
type ServerStreamConn[Req proto.Message, Resp proto.Message] struct {
    gohttp.BaseConn[any, any]  // Control messages in, wrapped responses out

    recvFunc    func() (Resp, error)
    streamCtx   context.Context
    cancelFunc  context.CancelFunc

    // Metrics
    connectedAt time.Time
    msgsSent    int64
}

type ServerStreamHandler[Req, Resp proto.Message, Stream interface {
    Recv() (Resp, error)
    grpc.ClientStream
}] struct {
    CreateStream func(ctx context.Context, req Req) (Stream, error)
    ParseRequest func(r *http.Request) (Req, error)
}
```

### 2.3 Client Streaming (`grpcws/client_stream.go`)

For RPCs like: `rpc SendCommands(stream Req) returns (Resp)`

```go
type ClientStreamConn[Req proto.Message, Resp proto.Message] struct {
    gohttp.BaseConn[any, any]

    sendFunc     func(Req) error
    closeAndRecv func() (Resp, error)
    streamCtx    context.Context
    cancelFunc   context.CancelFunc
    parseMessage func(any) (Req, error)

    msgsReceived int64
}
```

### 2.4 Bidirectional Streaming (`grpcws/bidi_stream.go`)

For RPCs like: `rpc SyncGame(stream Req) returns (stream Resp)`

```go
type BidiStreamConn[Req proto.Message, Resp proto.Message] struct {
    gohttp.BaseConn[any, any]

    sendFunc     func(Req) error
    recvFunc     func() (Resp, error)
    streamCtx    context.Context
    cancelFunc   context.CancelFunc
    parseMessage func(any) (Req, error)

    msgsSent     int64
    msgsReceived int64
}
```

### 2.5 Handler Factories (`grpcws/handler.go`)

Convenience functions to create handlers:

```go
// Server streaming
func NewServerStreamHandler[Req, Resp proto.Message, Stream ServerStream[Resp]](
    createStream func(context.Context, Req) (Stream, error),
    parseRequest func(*http.Request) (Req, error),
) *ServerStreamHandler[Req, Resp, Stream]

// Client streaming
func NewClientStreamHandler[Req, Resp proto.Message, Stream ClientStream[Req, Resp]](
    createStream func(context.Context) (Stream, error),
    parseMessage func(any) (Req, error),
) *ClientStreamHandler[Req, Resp, Stream]

// Bidirectional streaming
func NewBidiStreamHandler[Req, Resp proto.Message, Stream BidiStream[Req, Resp]](
    createStream func(context.Context) (Stream, error),
    parseMessage func(any) (Req, error),
) *BidiStreamHandler[Req, Resp, Stream]
```

---

## Phase 3: Route Registration

### Strategy: Separate `/ws/` Prefix

```go
func SetupRoutes(
    router *mux.Router,
    grpcClient pb.GameSyncServiceClient,
    gwMux *runtime.ServeMux,
) {
    // WebSocket endpoints on /ws/v1/...
    wsRouter := router.PathPrefix("/ws").Subrouter()

    // Server streaming
    wsRouter.HandleFunc("/v1/games/{game_id}/subscribe",
        gohttp.WSServe(
            grpcws.NewServerStreamHandler(
                func(ctx context.Context, req *pb.SubscribeRequest) (pb.GameSyncService_SubscribeClient, error) {
                    return grpcClient.Subscribe(ctx, req)
                },
                parseSubscribeRequest,
            ),
            nil,
        ),
    )

    // Bidirectional streaming
    wsRouter.HandleFunc("/v1/games/{game_id}/sync",
        gohttp.WSServe(
            grpcws.NewBidiStreamHandler(
                func(ctx context.Context) (pb.GameSyncService_SyncGameClient, error) {
                    return grpcClient.SyncGame(ctx)
                },
                parsePlayerAction,
            ),
            nil,
        ),
    )

    // grpc-gateway handles REST/SSE on /v1/...
    router.PathPrefix("/v1/").Handler(gwMux)
}
```

### Client Connection URLs

| Mode | WebSocket URL | grpc-gateway URL |
|------|---------------|------------------|
| Server Stream | `wss://api.example.com/ws/v1/games/123/subscribe` | `https://api.example.com/v1/games/123/subscribe` (SSE) |
| Bidi Stream | `wss://api.example.com/ws/v1/games/123/sync` | N/A (not supported by SSE) |

---

## Phase 4: Examples

### 4.1 Update `cmd/timews/main.go`

Update to use new `BaseConn` with explicit codec:

```go
type TimeConn struct {
    gohttp.BaseConn[any, any]
    handler *TimeHandler
}

func (t *TimeHandler) Validate(w http.ResponseWriter, r *http.Request) (*TimeConn, bool) {
    return &TimeConn{
        BaseConn: gohttp.BaseConn[any, any]{
            Codec: &gohttp.JSONCodec{},
        },
        handler: t,
    }, true
}
```

### 4.2 New `cmd/grpcws-demo/main.go`

Demonstrate all three streaming modes with a mock game service.

---

## Phase 5: Documentation Updates

### 5.1 README.md Sections to Update

1. **Architecture** - Add Codec interface diagram
2. **Basic Usage** - Show `BaseConn` with `JSONCodec`
3. **Typed Messages** - Show `TypedJSONCodec`
4. **Protobuf Support** - New section for `ProtoJSONCodec` and `BinaryProtoCodec`
5. **gRPC-WebSocket** - New major section
6. **Examples** - Update all examples

### 5.2 New Documentation Files

- `grpcws/README.md` - gRPC-WebSocket package documentation
- `docs/MIGRATION.md` - Migration guide from old JSONConn

---

## Implementation Order

### Step 1: Core Codec Infrastructure
- [x] Create `http/codec.go` with Codec interface
- [x] Implement `JSONCodec`
- [x] Implement `TypedJSONCodec[I, O]`
- [x] Implement `ProtoJSONCodec[I, O]`
- [x] Implement `BinaryProtoCodec[I, O]`

### Step 2: BaseConn Refactor
- [x] Create `http/baseconn.go` with `BaseConn[I, O]`
- [x] Update `http/ws.go` to use BaseConn
- [x] Remove old JSONConn (alias to BaseConn[any, any])
- [x] Update tests

### Step 3: gRPC-WebSocket Package
- [x] Create `grpcws/doc.go`
- [x] Create `grpcws/conn.go` (shared utilities)
- [x] Create `grpcws/server_stream.go`
- [x] Create `grpcws/client_stream.go`
- [x] Create `grpcws/bidi_stream.go`
- [x] Add tests

### Step 4: Examples
- [x] Update `cmd/timews/main.go`
- [x] Create `cmd/grpcws-demo/main.go` with real proto files

### Step 5: Documentation
- [x] Update `README.md`
- [x] Create `grpcws/README.md`
- [x] Update code comments/godoc

### Step 6: Testing & Validation
- [x] Update `http/ws_test.go`
- [x] Update `http/wsextra_test.go`
- [x] Add `grpcws/` tests
- [x] Run full test suite
- [ ] Manual testing with browser client

---

## File Changes Summary

| File | Action | Description |
|------|--------|-------------|
| `http/codec.go` | CREATE | Codec interface + all implementations |
| `http/baseconn.go` | CREATE | Generic BaseConn[I, O] |
| `http/ws.go` | MODIFY | Remove JSONConn, update to use BaseConn |
| `http/ws_test.go` | MODIFY | Update tests for new types |
| `http/wsextra_test.go` | MODIFY | Update tests for new types |
| `grpcws/doc.go` | CREATE | Package documentation |
| `grpcws/conn.go` | CREATE | Shared connection utilities |
| `grpcws/server_stream.go` | CREATE | Server streaming support |
| `grpcws/client_stream.go` | CREATE | Client streaming support |
| `grpcws/bidi_stream.go` | CREATE | Bidirectional streaming |
| `grpcws/handler.go` | CREATE | Handler factory functions |
| `grpcws/grpcws_test.go` | CREATE | Package tests |
| `cmd/timews/main.go` | MODIFY | Use new BaseConn |
| `cmd/grpcws-demo/main.go` | CREATE | gRPC-WS demo |
| `README.md` | MODIFY | Full documentation update |
| `grpcws/README.md` | CREATE | gRPC-WS documentation |

---

## Testing Strategy

### Unit Tests
- Codec encode/decode for each implementation
- BaseConn lifecycle (OnStart, OnClose, etc.)
- Message type handling

### Integration Tests
- WebSocket connection with each codec
- gRPC stream forwarding
- Ping/pong mechanism
- Timeout handling
- Error recovery

### Load Tests
- Concurrent connections
- Message throughput
- Memory usage under load

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing users | High | Clear migration guide, version bump |
| Generic complexity | Medium | Good documentation, helper functions |
| Proto dependency | Low | Make proto codecs optional (build tags) |
| Performance regression | Medium | Benchmark before/after |

---

## Success Criteria

1. All existing tests pass (with updates)
2. New codec tests achieve >90% coverage
3. grpcws package tests achieve >80% coverage
4. Examples run successfully
5. Documentation is complete and accurate
6. No performance regression (benchmark validation)

---

## Timeline Estimate

| Phase | Effort |
|-------|--------|
| Phase 1: Codec + BaseConn | ~2-3 hours |
| Phase 2: grpcws package | ~3-4 hours |
| Phase 3: Route integration | ~1 hour |
| Phase 4: Examples | ~1-2 hours |
| Phase 5: Documentation | ~2 hours |
| Phase 6: Testing | ~2-3 hours |
| **Total** | **~12-15 hours** |

---

## Next Steps

1. Review and approve this plan
2. Begin with Phase 1: Create `http/codec.go`
3. Iterate through implementation steps
4. Checkpoint progress in this document

---

## Progress Log

### 2025-12-30: Core Implementation Complete
- Created `http/codec.go` with Codec interface and 4 implementations
- Created `http/baseconn.go` with generic BaseConn[I, O]
- Refactored `http/ws.go` - JSONConn is now alias to BaseConn[any, any]
- Updated all tests in `http/wsextra_test.go`
- Updated `cmd/timews/main.go` example
- Created `grpcws/` package with:
  - `doc.go` - Package documentation
  - `conn.go` - Shared types (ControlMessage, GRPCWSCodec, StreamMetrics)
  - `server_stream.go` - Server streaming support
  - `client_stream.go` - Client streaming support
  - `bidi_stream.go` - Bidirectional streaming support
- All tests passing

### 2025-12-30: grpcws-demo Complete
- Created `cmd/grpcws-demo/` with real protobuf files:
  - `pb/game.proto` - Game service with all 3 streaming patterns
  - `buf.yaml` and `buf.gen.yaml` - Buf configuration
  - `gen/` - Generated Go code from proto
  - `main.go` - Demo server with mock gRPC streams
  - `index.html` - Browser test UI
- Updated grpc-go to v1.78.0 (required for generic streaming interfaces)
- Fixed pre-existing `JsonToQueryString` test flake (map ordering)
- All tests passing

### 2025-12-30: Documentation & Tests Complete
- Updated `README.md` with:
  - Codec system documentation
  - BaseConn[I, O] generic type documentation
  - grpcws package usage examples
- Created `grpcws/README.md` with comprehensive package documentation
- Added `grpcws/grpcws_test.go` with tests for:
  - GRPCWSCodec encode/decode
  - StreamMetrics
  - ControlMessage marshaling
  - Interface compliance
- All tests passing

### 2025-12-30: Code Comments & Godoc Complete
- Enhanced documentation in `http/doc.go` (added Codec system to Key Features)
- Enhanced `http/bidir.go` with comprehensive interface documentation
- Enhanced `http/utils.go` with function documentation and examples
- Enhanced `http/ws.go` with interface and function documentation
- All tests passing

### Remaining Work
- Manual testing with browser client (optional)

*Last Updated: 2025-12-30*
