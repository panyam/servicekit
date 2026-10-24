package grpcws

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// expectNormalClose reads past pings until the server's close frame and
// checks that it is a normal closure.
func expectNormalClose(t *testing.T, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		_, _, err := conn.ReadMessage()
		if err == nil {
			continue
		}
		var ce *websocket.CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("expected a close frame, got %v", err)
		}
		if ce.Code != websocket.CloseNormalClosure {
			t.Fatalf("got close code %d, want %d", ce.Code, websocket.CloseNormalClosure)
		}
		return
	}
}

func expectStillOpen(t *testing.T, conn *websocket.Conn, wait time.Duration) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(wait))
	_, _, err := conn.ReadMessage()
	var nerr net.Error
	if !errors.As(err, &nerr) || !nerr.Timeout() {
		t.Fatalf("expected the socket to stay open (read timeout), got %v", err)
	}
}

func serveBidi(t *testing.T, opts ConnOptions, recvCh chan *timestamppb.Timestamp) *websocket.Conn {
	t.Helper()
	handler := NewBidiStreamHandler(
		func(ctx context.Context) (*testBidiStream, error) {
			return &testBidiStream{ctx: ctx, sendCh: make(chan *timestamppb.Timestamp, 10), recvCh: recvCh}, nil
		},
		func() *timestamppb.Timestamp { return &timestamppb.Timestamp{} },
	)
	handler.ConnOptions = opts
	router := mux.NewRouter()
	router.HandleFunc("/ws/sync", gohttp.WSServe(handler, nil))
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	conn := dialWS(t, server.URL, "/ws/sync")
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestBidiStream_CloseOnStreamEnd_ClosesAfterStreamEnd(t *testing.T) {
	recvCh := make(chan *timestamppb.Timestamp, 1)
	conn := serveBidi(t, ConnOptions{CloseOnStreamEnd: true}, recvCh)

	recvCh <- &timestamppb.Timestamp{Seconds: 42}
	close(recvCh)

	if got := extractTimestampSeconds(t, recvControlOfType(t, conn, TypeData, 2*time.Second)); got != 42 {
		t.Fatalf("got seconds=%d, want 42", got)
	}
	recvControlOfType(t, conn, TypeStreamEnd, 2*time.Second)
	expectNormalClose(t, conn, 3*time.Second)
}

func TestBidiStream_StreamEndKeepsSocketOpenByDefault(t *testing.T) {
	recvCh := make(chan *timestamppb.Timestamp)
	conn := serveBidi(t, ConnOptions{}, recvCh)

	close(recvCh)
	recvControlOfType(t, conn, TypeStreamEnd, 2*time.Second)
	expectStillOpen(t, conn, 500*time.Millisecond)
}

func TestServerStream_CloseOnStreamEnd_ClosesAfterStreamEnd(t *testing.T) {
	ch := make(chan *timestamppb.Timestamp)
	handler := NewServerStreamHandler(
		func(ctx context.Context, req *timestamppb.Timestamp) (*testServerStream, error) {
			return &testServerStream{ch: ch, ctx: ctx}, nil
		},
		func(r *http.Request) (*timestamppb.Timestamp, error) { return &timestamppb.Timestamp{}, nil },
	)
	handler.CloseOnStreamEnd = true
	router := mux.NewRouter()
	router.HandleFunc("/ws/subscribe", gohttp.WSServe(handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()
	conn := dialWS(t, server.URL, "/ws/subscribe")
	defer conn.Close()

	close(ch)
	recvControlOfType(t, conn, TypeStreamEnd, 2*time.Second)
	expectNormalClose(t, conn, 3*time.Second)
}

func TestClientStream_CloseOnStreamEnd_ClosesAfterResponse(t *testing.T) {
	handler := NewClientStreamHandler(
		func(ctx context.Context) (*testClientStream, error) {
			return &testClientStream{
				ctx:      ctx,
				received: make(chan *timestamppb.Timestamp, 10),
				response: &timestamppb.Timestamp{Seconds: 9999},
			}, nil
		},
		func() *timestamppb.Timestamp { return &timestamppb.Timestamp{} },
	)
	handler.CloseOnStreamEnd = true
	router := mux.NewRouter()
	router.HandleFunc("/ws/commands", gohttp.WSServe(handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()
	conn := dialWS(t, server.URL, "/ws/commands")
	defer conn.Close()

	sendControl(t, conn, ControlMessage{Type: TypeEndSend})
	if got := extractTimestampSeconds(t, recvControlOfType(t, conn, TypeData, 2*time.Second)); got != 9999 {
		t.Fatalf("got seconds=%d, want 9999", got)
	}
	recvControlOfType(t, conn, TypeStreamEnd, 2*time.Second)
	expectNormalClose(t, conn, 3*time.Second)
}

// floodBidiStream returns 256KB responses as fast as the server will take
// them, standing in for a busy room feeding a client that has stopped reading.
// Small frames would sit in loopback's socket buffers without ever blocking.
type floodBidiStream struct {
	ctx context.Context
}

var floodPayload = wrapperspb.String(strings.Repeat("x", 256<<10))

func (s *floodBidiStream) Send(*timestamppb.Timestamp) error { return nil }
func (s *floodBidiStream) Recv() (*wrapperspb.StringValue, error) {
	if s.ctx.Err() != nil {
		return nil, io.EOF
	}
	return floodPayload, nil
}
func (s *floodBidiStream) CloseSend() error             { return nil }
func (s *floodBidiStream) Header() (metadata.MD, error) { return nil, nil }
func (s *floodBidiStream) Trailer() metadata.MD         { return nil }
func (s *floodBidiStream) Context() context.Context     { return s.ctx }
func (s *floodBidiStream) SendMsg(m any) error          { return nil }
func (s *floodBidiStream) RecvMsg(m any) error          { return nil }

func TestBidiStream_WriteTimeoutDropsStuckClient(t *testing.T) {
	ctxDone := make(chan struct{})
	handler := NewBidiStreamHandler(
		func(ctx context.Context) (*floodBidiStream, error) {
			go func() {
				<-ctx.Done()
				close(ctxDone)
			}()
			return &floodBidiStream{ctx: ctx}, nil
		},
		func() *timestamppb.Timestamp { return &timestamppb.Timestamp{} },
	)
	handler.WriteTimeout = 200 * time.Millisecond
	router := mux.NewRouter()
	router.HandleFunc("/ws/sync", gohttp.WSServe(handler, nil))
	server := httptest.NewServer(router)
	defer server.Close()
	conn := dialWS(t, server.URL, "/ws/sync")
	defer conn.Close()

	select {
	case <-ctxDone:
	case <-time.After(10 * time.Second):
		t.Fatal("server kept a client that stopped reading")
	}
}
