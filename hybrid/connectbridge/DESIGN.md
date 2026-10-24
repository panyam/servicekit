---
package: connectbridge
purpose: Adapts a Connect server stream so a Connect handler can delegate a server-streaming RPC to gRPC-generated service code.
last_rebuilt: 04f5d50ab9801bba49c428990700c9aded8b119b
depends_on:
entities:
  - name: ConnectStreamBridge
    kind: struct
    role: Wraps a *connect.ServerStream[T] to satisfy grpc.ServerStreamingServer[T].
    why: Lets one server-streaming impl serve both Connect and gRPC transports without forking the handler; metadata methods are deliberately no-ops in v1.
  - name: connectSender
    kind: interface
    role: Unexported minimal Send-only subset of *connect.ServerStream[T] that the bridge actually depends on.
    why: Connect's ServerStream has no exported constructor, so narrowing to this seam is the only way tests can inject a fake while the public constructor still takes the concrete type.
  - name: NewConnectStreamBridge
    kind: func
    role: Constructor wrapping a concrete *connect.ServerStream[T] with a context.
    why: Keeps the public API tied to the concrete connect type while internals hold the connectSender seam.
  - name: SendMsg
    kind: func
    role: gRPC-interface entry point; type-asserts any to *T then forwards, else errors.
    why: Generated gRPC code calls SendMsg with any; the assertion guards against wrong-type/nil messages reaching the stream (returns "invalid message type").
  - name: RecvMsg
    kind: func
    role: Always returns an error.
    why: Server streaming is send-only; receiving is structurally unsupported, so it fails loudly rather than silently no-op.
  - name: SetHeader / SendHeader / SetTrailer
    kind: func
    role: Metadata methods required by the gRPC interface, implemented as no-ops.
    why: Header/trailer propagation between Connect and gRPC is deferred to a future version; present only to satisfy the interface contract.
---

The package is a thin transport-shim: it bridges the structural mismatch between Connect's `*connect.ServerStream[T]` and the `grpc.ServerStreamingServer[T]` interface that protobuf-generated server code expects. A compile-time assertion in the test (`var _ grpc.ServerStreamingServer[int] = (*ConnectStreamBridge[int])(nil)`) is the load-bearing contract.

Key gotcha: there is no real `*connect.ServerStream` constructor exported, which is the entire reason for the unexported `connectSender[T]` interface seam — it exists for testability, not for production polymorphism. Header/trailer propagation is intentionally unimplemented (no-ops), so this bridge is unsuitable as-is for flows that depend on gRPC metadata reaching the Connect client.
