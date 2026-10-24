package connectbridge

import (
	"context"
	"fmt"

	"google.golang.org/grpc/metadata"
)

// Sender is the one method the bridge needs from a Connect server stream. Both
// Connect majors have it: v1's *connect.ServerStream[T], and the per-method
// stream struct v2 generates (Game<Service><Method>ServerStream). Because the
// bridge depends only on this, it doesn't import either major.
type Sender[T any] interface {
	Send(*T) error
}

// ConnectStreamBridge adapts a Connect server stream to satisfy
// grpc.ServerStreamingServer[T]. It lets a Connect handler delegate a
// server-streaming RPC to a gRPC-shape service implementation.
//
// SetHeader, SendHeader, and SetTrailer are no-ops; header and trailer
// propagation is deferred until a consumer needs it.
type ConnectStreamBridge[T any] struct {
	ctx    context.Context
	stream Sender[T]
}

// NewConnectStreamBridge wraps a Connect server stream so it can be passed to
// gRPC-generated server-streaming code. ctx is what Context returns; pass the
// handler's context so cancellation reaches the gRPC-shape implementation.
//
// Until v0.2.0 this took a *connect.ServerStream[T] (Connect v1). That type
// still satisfies Sender[T], so v1 callers need no change beyond the import
// path, github.com/panyam/servicekit/hybrid/connectbridge.
func NewConnectStreamBridge[T any](ctx context.Context, s Sender[T]) *ConnectStreamBridge[T] {
	return &ConnectStreamBridge[T]{ctx: ctx, stream: s}
}

func (b *ConnectStreamBridge[T]) Send(msg *T) error {
	return b.stream.Send(msg)
}

func (b *ConnectStreamBridge[T]) Context() context.Context {
	return b.ctx
}

func (b *ConnectStreamBridge[T]) SendMsg(m any) error {
	if msg, ok := m.(*T); ok {
		return b.stream.Send(msg)
	}
	return fmt.Errorf("invalid message type")
}

func (b *ConnectStreamBridge[T]) RecvMsg(m any) error {
	return fmt.Errorf("RecvMsg not supported for server streaming")
}

func (b *ConnectStreamBridge[T]) SetHeader(metadata.MD) error {
	return nil
}

func (b *ConnectStreamBridge[T]) SendHeader(metadata.MD) error {
	return nil
}

func (b *ConnectStreamBridge[T]) SetTrailer(metadata.MD) {}
