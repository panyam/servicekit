# ServiceKit

## Version
0.0.12

## Provides
- websocket-infrastructure: Production-grade WebSocket connection management
- generic-connection: Generic BaseConn[I, O] for type-safe message handling
- codec-system: Built-in codecs (JSON, TypedJSON, ProtoJSON, BinaryProto)
- grpc-over-websocket: gRPC streaming over WebSocket via grpcws package
- http-middleware: Guard, RateLimiter, ConnLimiter, BodyLimiter, OriginChecker, CORS, Recovery, RequestLogger, RequestID, HealthCheck
- server-timeouts: ApplyDefaults helper for sensible http.Server timeout defaults
- sse-connection: SSEConn[O] for server-sent event connections with keepalive and codec support
- sse-hub: SSEHub[O] generic session manager for SSE connections with register/broadcast/close
- graceful-shutdown: ListenAndServeGraceful with signal handling, drain timeout, and OnShutdown callbacks
- streamable-http: StreamableServe for POST-that-optionally-streams pattern (MCP 2025-03-26 Streamable HTTP)
- connection-lifecycle: OnStart, HandleMessage, OnClose hooks with heartbeat/ping-pong
- typescript-client: @panyam/servicekit-client npm package for browser WebSocket
- trusted-proxy: IP extraction with trusted proxy support

## Module
github.com/panyam/servicekit

## Location
newstack/servicekit/master

## Stack Dependencies
- gocurrent (github.com/panyam/gocurrent)
- goutils (github.com/panyam/goutils)

## Integration

### Go Module
```go
// go.mod
require github.com/panyam/servicekit 0.0.12

// Local development
replace github.com/panyam/servicekit => ~/newstack/servicekit/master
```

### Key Imports
```go
import gohttp "github.com/panyam/servicekit/http"
```

## Status
Mature

## Conventions
- Generic types for message safety
- Transport/codec separation
- Nil-safe middleware
- OutgoingMessage union type for thread-safe writes
