# ServiceKit Architecture

## Overview

ServiceKit is a Go library providing production-grade WebSocket infrastructure for real-time applications. It builds on top of the Gorilla WebSocket library with abstractions for concurrent message handling, connection lifecycle management, and gRPC streaming integration.

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
├── http/                    # Core WebSocket infrastructure
│   ├── codec.go            # Codec interface + implementations
│   ├── baseconn.go         # Generic BaseConn[I, O]
│   ├── ws.go               # WebSocket serving, WSConn interface
│   ├── bidir.go            # BiDirStreamConn interface
│   └── utils.go            # HTTP/WS utilities
│
├── grpcws/                  # gRPC-over-WebSocket
│   ├── conn.go             # Shared types, GRPCWSCodec
│   ├── server_stream.go    # Server streaming support
│   ├── client_stream.go    # Client streaming support
│   └── bidi_stream.go      # Bidirectional streaming
│
├── auth/                    # Authentication utilities
│   └── flask/              # Flask session compatibility
│
└── cmd/                     # Examples
    ├── timews/             # Basic WebSocket example
    └── grpcws-demo/        # gRPC streaming demo
```

## Key Components

### Codec System

The `Codec[I, O]` interface decouples message encoding from transport:

```go
type Codec[I any, O any] interface {
    Decode(data []byte, msgType MessageType) (I, error)
    Encode(msg O) ([]byte, MessageType, error)
    EncodePing(pingId int64, connId, name string) ([]byte, MessageType, error)
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
- Concurrent message writing via `conc.Writer`
- Ping-pong heartbeat mechanism
- Connection identification (`Name()`, `ConnId()`)

```go
type BaseConn[I any, O any] struct {
    Codec     Codec[I, O]
    Writer    *conc.Writer[conc.Message[O]]
    NameStr   string
    ConnIdStr string
    PingId    int64
}
```

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

- **Reader**: Single goroutine reads from WebSocket
- **Writer**: `conc.Writer` handles concurrent sends safely
- **gRPC forwarding**: Separate goroutine for stream responses
- **Ping timer**: Background timer for heartbeats

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
2. **Custom Connection**: Embed `BaseConn` and override lifecycle methods
3. **Custom Handler**: Implement `WSHandler` for request validation/auth
4. **Stream Wrappers**: Implement stream interfaces for custom gRPC patterns
