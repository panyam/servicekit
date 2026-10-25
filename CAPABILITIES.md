# ServiceKit

## Version
0.2.0

## Provides
- websocket-infrastructure: Production-grade WebSocket connection management
- generic-connection: Generic BaseConn[I, O] for type-safe message handling
- codec-system: Built-in codecs (JSON, TypedJSON, ProtoJSON, BinaryProto)
- grpc-over-websocket: gRPC streaming over WebSocket via grpcws package
- hybrid-transports: servicekit/hybrid submodule serves one proto service over grpcws, grpc-gateway JSON and Connect with one shared protojson config (ProtoJSON) and one router (Mux)
- connect-to-grpc-bridge: hybrid/connectbridge.ConnectStreamBridge[T] adapts any Connect server stream (v1 *connect.ServerStream[T] or a v2 generated stream; anything with Send(*T) error) to grpc.ServerStreamingServer[T]
- http-middleware: Guard, RateLimiter, ConnLimiter, BodyLimiter, OriginChecker, CORS, Recovery, RequestLogger, RequestID, HealthCheck
- server-timeouts: ApplyDefaults helper for sensible http.Server timeout defaults
- sse-connection: SSEConn[O] for server-sent event connections with keepalive and codec support
- sse-hub: SSEHub[O] generic session manager for SSE connections with register/broadcast/close
- graceful-shutdown: ListenAndServeGraceful with signal handling, drain timeout, and OnShutdown callbacks
- streamable-http: StreamableServe for POST-that-optionally-streams pattern (MCP 2025-03-26 Streamable HTTP)
- jsonrpc-conn: RPCConn — bidirectional JSON-RPC 2.0 over Content-Length framed streams (LSP / MCP-stdio); Call/Notify with id correlation + a peer-request handler, so stdio JSON-RPC clients need not re-implement it
- connection-lifecycle: OnStart, HandleMessage, OnClose hooks with heartbeat/ping-pong
- server-initiated-close: BaseConn.CloseAfterSend and grpcws ConnOptions.CloseOnStreamEnd end a socket from the server, with a hard-close grace period for stuck clients
- slow-client-eviction: per-write WriteTimeout on BaseConn and grpcws handlers drops clients that stop reading
- connection-observer: http.ConnObserver is told when WebSocket, SSE and grpcws connections open and close (with a CloseReason such as write_timeout or server_close) and about every frame; no metrics dependency, so an app can implement it over OpenTelemetry
- prometheus-metrics: servicekit/prom.ConnMetrics (a ConnObserver) and HTTPMetrics middleware export connection, message, close-reason and RED request metrics, labelled by mux route template
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
require github.com/panyam/servicekit 0.1.2

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
