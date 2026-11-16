package grpcws

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	gohttp "github.com/panyam/servicekit/http"
	"github.com/panyam/servicekit/internal/testcert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// A grpc-go client reaches a gRPC server mounted as an http.Handler behind
// ListenAndServeGraceful with WithTLS. gRPC needs HTTP/2, so this fails unless
// the TLS listener offers h2 through ALPN.
func TestGracefulTLS_GRPCClient(t *testing.T) {
	c := testcert.New(t)
	gs := grpc.NewServer()
	healthpb.RegisterHealthServer(gs, health.NewServer())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- gohttp.ListenAndServeGraceful(&http.Server{Handler: gs},
			gohttp.WithListener(ln), gohttp.WithTLS(c.CertFile, c.KeyFile), gohttp.WithContext(ctx))
	}()

	creds := credentials.NewTLS(&tls.Config{RootCAs: c.Pool})
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer conn.Close()

	callCtx, callCancel := context.WithTimeout(ctx, 5*time.Second)
	defer callCancel()
	resp, err := healthpb.NewHealthClient(conn).Check(callCtx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Health.Check over TLS: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("status = %v, want SERVING", resp.Status)
	}

	conn.Close()
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("shutdown returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for graceful shutdown")
	}
}
