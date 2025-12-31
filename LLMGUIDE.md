# ServiceKit LLM Guide

> Quick reference for LLMs to understand and use this library effectively.

## What This Is (3 sentences)

ServiceKit provides production-grade WebSocket infrastructure for Go applications. It handles connection lifecycle, ping/pong heartbeats, and concurrent writes so you don't have to. The `grpcws` package adds gRPC streaming over WebSocket for browser clients.

## When to Use

| Need | Use |
|------|-----|
| Real-time WebSocket server | `http.BaseConn` + `http.WSServe` |
| gRPC streaming from browsers | `grpcws` package |
| TypeScript WebSocket client | `@panyam/servicekit-client` |

## Quick Start (Read First)

For context, read in this order:
1. This file (LLMGUIDE.md)
2. `SUMMARY.md` - Architecture overview
3. `cmd/grpcws-demo/main.go` - Working example

## Templates

### Template 1: Basic WebSocket Endpoint

```go
package main

import (
    "log"
    "net/http"

    "github.com/gorilla/mux"
    gohttp "github.com/panyam/servicekit/http"
)

// Step 1: Define your connection (embed JSONConn)
type MyConn struct {
    gohttp.JSONConn  // MUST embed this for JSON messages
    // Add your fields here
}

// Step 2: Implement HandleMessage (REQUIRED)
func (c *MyConn) HandleMessage(msg any) error {
    log.Printf("Received: %v", msg)

    // Send response through Writer (NEVER write directly to WebSocket)
    c.SendOutput(map[string]any{"status": "ok"})
    return nil
}

// Step 3: Define handler for validation/auth
type MyHandler struct{}

func (h *MyHandler) Validate(w http.ResponseWriter, r *http.Request) (*MyConn, bool) {
    // Return nil, false to reject connection
    return &MyConn{
        JSONConn: gohttp.JSONConn{
            Codec:   &gohttp.JSONCodec{},
            NameStr: "MyConn",
        },
    }, true
}

// Step 4: Wire it up
func main() {
    router := mux.NewRouter()
    router.HandleFunc("/ws", gohttp.WSServe(&MyHandler{}, nil))
    log.Fatal(http.ListenAndServe(":8080", router))
}
```

### Template 2: gRPC Server Streaming (Subscribe Pattern)

```go
import (
    "context"
    "net/http"

    "github.com/gorilla/mux"
    "github.com/panyam/servicekit/grpcws"
    gohttp "github.com/panyam/servicekit/http"
    "your/proto/gen"  // Your protobuf types
)

// Implement grpcws.ServerStream[YourResponseType]
type MyServerStream struct {
    ctx    context.Context
    cancel context.CancelFunc
    events chan *gen.YourEvent
}

func (s *MyServerStream) Recv() (*gen.YourEvent, error) {
    select {
    case <-s.ctx.Done():
        return nil, io.EOF
    case event := <-s.events:
        return event, nil
    }
}

// Required interface methods
func (s *MyServerStream) Header() (metadata.MD, error) { return nil, nil }
func (s *MyServerStream) Trailer() metadata.MD         { return nil }
func (s *MyServerStream) CloseSend() error             { return nil }
func (s *MyServerStream) Context() context.Context     { return s.ctx }
func (s *MyServerStream) SendMsg(m any) error          { return nil }
func (s *MyServerStream) RecvMsg(m any) error          { return nil }

// Wire it up
router.HandleFunc("/ws/subscribe", gohttp.WSServe(
    grpcws.NewServerStreamHandler(
        // CreateStream: creates your stream from request
        func(ctx context.Context, req *gen.SubscribeRequest) (*MyServerStream, error) {
            ctx, cancel := context.WithCancel(ctx)
            return &MyServerStream{ctx: ctx, cancel: cancel, events: make(chan *gen.YourEvent, 10)}, nil
        },
        // ParseRequest: extracts request from HTTP
        func(r *http.Request) (*gen.SubscribeRequest, error) {
            return &gen.SubscribeRequest{Id: mux.Vars(r)["id"]}, nil
        },
    ),
    nil,
))
```

### Template 3: gRPC Bidirectional Streaming

```go
// Implement grpcws.BidiStream[InputType, OutputType]
type MyBidiStream struct {
    ctx    context.Context
    cancel context.CancelFunc
    input  chan *gen.InputMsg
    output chan *gen.OutputMsg
}

func (s *MyBidiStream) Send(msg *gen.InputMsg) error {
    // Process incoming message, maybe trigger output
    s.output <- &gen.OutputMsg{Result: "processed"}
    return nil
}

func (s *MyBidiStream) Recv() (*gen.OutputMsg, error) {
    select {
    case <-s.ctx.Done():
        return nil, io.EOF
    case msg := <-s.output:
        return msg, nil
    }
}

func (s *MyBidiStream) CloseSend() error { return nil }
// ... other interface methods same as ServerStream

// Wire it up
router.HandleFunc("/ws/sync", gohttp.WSServe(
    grpcws.NewBidiStreamHandler(
        func(ctx context.Context) (*MyBidiStream, error) {
            ctx, cancel := context.WithCancel(ctx)
            return &MyBidiStream{ctx: ctx, cancel: cancel, output: make(chan *gen.OutputMsg, 10)}, nil
        },
        func() *gen.InputMsg { return &gen.InputMsg{} },  // Message factory
    ),
    nil,
))
```

