package connectbridge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// fakeSender captures messages passed to Send so the bridge's forwarding
// paths can be verified without constructing a real *connect.ServerStream
// (which has no exported constructor).
type fakeSender[T any] struct {
	sent []*T
	err  error
}

func (f *fakeSender[T]) Send(msg *T) error {
	f.sent = append(f.sent, msg)
	return f.err
}

func newBridgeWithFake[T any](ctx context.Context, f *fakeSender[T]) *ConnectStreamBridge[T] {
	return &ConnectStreamBridge[T]{ctx: ctx, stream: f}
}

// Compile-time assertion: ConnectStreamBridge[T] satisfies the gRPC
// server-streaming server interface generated proto code expects.
var _ grpc.ServerStreamingServer[int] = (*ConnectStreamBridge[int])(nil)

func TestSend_ForwardsToUnderlyingStream(t *testing.T) {
	fake := &fakeSender[string]{}
	bridge := newBridgeWithFake(context.Background(), fake)

	msg := "hello"
	if err := bridge.Send(&msg); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if len(fake.sent) != 1 || fake.sent[0] != &msg {
		t.Fatalf("Send did not forward message: got %v", fake.sent)
	}
}

func TestSend_PropagatesUnderlyingError(t *testing.T) {
	sentinel := errors.New("downstream failure")
	fake := &fakeSender[string]{err: sentinel}
	bridge := newBridgeWithFake(context.Background(), fake)

	msg := "x"
	if err := bridge.Send(&msg); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestContext_ReturnsConstructorContext(t *testing.T) {
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	bridge := newBridgeWithFake(ctx, &fakeSender[string]{})

	if got := bridge.Context().Value(ctxKey{}); got != "marker" {
		t.Fatalf("Context did not return the constructor context: got %v", got)
	}
}

func TestSendMsg_TypedPointerForwards(t *testing.T) {
	fake := &fakeSender[string]{}
	bridge := newBridgeWithFake(context.Background(), fake)

	msg := "via SendMsg"
	if err := bridge.SendMsg(&msg); err != nil {
		t.Fatalf("SendMsg returned error: %v", err)
	}
	if len(fake.sent) != 1 || fake.sent[0] != &msg {
		t.Fatalf("SendMsg did not forward message: got %v", fake.sent)
	}
}

func TestSendMsg_WrongTypeRejected(t *testing.T) {
	fake := &fakeSender[string]{}
	bridge := newBridgeWithFake(context.Background(), fake)

	// *int does not satisfy *string — must be rejected without touching the stream.
	wrong := 42
	err := bridge.SendMsg(&wrong)
	if err == nil {
		t.Fatal("SendMsg with wrong-type message should have errored")
	}
	if !strings.Contains(err.Error(), "invalid message type") {
		t.Fatalf("expected 'invalid message type' error, got %v", err)
	}
	if len(fake.sent) != 0 {
		t.Fatalf("SendMsg with wrong type should not forward, got %v", fake.sent)
	}
}

func TestSendMsg_NilRejected(t *testing.T) {
	fake := &fakeSender[string]{}
	bridge := newBridgeWithFake(context.Background(), fake)

	if err := bridge.SendMsg(nil); err == nil {
		t.Fatal("SendMsg(nil) should have errored")
	}
	if len(fake.sent) != 0 {
		t.Fatalf("SendMsg(nil) should not forward, got %v", fake.sent)
	}
}

func TestRecvMsg_NotSupported(t *testing.T) {
	bridge := newBridgeWithFake(context.Background(), &fakeSender[string]{})

	err := bridge.RecvMsg(new(string))
	if err == nil {
		t.Fatal("RecvMsg should return an error for server-streaming bridge")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected 'not supported' error, got %v", err)
	}
}

func TestMetadataMethods_AreNoOps(t *testing.T) {
	bridge := newBridgeWithFake(context.Background(), &fakeSender[string]{})
	md := metadata.New(map[string]string{"x-trace": "1"})

	if err := bridge.SetHeader(md); err != nil {
		t.Errorf("SetHeader returned error: %v", err)
	}
	if err := bridge.SendHeader(md); err != nil {
		t.Errorf("SendHeader returned error: %v", err)
	}
	bridge.SetTrailer(md) // must not panic
}

func TestNewConnectStreamBridge_AcceptsAnySender(t *testing.T) {
	fake := &fakeSender[string]{}
	bridge := NewConnectStreamBridge[string](context.Background(), fake)
	msg := "hi"
	if err := bridge.Send(&msg); err != nil {
		t.Fatal(err)
	}
	if len(fake.sent) != 1 || *fake.sent[0] != "hi" {
		t.Fatalf("got %v", fake.sent)
	}
}
