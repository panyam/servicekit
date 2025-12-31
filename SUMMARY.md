# ServiceKit Summary

## Purpose

ServiceKit provides production-grade WebSocket infrastructure for Go applications, with special focus on:

1. **Real-time communication** - Chat, gaming, live updates
2. **gRPC streaming over WebSocket** - Browser-compatible gRPC streaming
3. **Connection robustness** - Heartbeats, timeouts, graceful shutdown

## Recent Changes (2025-12-30)

### Codec System
Added pluggable message encoding via `Codec[I, O]` interface:
- Decouples encoding from transport
- Four built-in codecs: JSON, TypedJSON, ProtoJSON, BinaryProto
- Easy to add custom codecs

### Generic BaseConn
Refactored to `BaseConn[I, O]` for type-safe message handling:
- `JSONConn` is now an alias for `BaseConn[any, any]`
- Full backward compatibility maintained

### grpcws Package
New package for gRPC-over-WebSocket:
- Server streaming, client streaming, bidirectional streaming
- JSON envelope protocol with protojson payloads
- Full lifecycle management (heartbeats, cancellation, graceful close)

### TypeScript Client (`clients/typescript/`)
New TypeScript client library (`@panyam/servicekit-client`):
- **BaseWSClient**: Low-level WebSocket with auto ping/pong (for http/JSONConn)
- **GRPCWSClient**: gRPC-style streaming with envelope protocol (for grpcws)
- **TypedGRPCWSClient<TIn, TOut>**: Type-safe wrapper for protobuf types
- Works with any TS protoc plugin (@bufbuild/protobuf, ts-proto, protobuf-ts)

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
| `http/codec.go` | Codec interface + 4 implementations |
| `http/baseconn.go` | Generic BaseConn[I, O] |
| `http/ws.go` | WSServe, WSConn interface, JSONConn alias |
| `grpcws/*.go` | gRPC streaming over WebSocket |
| `clients/typescript/` | TypeScript client library |
| `cmd/grpcws-demo/` | Working demo with proto files |