### Template 4: TypeScript Client

```typescript
import { GRPCWSClient } from '@panyam/servicekit-client';

const client = new GRPCWSClient();

// Set up handlers BEFORE connecting
client.onMessage = (data) => console.log('Received:', data);
client.onError = (error) => console.error('Error:', error);
client.onClose = () => console.log('Disconnected');
client.onPing = (pingId) => console.log('Ping:', pingId);  // Auto-responds with pong

// Connect
await client.connect('ws://localhost:8080/ws/endpoint');

// Send messages
client.send({ type: 'action', data: { foo: 'bar' } });

// For client streaming: signal end of input
client.endSend();

// Clean up
client.close();
```

## Key Rules (Do's and Don'ts)

### MUST Do

```go
// 1. ALWAYS embed JSONConn (or BaseConn) for connections
type MyConn struct {
    gohttp.JSONConn  // Required
}

// 2. ALWAYS call parent OnStart if you override it
func (c *MyConn) OnStart(conn *websocket.Conn) error {
    if err := c.JSONConn.OnStart(conn); err != nil {  // Call parent FIRST
        return err
    }
    // Your init code here
    return nil
}

// 3. ALWAYS send through Writer, never directly
c.SendOutput(msg)  // Correct
// or
c.Writer.Send(gohttp.OutgoingMessage[any]{Data: &msg})  // Also correct
```

### NEVER Do

```go
// 1. NEVER write directly to WebSocket (causes concurrent write panic)
conn.WriteMessage(...)  // WRONG - will panic under load

// 2. NEVER skip parent OnClose
func (c *MyConn) OnClose() {
    // Your cleanup
    c.JSONConn.OnClose()  // MUST call parent
}

// 3. NEVER block in HandleMessage (use goroutines for slow operations)
func (c *MyConn) HandleMessage(msg any) error {
    go c.slowOperation(msg)  // Correct - don't block
    return nil
}
```

## Architecture Quick Reference

```
┌─────────────────────────────────────────────────────────────┐
│                    Transport Layer                          │
│  • Ping/Pong always JSON (handled automatically)            │
│  • All writes through OutgoingMessage union type            │
│  • Thread-safe via Writer                                   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      Codec Layer                            │
│  • Data encoding only (not control messages)                │
│  • JSONCodec (default), TypedJSONCodec, ProtoJSONCodec      │
│  • BinaryProtoCodec for efficiency                          │
└─────────────────────────────────────────────────────────────┘
```

## File Reference

| When you need... | Read this file |
|------------------|----------------|
| Architecture overview | `SUMMARY.md` |
| Design decisions | `ARCHITECTURE.md` |
| Working multiplayer example | `cmd/grpcws-demo/main.go` |
| gRPC-WS details | `grpcws/README.md` |
| TypeScript client | `clients/typescript/README.md` |
| All codec types | `http/codec.go` |
| BaseConn implementation | `http/baseconn.go` |
| Stream handlers | `grpcws/server_stream.go`, `grpcws/client_stream.go`, `grpcws/bidi_stream.go` |

## Message Protocol (gRPC-WS)

Server sends:
```json
{"type": "data", "data": {...}}
{"type": "error", "error": "message"}
{"type": "stream_end"}
{"type": "ping", "pingId": 123}
```

Client sends:
```json
{"type": "data", "data": {...}}
{"type": "pong", "pingId": 123}
{"type": "end_send"}
{"type": "cancel"}
```

## Common Patterns

### Room/Hub Pattern (Multiplayer)
See `cmd/grpcws-demo/main.go` for complete implementation:
- `GameHub` manages rooms by ID
- `GameRoom` tracks players, broadcasts events
- `PlayerConn` has channels for events/state

### Lifecycle Hooks Order
```
Validate() → OnStart() → [HandleMessage()...] → OnClose()
                              ↓
                         OnError()
                              ↓
                         OnTimeout()
```

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| "concurrent write to websocket" | Direct WebSocket write | Use `c.SendOutput()` |
| Messages not received | Forgot `OnStart()` parent call | Call `c.JSONConn.OnStart(conn)` |
| Client not responding to ping | Wrong client library | Use `GRPCWSClient` for grpcws endpoints |
| Binary messages not working | Codec mismatch | Match client codec to server codec |
