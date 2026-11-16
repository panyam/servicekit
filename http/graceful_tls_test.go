package http

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/panyam/servicekit/internal/testcert"
)

// protoHandler answers with the protocol the request arrived on, and 500 if it
// did not arrive over TLS.
var protoHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		http.Error(w, "not TLS", http.StatusInternalServerError)
		return
	}
	io.WriteString(w, r.Proto)
})

func h2Client(c testcert.Cert) *http.Client {
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: c.Pool},
		ForceAttemptHTTP2: true,
	}, Timeout: 5 * time.Second}
}

func h1Client(c testcert.Cert) *http.Client {
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: c.Pool, NextProtos: []string{"http/1.1"}},
	}, Timeout: 5 * time.Second}
}

// serveTLS runs ListenAndServeGraceful on a pre-bound listener with opts and
// returns the https base URL and a stop func that shuts down and returns the
// helper's error.
func serveTLS(t *testing.T, srv *http.Server, opts ...GracefulOption) (string, func() error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	opts = append(opts, WithListener(ln), WithContext(ctx))
	go func() { errCh <- ListenAndServeGraceful(srv, opts...) }()
	stop := func() error {
		cancel()
		select {
		case err := <-errCh:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for graceful shutdown")
			return nil
		}
	}
	t.Cleanup(func() { cancel() })
	return "https://" + ln.Addr().String(), stop
}

func getBody(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Get %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %q", resp.StatusCode, body)
	}
	return string(body)
}

func TestGracefulTLS_FilesNegotiateHTTP2AndHTTP1(t *testing.T) {
	c := testcert.New(t)
	url, stop := serveTLS(t, &http.Server{Handler: protoHandler}, WithTLS(c.CertFile, c.KeyFile))

	if got := getBody(t, h2Client(c), url); got != "HTTP/2.0" {
		t.Errorf("h2 client got %q, want HTTP/2.0", got)
	}
	if got := getBody(t, h1Client(c), url); got != "HTTP/1.1" {
		t.Errorf("http/1.1-only client got %q, want HTTP/1.1", got)
	}
	if err := stop(); err != nil {
		t.Errorf("shutdown returned %v, want nil", err)
	}
}

func TestGracefulTLS_ConfigNegotiatesHTTP2(t *testing.T) {
	c := testcert.New(t)
	cfg := &tls.Config{Certificates: []tls.Certificate{c.Pair}}
	url, stop := serveTLS(t, &http.Server{Handler: protoHandler}, WithTLSConfig(cfg))

	if got := getBody(t, h2Client(c), url); got != "HTTP/2.0" {
		t.Errorf("h2 client got %q, want HTTP/2.0", got)
	}
	if err := stop(); err != nil {
		t.Errorf("shutdown returned %v, want nil", err)
	}
	if cfg.NextProtos != nil {
		t.Errorf("caller's tls.Config was modified: NextProtos = %v", cfg.NextProtos)
	}
}

// A server set up for cleartext HTTP/2 as well still offers h2 over TLS.
func TestGracefulTLS_WithUnencryptedHTTP2Protocols(t *testing.T) {
	c := testcert.New(t)
	var p http.Protocols
	p.SetHTTP1(true)
	p.SetHTTP2(true)
	p.SetUnencryptedHTTP2(true)
	url, stop := serveTLS(t, &http.Server{Handler: protoHandler, Protocols: &p}, WithTLS(c.CertFile, c.KeyFile))

	if got := getBody(t, h2Client(c), url); got != "HTTP/2.0" {
		t.Errorf("h2 client got %q, want HTTP/2.0", got)
	}
	if err := stop(); err != nil {
		t.Errorf("shutdown returned %v, want nil", err)
	}
}

// Without WithListener the helper binds srv.Addr itself and serves TLS there.
func TestGracefulTLS_BindsAddr(t *testing.T) {
	c := testcert.New(t)
	addr := getFreePorts(t, 1)[0]
	srv := &http.Server{Addr: addr, Handler: protoHandler}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- ListenAndServeGraceful(srv, WithTLS(c.CertFile, c.KeyFile), WithContext(ctx)) }()
	waitForServer(t, addr, 2*time.Second)

	if got := getBody(t, h2Client(c), "https://"+addr); got != "HTTP/2.0" {
		t.Errorf("h2 client got %q, want HTTP/2.0", got)
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Errorf("shutdown returned %v, want nil", err)
	}
}

// An HTTP/2 request in flight when shutdown starts finishes before the helper
// returns, and the helper reports a clean shutdown.
func TestGracefulTLS_DrainsInFlightHTTP2(t *testing.T) {
	c := testcert.New(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		io.WriteString(w, r.Proto)
	})}
	url, stop := serveTLS(t, srv, WithTLS(c.CertFile, c.KeyFile), WithCancelRequests(false))

	type result struct {
		body string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := h2Client(c).Get(url)
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		resCh <- result{string(b), err}
	}()
	<-entered

	stopErr := make(chan error, 1)
	go func() { stopErr <- stop() }()
	select {
	case err := <-stopErr:
		t.Fatalf("shutdown returned %v before the in-flight request finished", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)

	res := <-resCh
	if res.err != nil || res.body != "HTTP/2.0" {
		t.Errorf("in-flight request got (%q, %v), want (HTTP/2.0, nil)", res.body, res.err)
	}
	if err := <-stopErr; err != nil {
		t.Errorf("shutdown returned %v, want nil", err)
	}
}

func TestGracefulTLS_BadCertFileFailsStartup(t *testing.T) {
	srv := &http.Server{Handler: protoHandler}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	err = ListenAndServeGraceful(srv, WithListener(ln), WithTLS("/nonexistent/cert.pem", "/nonexistent/key.pem"))
	if err == nil {
		t.Fatal("expected a startup error for a missing certificate file")
	}
}

func TestGracefulTLS_BothOptionsRejected(t *testing.T) {
	c := testcert.New(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	srv := &http.Server{Handler: protoHandler}
	cfg := &tls.Config{Certificates: []tls.Certificate{c.Pair}}

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(srv, WithListener(ln), WithTLS(c.CertFile, c.KeyFile), WithTLSConfig(cfg))
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrTLSOptionsConflict) {
			t.Errorf("err = %v, want ErrTLSOptionsConflict", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListenAndServeGraceful served instead of rejecting both TLS options")
	}
	if srv.TLSConfig != nil {
		t.Error("srv.TLSConfig was set although the options were rejected")
	}
}

// A listener from tls.NewListener passed with WithTLS would do the handshake
// twice. The first connection stops the server with ErrListenerAlreadyTLS.
func TestGracefulTLS_AlreadyTLSListenerRejected(t *testing.T) {
	c := testcert.New(t)
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer raw.Close()
	ln := tls.NewListener(raw, &tls.Config{Certificates: []tls.Certificate{c.Pair}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeGraceful(&http.Server{Handler: protoHandler},
			WithListener(ln), WithTLS(c.CertFile, c.KeyFile), WithContext(ctx))
	}()

	if resp, err := h2Client(c).Get("https://" + raw.Addr().String()); err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Error("request succeeded through a doubly wrapped TLS listener")
		}
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrListenerAlreadyTLS) {
			t.Errorf("err = %v, want ErrListenerAlreadyTLS", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server kept running on a listener that already does TLS")
	}
}
