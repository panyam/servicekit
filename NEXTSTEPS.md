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
- [ ] Publish TypeScript client to npm

## Future Enhancements

### Testing
- [ ] Add integration tests for grpcws with real gRPC server
- [ ] Add load tests for concurrent WebSocket connections
- [ ] Add benchmarks comparing codec implementations

### Features
- [ ] Add connection pooling/limiting middleware
- [ ] Add metrics export (Prometheus)
- [ ] Add OpenTelemetry tracing support
- [ ] Consider grpc-gateway integration for hybrid deployments

### Documentation
- [ ] Add migration guide for existing JSONConn users
- [ ] Add examples for TypedJSONCodec usage
- [ ] Add examples for ProtoJSONCodec with custom options

### Code Quality
- [ ] Consider build tags for optional proto dependency
- [ ] Add more godoc examples
- [ ] Review error handling consistency
