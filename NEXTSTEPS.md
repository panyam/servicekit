# Next Steps

## Immediate Tasks

- [x] Update code comments and godoc for all new files
- [x] Create TypeScript client library (`@panyam/servicekit-client`)
- [x] Update grpcws-demo to use the new client library
- [x] Fix concurrent write panic (OutgoingMessage union type)
- [x] Separate transport layer from codec layer (pings always JSON)
- [x] Add client-side codec support (JSONCodec, BinaryCodec)
- [x] Create multiplayer demo with gameId-based rooms
- [x] Manual browser testing of grpcws-demo multiplayer
- [x] Create LLMGUIDE.md for LLM discoverability
- [x] Publish TypeScript client to npm (@panyam/servicekit-client)
- [x] Add TypeScript SSE client: SSEClient, StreamableClient, vendored parser (issue #10)

## Future Enhancements

### Testing
- [x] Add `GRPCWSClient.createMock()` test utility (issue #1)
- [x] Add reusable `MockWebSocket` + `MockWSController` in `mock.ts`
- [x] Add integration tests for grpcws over real WebSocket connections (issue #9)
- [ ] Add load tests for concurrent WebSocket connections
- [ ] Add benchmarks comparing codec implementations

### Features
- [x] Add connection pooling/limiting middleware (`middleware/` package)
- [x] Add SSE connection type: SSEConn[O], SSEHub[O] (issue #6)
- [x] Add graceful shutdown helper: ListenAndServeGraceful (issue #7)
- [x] Add Streamable HTTP handler: StreamableServe (issue #8)
- [ ] Add metrics export (Prometheus)
- [ ] Add OpenTelemetry tracing support
- [ ] Consider grpc-gateway integration for hybrid deployments

### Middleware
- [x] Add BodyLimiter middleware (issue #2)
- [x] Add HealthCheck handler (issue #3)
- [x] Add RequestID middleware (issue #4)
- [x] Add ServerTimeouts helper (issue #5)
- [x] Add Makefile and pre-push git hook for test automation
- [x] Enhance RequestLogger to include request ID from context
- [ ] Add middleware integration tests with real WebSocket connections
- [ ] Add middleware benchmarks

### Documentation
- [ ] Add migration guide for existing JSONConn users
- [ ] Add examples for TypedJSONCodec usage
- [ ] Add examples for ProtoJSONCodec with custom options

### Code Quality
- [ ] Consider build tags for optional proto dependency
- [ ] Add more godoc examples
- [ ] Review error handling consistency
