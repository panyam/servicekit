# grpcws - gRPC-over-WebSocket

The `grpcws` package provides WebSocket transport for gRPC streaming RPCs with full lifecycle management, heartbeat detection, and graceful error handling.

## Overview

This package bridges the gap between gRPC streaming and browser-based WebSocket clients. It wraps gRPC client streams and exposes them over WebSocket with:

- **All three streaming modes**: Server, Client, and Bidirectional streaming
- **JSON envelope protocol**: Human-readable message format with protojson payloads
- **Full lifecycle management**: Connection tracking, heartbeat ping-pong, graceful shutdown
- **Metrics tracking**: Message counts, connection duration, and more

## Installation

```go
import "github.com/panyam/servicekit/grpcws"
```

## Quick Start

### Server Streaming

For RPCs like `rpc Subscribe(Request) returns (stream Response)`:

```go
router.HandleFunc("/ws/v1/subscribe", gohttp.WSServe(
    grpcws.NewServerStreamHandler(
        // Create the gRPC stream from a request
        func(ctx context.Context, req *pb.SubscribeRequest) (pb.GameService_SubscribeClient, error) {
            return grpcClient.Subscribe(ctx, req)
        },
        // Parse the initial request from HTTP
        func(r *http.Request) (*pb.SubscribeRequest, error) {
            return &pb.SubscribeRequest{
                GameId: mux.Vars(r)["game_id"],
            }, nil
        },
    ),
    nil,
))
```

### Client Streaming

For RPCs like `rpc SendBatch(stream Request) returns (Response)`:

```go
router.HandleFunc("/ws/v1/commands", gohttp.WSServe(
    grpcws.NewClientStreamHandler(
        // Create the gRPC stream
        func(ctx context.Context) (pb.GameService_SendCommandsClient, error) {
            return grpcClient.SendCommands(ctx)
        },
        // Factory for new request messages
        func() *pb.GameCommand { return &pb.GameCommand{} },
    ),
    nil,
))
```

### Bidirectional Streaming

For RPCs like `rpc Chat(stream Request) returns (stream Response)`:

```go
router.HandleFunc("/ws/v1/sync", gohttp.WSServe(
    grpcws.NewBidiStreamHandler(
        // Create the gRPC stream
        func(ctx context.Context) (pb.GameService_SyncGameClient, error) {
            return grpcClient.SyncGame(ctx)
        },
        // Factory for new request messages
        func() *pb.PlayerAction { return &pb.PlayerAction{} },
    ),
    nil,
))
```

## Message Protocol

All messages use a JSON envelope format for consistency and easy debugging.

### Server → Client Messages

```json
// Data message with protojson payload
{"type": "data", "data": {"field": "value", ...}}

// Error message
{"type": "error", "error": "error message"}

// Stream completed
{"type": "stream_end"}

// Heartbeat ping
{"type": "ping", "pingId": 123}
```

### Client → Server Messages

```json
// Data message with protojson payload
{"type": "data", "data": {"field": "value", ...}}

// Heartbeat response
{"type": "pong", "pingId": 123}

// Request stream cancellation
{"type": "cancel"}

// Half-close (client done sending, for client/bidi streaming)
{"type": "end_send"}
```

## Architecture

### Stream Interfaces

The package defines generic interfaces that match gRPC streaming patterns:

```go
// ServerStream - for server streaming RPCs
type ServerStream[Resp proto.Message] interface {
    Recv() (Resp, error)
    grpc.ClientStream
}

// ClientStream - for client streaming RPCs
type ClientStream[Req, Resp proto.Message] interface {
    Send(Req) error
    CloseAndRecv() (Resp, error)
    grpc.ClientStream
}

// BidiStream - for bidirectional streaming RPCs
type BidiStream[Req, Resp proto.Message] interface {
    Send(Req) error
    Recv() (Resp, error)
    CloseSend() error
    grpc.ClientStream
}
```

### Connection Types

Each streaming pattern has a corresponding connection type:

| Type | Pattern | Client Can | Server Can |
|------|---------|------------|------------|
| `ServerStreamConn` | Server Streaming | Control (cancel, pong) | Send data, ping |
| `ClientStreamConn` | Client Streaming | Send data, end_send | Respond once |
| `BidiStreamConn` | Bidirectional | Send data, end_send | Send data |

### Handler Types

Handlers validate HTTP requests and create connections:

```go
type ServerStreamHandler[Req, Resp proto.Message, Stream ServerStream[Resp]] struct {
    CreateStream func(ctx context.Context, req Req) (Stream, error)
    ParseRequest func(r *http.Request) (Req, error)
}

type ClientStreamHandler[Req, Resp proto.Message, Stream ClientStream[Req, Resp]] struct {
    CreateStream func(ctx context.Context) (Stream, error)
    NewRequest   func() Req
}

type BidiStreamHandler[Req, Resp proto.Message, Stream BidiStream[Req, Resp]] struct {
    CreateStream func(ctx context.Context) (Stream, error)
    NewRequest   func() Req
}
```

