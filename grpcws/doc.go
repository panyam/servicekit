// Package grpcws provides WebSocket transport for gRPC streaming RPCs.
//
// This package bridges gRPC streaming with WebSocket connections, providing:
//   - Full lifecycle management (ping/pong, timeouts, graceful shutdown)
//   - Support for all streaming modes (server, client, bidirectional)
//   - Type-safe message handling via generics
//   - Coexistence with grpc-gateway (separate route prefixes)
//
// # Why Use gRPC-over-WebSocket?
//
// While grpc-gateway provides SSE (Server-Sent Events) for streaming, WebSocket offers:
//   - Bidirectional communication (SSE is server-to-client only)
//   - Heartbeat/ping-pong for dead connection detection
//   - Connection lifecycle hooks for proper resource management
//   - Better proxy/load balancer compatibility
//
// # Streaming Modes
//
// Server Streaming (rpc Foo(Req) returns (stream Resp)):
//
//	handler := grpcws.NewServerStreamHandler(
//	    func(ctx context.Context, req *pb.SubscribeRequest) (pb.Service_SubscribeClient, error) {
//	        return client.Subscribe(ctx, req)
//	    },
//	    parseRequest,
//	)
//	router.HandleFunc("/ws/v1/subscribe", gohttp.WSServe(handler, nil))
//
// Client Streaming (rpc Foo(stream Req) returns (Resp)):
//
//	handler := grpcws.NewClientStreamHandler(
//	    func(ctx context.Context) (pb.Service_SendClient, error) {
//	        return client.Send(ctx)
//	    },
//	    parseMessage,
//	)
//
// Bidirectional Streaming (rpc Foo(stream Req) returns (stream Resp)):
//
//	handler := grpcws.NewBidiStreamHandler(
//	    func(ctx context.Context) (pb.Service_SyncClient, error) {
//	        return client.Sync(ctx)
//	    },
//	    parseMessage,
//	)
//
// # Message Protocol
//
// All messages use a JSON envelope for control:
//
//	// Server → Client
//	{"type": "data", "data": <payload>}      // Stream message
//	{"type": "error", "error": "msg"}         // Error
//	{"type": "stream_end"}                    // Stream completed
//	{"type": "ping", "pingId": 1}            // Heartbeat
//
//	// Client → Server
//	{"type": "data", "data": <payload>}      // Send message (client/bidi streaming)
//	{"type": "pong", "pingId": 1}            // Heartbeat response
//	{"type": "cancel"}                        // Cancel stream
//	{"type": "end_send"}                      // Half-close (client done sending)
//
// # Route Registration
//
// Use a separate route prefix to coexist with grpc-gateway:
//
//	// WebSocket on /ws/v1/...
//	wsRouter := router.PathPrefix("/ws").Subrouter()
//	wsRouter.HandleFunc("/v1/games/{id}/subscribe", gohttp.WSServe(handler, nil))
//
//	// grpc-gateway on /v1/... (REST/SSE)
//	router.PathPrefix("/v1/").Handler(gwMux)
package grpcws
