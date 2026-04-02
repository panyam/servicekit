# ServiceKit Architecture

## Overview

ServiceKit is a Go library providing production-grade WebSocket and SSE (Server-Sent Events) infrastructure for real-time applications. It builds on top of the Gorilla WebSocket library with abstractions for concurrent message handling, connection lifecycle management, gRPC streaming integration, and server-push via SSE.

## Core Design Principles

### 1. Separation of Concerns

The architecture separates:
- **Transport** (WebSocket connection handling) from **Encoding** (message serialization)
- **Connection management** (lifecycle, heartbeats) from **Business logic** (message handling)
- **Generic infrastructure** (`BaseConn[I, O]`) from **Protocol-specific implementations** (`grpcws`)

### 2. Generic Type Safety

The library uses Go generics extensively:
- `BaseConn[I, O]` - Input type `I`, Output type `O`
- `Codec[I, O]` - Encoder/decoder for message types
- Stream handlers parameterized by proto message types

### 3. Composable Lifecycle Hooks

Connections implement lifecycle interfaces:
```
OnStart(conn) → HandleMessage(msg) → OnClose()
                     ↓
                OnError(err)
                     ↓
                OnTimeout()
```

## Package Structure

```
servicekit/
├── http/                    # Core WebSocket + SSE infrastructure
│   ├── codec.go            # Codec interface + implementations
│   ├── baseconn.go         # Generic BaseConn[I, O]
│   ├── ws.go               # WebSocket serving, WSConn interface
│   ├── bidir.go            # BiDirStreamConn interface
│   ├── sseconn.go          # SSEConn[O], BaseSSEConn[O], SSEServe
│   ├── ssehub.go           # SSEHub[O] session manager
│   └── utils.go            # HTTP/WS utilities
│
├── grpcws/                  # gRPC-over-WebSocket
│   ├── conn.go             # Shared types, GRPCWSCodec
│   ├── server_stream.go    # Server streaming support
│   ├── client_stream.go    # Client streaming support
│   └── bidi_stream.go      # Bidirectional streaming
│
├── middleware/               # HTTP middleware (Guard, rate limit, CORS, etc.)
│
├── auth/                    # Authentication utilities
│   └── flask/              # Flask session compatibility
│
└── cmd/                     # Examples
    ├── timews/             # Basic WebSocket example
    └── grpcws-demo/        # gRPC streaming demo
```

## Key Components

### SSE (Server-Sent Events)

The `http` package provides SSE support via `SSEConn[O]` and `SSEHub[O]`, mirroring the WebSocket patterns:

**SSEConn[O]** — Write-only counterpart to BaseConn[I, O]:
```
HTTP Request → SSEHandler.Validate() → SSEConn created
    → Set SSE headers (text/event-stream, no-cache)
    → OnStart(w, r) → [keepalive/sends...] → client disconnect → OnClose()
```

- Uses `conc.Writer[SSEOutgoingMessage[O]]` for thread-safe writes (same pattern as BaseConn)
- Uses `Codec.Encode` for serializing typed output to SSE data fields
- Keepalive via SSE comments (`: keepalive\n\n`) at configurable interval
- Context-aware: tied to `r.Context()`, cleans up on client disconnect
- Per WHATWG SSE spec: https://html.spec.whatwg.org/multipage/server-sent-events.html

**SSEHub[O]** — Session manager for SSE connections:
- Instance-based (not global) for testability
- Register/Unregister connections by ConnId
- Send (targeted) and Broadcast (all) with optional event types
- CloseAll for graceful shutdown

| BaseConn[I, O] (WebSocket) | BaseSSEConn[O] (SSE) |
|---|---|
| Read + Write | Write only |
| WebSocket ping/pong frames | `: keepalive` SSE comments |
| `conc.Writer[OutgoingMessage[O]]` | `conc.Writer[SSEOutgoingMessage[O]]` |
| `OnStart(*websocket.Conn)` | `OnStart(ResponseWriter, *Request)` |
| `WSServe()` handler factory | `SSEServe()` handler factory |

**Important:** SSE endpoints require `http.Server.WriteTimeout = 0` to prevent the server from closing long-lived connections. See `middleware.ApplyDefaults` documentation.

### Transport/Codec Separation

The architecture cleanly separates **transport concerns** from **encoding concerns**:

**Transport Layer** (BaseConn, grpcws):
- WebSocket connection management
- Ping/pong heartbeats (always JSON)
- Error messages (always JSON)
- Thread-safe writes via OutgoingMessage union type

**Codec Layer** (Codec interface):
- Data message encoding/decoding only
- No knowledge of control messages

This separation ensures control messages work consistently even with binary protocols.

### Codec System

The `Codec[I, O]` interface handles **data messages only**:

```go
type Codec[I any, O any] interface {
    Decode(data []byte, msgType MessageType) (I, error)
    Encode(msg O) ([]byte, MessageType, error)
}
```

**Built-in implementations:**
| Codec | Use Case |
|-------|----------|
| `JSONCodec` | Dynamic JSON messages |
| `TypedJSONCodec[I, O]` | Typed Go structs |
| `ProtoJSONCodec[I, O]` | Protobuf with JSON wire format |
| `BinaryProtoCodec[I, O]` | Protobuf with binary wire format |

### BaseConn[I, O]

Generic connection type that handles:
- WebSocket read/write with codec
- Thread-safe writes via `OutgoingMessage[O]` union type
- Ping-pong heartbeat mechanism (transport-level, always JSON)
- Connection identification (`Name()`, `ConnId()`)

