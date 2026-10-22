package http

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// serveGraceful starts ListenAndServeGraceful on a pre-bound loopback listener
// and returns its address and the channel its result arrives on.
func serveGraceful(t *testing.T, srv *http.Server, opts ...GracefulOption) (string, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(srv, append(opts, WithListener(ln))...)
	}()
	return ln.Addr().String(), errCh
}

func waitResult(t *testing.T, errCh <-chan error, within time.Duration) (error, time.Duration) {
	t.Helper()
	start := time.Now()
	select {
	case err := <-errCh:
		return err, time.Since(start)
	case <-time.After(within):
		t.Fatalf("ListenAndServeGraceful did not return within %v", within)
		return nil, 0
	}
}

func TestGracefulShutdown_CancelsRequestContexts(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(cancelled)
	})}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, errCh := serveGraceful(t, srv, WithContext(ctx), WithDrainTimeout(5*time.Second))

	go http.Get("http://" + addr)
	<-entered
	cancel()

	err, elapsed := waitResult(t, errCh, 10*time.Second)
	if err != nil {
		t.Errorf("expected nil error once the stream handler returns, got %v", err)
	}
	if elapsed > time.Second {
		t.Errorf("shutdown took %v; the handler's context should have been cancelled well inside the 5s drain", elapsed)
	}
	select {
	case <-cancelled:
	default:
		t.Error("handler's request context was never cancelled")
	}
}

func TestGracefulShutdown_CancelRequestsKeepsCallerBaseContext(t *testing.T) {
	type key struct{}
	gotValue := make(chan any, 1)
	entered := make(chan struct{})
	srv := &http.Server{
		BaseContext: func(net.Listener) context.Context {
			return context.WithValue(context.Background(), key{}, "caller")
		},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotValue <- r.Context().Value(key{})
			close(entered)
			<-r.Context().Done()
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, errCh := serveGraceful(t, srv, WithContext(ctx), WithDrainTimeout(5*time.Second))

	go http.Get("http://" + addr)
	<-entered
	if v := <-gotValue; v != "caller" {
		t.Errorf("request context lost the caller's BaseContext value: got %v", v)
	}
	cancel()

	if _, elapsed := waitResult(t, errCh, 10*time.Second); elapsed > time.Second {
		t.Errorf("shutdown took %v; a caller-set BaseContext should still be cancelled on shutdown", elapsed)
	}
}

func TestGracefulShutdown_CancelRequestsDisabled(t *testing.T) {
	entered := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	})}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, errCh := serveGraceful(t, srv,
		WithContext(ctx),
		WithDrainTimeout(300*time.Millisecond),
		WithCancelRequests(false),
	)

	go http.Get("http://" + addr)
	<-entered
	cancel()

	err, elapsed := waitResult(t, errCh, 5*time.Second)
	if elapsed < 250*time.Millisecond {
		t.Errorf("shutdown returned after %v; with cancellation off the handler should hold the drain", elapsed)
	}
	if !errors.Is(err, ErrDrainTimeout) {
		t.Errorf("expected ErrDrainTimeout, got %v", err)
	}
}

func TestGracefulShutdown_DrainTimeoutClosesConns(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	entered := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(entered)
		<-release // ignores its context on purpose
	})}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, errCh := serveGraceful(t, srv, WithContext(ctx), WithDrainTimeout(200*time.Millisecond))

	resp, err := http.Get("http://" + addr)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	<-entered

	bodyDone := make(chan error, 1)
	go func() {
		_, err := resp.Body.Read(make([]byte, 1))
		bodyDone <- err
	}()

	cancel()
	err, _ = waitResult(t, errCh, 5*time.Second)
	if !errors.Is(err, ErrDrainTimeout) {
		t.Errorf("expected ErrDrainTimeout, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ErrDrainTimeout should still match context.DeadlineExceeded, got %v", err)
	}

	select {
	case <-bodyDone:
	case <-time.After(time.Second):
		t.Error("client connection still open after the drain timed out; expected srv.Close to drop it")
	}
}

const secondSignalChildEnv = "SERVICEKIT_GRACEFUL_SECOND_SIGNAL_CHILD"

// The child process is needed because once a test calls signal.Notify for
// SIGINT, Go never restores the default kill action in that process.
func TestGracefulShutdown_SecondSignalKills(t *testing.T) {
	if os.Getenv(secondSignalChildEnv) == "1" {
		runSecondSignalChild()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestGracefulShutdown_SecondSignalKills$")
	cmd.Env = append(os.Environ(), secondSignalChildEnv+"=1")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer cmd.Process.Kill()

	out := bufio.NewScanner(stdout)
	var addr string
	for out.Scan() {
		if a, ok := strings.CutPrefix(out.Text(), "ADDR "); ok {
			addr = a
			break
		}
	}
	if addr == "" {
		t.Fatal("child never reported its address")
	}
	go http.Get("http://" + addr)
	for out.Scan() && out.Text() != "IN_HANDLER" {
	}

	cmd.Process.Signal(syscall.SIGINT)
	errLog := bufio.NewScanner(stderr)
	for errLog.Scan() && !strings.Contains(errLog.Text(), "shutting down") {
	}
	go func() {
		for errLog.Scan() {
		}
	}()
	cmd.Process.Signal(syscall.SIGINT)

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err := <-waitErr:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected the child to die from SIGINT, got %v", err)
		}
		ws := exitErr.Sys().(syscall.WaitStatus)
		if !ws.Signaled() || ws.Signal() != syscall.SIGINT {
			t.Errorf("expected death by SIGINT, got %v", ws)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second SIGINT was swallowed; child still draining")
	}
}

func runSecondSignalChild() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("IN_HANDLER")
		select {} // holds the drain open regardless of context cancellation
	})}
	fmt.Println("ADDR " + ln.Addr().String())
	ListenAndServeGraceful(srv,
		WithListener(ln),
		WithSignals(syscall.SIGINT),
		WithDrainTimeout(30*time.Second),
	)
	os.Exit(0)
}
