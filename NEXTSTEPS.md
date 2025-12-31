# Next Steps

## Immediate Tasks

- [x] Update code comments and godoc for all new files
- [x] Create TypeScript client library (`@panyam/servicekit-client`)
- [x] Update grpcws-demo to use the new client library
- [ ] Manual browser testing of grpcws-demo
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