## Configuration

### Custom Protojson Options

Configure protojson marshaling/unmarshaling:

```go
handler := &grpcws.ServerStreamHandler[*pb.Req, *pb.Resp, pb.MyService_StreamClient]{
    CreateStream: createStreamFunc,
    ParseRequest: parseRequestFunc,
    MarshalOptions: protojson.MarshalOptions{
        EmitUnpopulated: true,
        UseProtoNames:   true,
    },
    UnmarshalOptions: protojson.UnmarshalOptions{
        DiscardUnknown: true,
    },
}
```

### WebSocket Configuration

Configure the underlying WebSocket via `WSConnConfig`:

```go
config := &gohttp.WSConnConfig{
    BiDirStreamConfig: &gohttp.BiDirStreamConfig{
        PingPeriod: 25 * time.Second,
        PongPeriod: 60 * time.Second,
    },
}

router.HandleFunc("/ws/v1/stream", gohttp.WSServe(handler, config))
```

## Metrics

Each connection tracks metrics via `StreamMetrics`:

```go
type StreamMetrics struct {
    ConnectedAt  time.Time
    MsgsSent     int64
    MsgsReceived int64
}
```

Access metrics in custom connection implementations or via logging.

## Error Handling

### Stream Errors

gRPC stream errors are automatically forwarded to the WebSocket client:

```json
{"type": "error", "error": "rpc error: code = NotFound desc = game not found"}
```

### Cancellation

Clients can cancel streams by sending:

```json
{"type": "cancel"}
```

This triggers context cancellation on the gRPC stream.

### Half-Close

For client and bidi streaming, clients signal they're done sending:

```json
{"type": "end_send"}
```

## Example: Complete Setup

```go
package main

import (
    "context"
    "log"
    "net/http"

    "github.com/gorilla/mux"
    "github.com/panyam/servicekit/grpcws"
    gohttp "github.com/panyam/servicekit/http"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    pb "your/proto/package"
)

func main() {
    // Connect to gRPC server
    conn, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := pb.NewGameServiceClient(conn)
    router := mux.NewRouter()

    // WebSocket endpoints
    ws := router.PathPrefix("/ws").Subrouter()

    // Server streaming
    ws.HandleFunc("/v1/games/{game_id}/subscribe", gohttp.WSServe(
        grpcws.NewServerStreamHandler(
            func(ctx context.Context, req *pb.SubscribeRequest) (pb.GameService_SubscribeClient, error) {
                return client.Subscribe(ctx, req)
            },
            func(r *http.Request) (*pb.SubscribeRequest, error) {
                return &pb.SubscribeRequest{GameId: mux.Vars(r)["game_id"]}, nil
            },
        ),
        nil,
    ))

    // Client streaming
    ws.HandleFunc("/v1/games/{game_id}/commands", gohttp.WSServe(
        grpcws.NewClientStreamHandler(
            func(ctx context.Context) (pb.GameService_SendCommandsClient, error) {
                return client.SendCommands(ctx)
            },
            func() *pb.GameCommand { return &pb.GameCommand{} },
        ),
        nil,
    ))

    // Bidirectional streaming
    ws.HandleFunc("/v1/games/{game_id}/sync", gohttp.WSServe(
        grpcws.NewBidiStreamHandler(
            func(ctx context.Context) (pb.GameService_SyncGameClient, error) {
                return client.SyncGame(ctx)
            },
            func() *pb.PlayerAction { return &pb.PlayerAction{} },
        ),
        nil,
    ))

    log.Println("Starting server on :8080")
    log.Fatal(http.ListenAndServe(":8080", router))
}
```

## Demo

See `cmd/grpcws-demo/` for a complete working example with:

- Real proto files (`pb/game.proto`)
- Buf configuration for code generation
- Mock gRPC streams for testing
- Browser-based test UI

Run the demo:

```bash
go run ./cmd/grpcws-demo
# Open http://localhost:8080
```

## Comparison with grpc-gateway

| Feature | grpc-gateway (SSE) | grpcws |
|---------|-------------------|--------|
| Server Streaming | Yes | Yes |
| Client Streaming | No | Yes |
| Bidirectional | No | Yes |
| Heartbeat | No | Yes (ping-pong) |
| Timeout Detection | Limited | Full support |
| Cancellation | Limited | Full support |
| Protocol | SSE/REST | WebSocket |

Use grpc-gateway for simple REST + server streaming. Use grpcws when you need client streaming, bidirectional streaming, or robust connection management.
