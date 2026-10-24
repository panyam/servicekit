// Package connectbridge adapts Connect server streams to the gRPC streaming
// server interfaces that protobuf-generated code expects.
//
// Use it when a Connect handler needs to delegate a server-streaming RPC to a
// gRPC-shape service implementation. The adapter wraps anything with
// Send(*T) error, which covers both Connect v1's *connect.ServerStream[T] and
// v2's generated stream structs, so it satisfies grpc.ServerStreamingServer[T].
//
// Metadata methods (SetHeader, SendHeader, SetTrailer) are no-ops in v1;
// header and trailer propagation is a future concern.
package connectbridge