```go
type BaseConn[I any, O any] struct {
    Codec     Codec[I, O]
    Writer    *conc.Writer[OutgoingMessage[O]]
    NameStr   string
    ConnIdStr string
    PingId    int64
}

// OutgoingMessage is a union type for all outgoing messages
type OutgoingMessage[O any] struct {
    Data  *O        // Data message (uses codec)
    Ping  *PingData // Ping message (always JSON)
    Error error     // Error message (always JSON)
}
```

All writes go through the Writer with this union type, ensuring:
- Thread-safe concurrent writes
- Consistent handling of all message types
- No direct WebSocket writes that could cause race conditions

### gRPC-WebSocket Bridge

The `grpcws` package wraps gRPC client streams for WebSocket transport:

```
Browser → WebSocket → grpcws Handler → gRPC Stream → gRPC Server
                ↓
         JSON Envelope Protocol
         {"type": "data", "data": {...}}
```

**Stream interfaces:**
- `ServerStream[Resp]` - `Recv() (Resp, error)`
- `ClientStream[Req, Resp]` - `Send(Req)`, `CloseAndRecv()`
- `BidiStream[Req, Resp]` - `Send(Req)`, `Recv()`, `CloseSend()`

### Middleware Package

The `middleware` package provides composable HTTP middleware with no application-specific imports.

**Key design decisions:**

1. **Instance-based `ClientIPExtractor`**: Replaces the package-global `trustedProxyCIDRs` pattern with an instance that holds its own CIDR list. A package-level `ClientIP(r)` convenience function uses a default extractor for backwards compatibility. This eliminates concurrency hazards when multiple listeners need different trust configs.

2. **Generalized `RateLimiter` with `KeyFunc`**: The `Middleware(keyFunc)` method accepts a `func(*http.Request) string` that extracts the rate limit key. `nil` defaults to `ClientIP`. This generalizes per-IP limiting to per-anything (per-user, per-tenant, per-API-key).

3. **`Guard` as middleware chain**: `Use(mw...)` adds middleware functions; `Wrap(h)` applies them. The first `Use`'d middleware is outermost. Nil middleware are silently skipped. Applications compose their own chain order.

4. **`RequestID` injects into context**: The `RequestID` middleware stores the ID in the request context. `RequestLogger` automatically includes `request_id` in log output when present, so placing `RequestID` before `RequestLogger` in the Guard chain gives correlated logs for free.

5. **`HealthCheck` is an `http.Handler`, not middleware**: It gets mounted directly on a mux (`mux.Handle(hc.Path(), hc)`), not chained via Guard. This avoids health probes triggering auth, rate limiting, or logging.

6. **`ApplyDefaults` is a helper function, not middleware**: It mutates zero-valued timeout fields on `*http.Server`. SSE/streaming callers must set `WriteTimeout = 0` separately.

```
servicekit/middleware/
├── clientip.go        # ClientIPExtractor (instance-based)
├── ratelimit.go       # RateLimiter (global + per-key, KeyFunc)
├── connlimit.go       # ConnLimiter (atomic counter)
├── bodylimit.go       # BodyLimiter (request body size via MaxBytesReader)
├── origin.go          # OriginChecker (WebSocket origin allowlist)
├── cors.go            # CORS (origin-aware, reuses OriginChecker)
├── requestid.go       # RequestID (X-Request-Id generation/propagation)
├── logging.go         # RequestLogger (slog structured logging, includes request ID)
├── recovery.go        # Recovery (panic → 500 + stack trace)
├── health.go          # HealthCheck (health/readiness endpoint handler)
├── servertimeouts.go  # ApplyDefaults (http.Server timeout defaults)
├── guard.go           # Guard (composable middleware chain)
└── doc.go             # Package godoc
```

## Message Flow

### Standard WebSocket

```
1. HTTP Request → WSHandler.Validate() → WSConn created
2. Upgrade to WebSocket
3. WSConn.OnStart(conn) → Initialize writer, start ping timer
4. Loop: ReadMessage() → HandleMessage(msg)
5. On error/close: OnClose() cleanup
```

### gRPC-WebSocket

```
1. HTTP Request → grpcws.Handler.Validate()
2. Create gRPC stream via CreateStream()
3. Upgrade to WebSocket
4. Start response forwarding goroutine
5. Handle incoming messages: data → gRPC Send, control → handle locally
6. On stream end: send stream_end, cleanup
```

## Concurrency Model

**WebSocket:**
- **Reader**: Single goroutine reads from WebSocket
- **Writer**: `conc.Writer` handles concurrent sends safely
- **gRPC forwarding**: Separate goroutine for stream responses
- **Ping timer**: Background timer for heartbeats

**SSE:**
- **Writer**: `conc.Writer` handles concurrent sends (same as WebSocket)
- **Keepalive ticker**: Background timer sends SSE comments
- **No reader**: SSE is unidirectional (server → client)
- **Context**: `r.Context().Done()` detects client disconnect

## Configuration

### WSConnConfig

```go
type WSConnConfig struct {
    BiDirStreamConfig *BiDirStreamConfig
    Upgrader          websocket.Upgrader
}

type BiDirStreamConfig struct {
    PingPeriod time.Duration  // How often to send pings
    PongPeriod time.Duration  // Timeout for pong response
}
```

## Extension Points

1. **Custom Codec**: Implement `Codec[I, O]` for new serialization formats
2. **Custom WS Connection**: Embed `BaseConn` and override lifecycle methods
3. **Custom SSE Connection**: Embed `BaseSSEConn` and override `OnStart`/`OnClose`
4. **Custom Handler**: Implement `WSHandler` or `SSEHandler` for request validation/auth
5. **SSE Hub**: Use `SSEHub[O]` for session management, or build custom on top of `BaseSSEConn`
6. **Stream Wrappers**: Implement stream interfaces for custom gRPC patterns
