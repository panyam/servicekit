package connectbridge

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/grpc/metadata"
)

// connectSender is the minimal subset of *connect.ServerStream[T] the bridge
// depends on. Keeping it unexported lets tests inject fakes while the public
// constructor continues to accept the concrete connect type.
type connectSender[T any] interface {
	Send(*T) error
}

// ConnectStreamBridge adapts a *connect.ServerStream[T] to satisfy
// grpc.ServerStreamingServer[T]. It lets a Connect handler delegate a
// server-streaming RPC to a gRPC-shape service implementation.
//
// SetHeader, SendHeader, and SetTrailer are no-ops; header and trailer
// propagation is deferred until a consumer needs it.
type ConnectStreamBridge[T any] struct {
	ctx    context.Context
	stream connectSender[T]
}

// NewConnectStreamBridge wraps a Connect server stream so it can be passed
// to gRPC-generated server-streaming code.
func NewConnectStreamBridge[T any](ctx context.Context, s *connect.ServerStream[T]) *ConnectStreamBridge[T] {
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
