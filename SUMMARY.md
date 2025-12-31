# ServiceKit Summary

## Purpose

ServiceKit provides production-grade WebSocket infrastructure for Go applications, with special focus on:

1. **Real-time communication** - Chat, gaming, live updates
2. **gRPC streaming over WebSocket** - Browser-compatible gRPC streaming
3. **Connection robustness** - Heartbeats, timeouts, graceful shutdown

## Architecture

### Layered Design

Both server and client use a two-layer architecture:

```
┌─────────────────────────────────────────────────────────────┐
│                    Transport Layer                          │
│  • WebSocket connection management                          │
│  • Ping/Pong heartbeats (always JSON)                       │
│  • Error messages (always JSON)                             │
│  • Thread-safe writes via OutgoingMessage union type        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      Codec Layer                            │
│  • Data message encoding/decoding only                      │
│  • JSON, TypedJSON, ProtoJSON, BinaryProto codecs           │
│  • Client and server must use matching codecs               │
└─────────────────────────────────────────────────────────────┘
```

**Key principle**: Control messages (ping/pong/error) are **always JSON** at the transport layer, regardless of what codec is used for data messages. This ensures consistent communication even with binary protocols.

## Recent Changes (2025-12-31)

### Transport/Codec Separation
- Removed ping handling from Codec interface (pings are transport-level)
- Added `OutgoingMessage[O]` union type for thread-safe writes
- All writes (data, pings, errors) go through serialized Writer
- Fixed concurrent write panic in WebSocket connections

### Codec System
Added pluggable message encoding via `Codec[I, O]` interface:
- Decouples encoding from transport
- Four built-in codecs: JSON, TypedJSON, ProtoJSON, BinaryProto
- Easy to add custom codecs

### Generic BaseConn
Refactored to `BaseConn[I, O]` for type-safe message handling:
- `JSONConn` is now an alias for `BaseConn[any, any]`
- Uses `OutgoingMessage[O]` for all outgoing messages
- Full backward compatibility maintained

### grpcws Package
New package for gRPC-over-WebSocket:
- Server streaming, client streaming, bidirectional streaming
- JSON envelope protocol with protojson payloads
- Full lifecycle management (heartbeats, cancellation, graceful close)

### TypeScript Client (`clients/typescript/`)
New TypeScript client library (`@panyam/servicekit-client`):
- **BaseWSClient<I, O>**: Low-level WebSocket with auto ping/pong and configurable codec
- **GRPCWSClient**: gRPC-style streaming with envelope protocol (for grpcws)
- **TypedGRPCWSClient<TIn, TOut>**: Type-safe wrapper for protobuf types
- **JSONCodec**: Default, matches server JSONCodec
- **BinaryCodec**: For binary protobuf, matches server BinaryProtoCodec
- Works with any TS protoc plugin (@bufbuild/protobuf, ts-proto, protobuf-ts)

### Multiplayer Demo (`cmd/grpcws-demo/`)
GameHub-based multiplayer demo showing real-time sync:
- **GameHub**: Manages game rooms by gameId
- **GameRoom**: Tracks players, broadcasts events to all in room
- **Routes**: `/ws/v1/{gameId}/subscribe`, `/ws/v1/{gameId}/commands`, `/ws/v1/{gameId}/sync`
- Server streaming broadcasts player join/leave/action events
- Bidi streaming shares game state across all connected players
- Client streaming remains per-connection (by design)

## Key Patterns

### Embedding BaseConn
```go
type MyConn struct {
    gohttp.BaseConn[any, any]  // or gohttp.JSONConn
    // custom fields
}

func (c *MyConn) HandleMessage(msg any) error {
    // business logic
}
```

### Creating Handlers
```go
type MyHandler struct{}

func (h *MyHandler) Validate(w http.ResponseWriter, r *http.Request) (*MyConn, bool) {
    // auth, validation
    return &MyConn{...}, true
}
```

### Serving WebSocket
```go
router.HandleFunc("/ws", gohttp.WSServe(&MyHandler{}, config))
```

## Files Overview

| File | Purpose |
|------|---------|
| `http/codec.go` | Codec interface + 4 implementations (data only) |
| `http/baseconn.go` | Generic BaseConn[I, O], OutgoingMessage union type |
| `http/ws.go` | WSServe, WSConn interface, JSONConn alias |
| `grpcws/*.go` | gRPC streaming over WebSocket |
| `clients/typescript/` | TypeScript client library with codec support |
| `cmd/grpcws-demo/` | Multiplayer game demo with GameHub |
