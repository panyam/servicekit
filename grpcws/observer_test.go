package grpcws

import (
	"sync"
	"testing"
	"time"

	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type streamObserver struct {
	mu     sync.Mutex
	opened []gohttp.ConnInfo
	closed []gohttp.CloseReason
	done   chan struct{}
}

func (o *streamObserver) ConnOpened(info gohttp.ConnInfo) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opened = append(o.opened, info)
}

func (o *streamObserver) ConnClosed(info gohttp.ConnInfo, r gohttp.CloseReason, _ time.Duration) {
	o.mu.Lock()
	o.closed = append(o.closed, r)
	o.mu.Unlock()
	close(o.done)
}

func (o *streamObserver) MessageSent(gohttp.ConnInfo, string) {}
func (o *streamObserver) MessageReceived(gohttp.ConnInfo)     {}

// A grpcws stream reports as transport grpcws with its mode, and a
// CloseOnStreamEnd close counts as a server close.
func TestObserver_GRPCWSStreamEndIsServerClose(t *testing.T) {
	obs := &streamObserver{done: make(chan struct{})}
	recvCh := make(chan *timestamppb.Timestamp)
	conn := serveBidi(t, ConnOptions{CloseOnStreamEnd: true, Observer: obs}, recvCh)

	close(recvCh)
	expectNormalClose(t, conn, 3*time.Second)
	select {
	case <-obs.done:
	case <-time.After(3 * time.Second):
		t.Fatal("ConnClosed was not called")
	}
	obs.mu.Lock()
	defer obs.mu.Unlock()
	want := gohttp.ConnInfo{Transport: "grpcws", Mode: "bidi"}
	if len(obs.opened) != 1 || obs.opened[0] != want {
		t.Fatalf("opened = %+v, want [%+v]", obs.opened, want)
	}
	if len(obs.closed) != 1 || obs.closed[0] != gohttp.CloseServer {
		t.Fatalf("closed = %v, want [server_close]", obs.closed)
	}
}
