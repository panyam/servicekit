package demo

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	gohttp "github.com/panyam/servicekit/http"

	"github.com/panyam/servicekit/hybrid"
	"github.com/panyam/servicekit/hybrid/demo/gen"
	"google.golang.org/protobuf/encoding/protojson"
)

// A game with zero-valued fields, where marshalers that disagree on
// EmitUnpopulated show it.
func testGames() *Games {
	return &Games{Games: map[string]*gen.Game{
		"g1": {GameId: "g1", DisplayName: "Opening night"},
	}}
}

func serve(t *testing.T, cfg Config) string {
	t.Helper()
	s, err := New(context.Background(), testGames(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv := httptest.NewServer(s.Handler)
	t.Cleanup(func() {
		srv.Close()
		s.Close()
	})
	return srv.URL
}

func TestTransportsReturnSameJSONWithSharedProtoJSON(t *testing.T) {
	pj := &hybrid.ProtoJSON{Marshal: protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}}
	p, err := Fetch(serve(t, Config{ProtoJSON: pj}), "g1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"game_id": "g1", "display_name": "Opening night", "score": float64(0), "players": []any{}}
	for name, got := range map[string]map[string]any{"gateway": p.Gateway, "connect": p.Connect, "grpc-web": p.GRPCWeb, "grpcws": p.GRPCWS} {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
}

// Wired up by hand, each transport keeps its own protojson default: the
// gateway emits zero values and the others don't. This is the mismatch
// ProtoJSON exists to remove.
func TestTransportsDisagreeOnDefaults(t *testing.T) {
	p, err := Fetch(serve(t, Config{}), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Gateway["score"]; !ok {
		t.Errorf("gateway default should emit score=0, got %v", p.Gateway)
	}
	for name, got := range map[string]map[string]any{"connect": p.Connect, "grpc-web": p.GRPCWeb, "grpcws": p.GRPCWS} {
		if _, ok := got["score"]; ok {
			t.Errorf("%s default should omit score=0, got %v", name, got)
		}
	}
	if reflect.DeepEqual(p.Gateway, p.Connect) {
		t.Error("expected the gateway and Connect defaults to differ")
	}
}

func TestGetGameNotFoundOverGatewayAndConnect(t *testing.T) {
	base := serve(t, Config{ProtoJSON: &hybrid.ProtoJSON{}})
	f := fetcher{http: http.DefaultClient}
	if _, err := f.getJSON(base + GatewayPath + "nope"); err == nil {
		t.Error("gateway: expected an error for a missing game")
	}
	if _, err := f.postJSON(base+ConnectPath, `{"gameId":"nope"}`); err == nil {
		t.Error("connect: expected an error for a missing game")
	}
	if _, _, err := f.postGRPCWeb(base+ConnectPath, `{"gameId":"nope"}`); err == nil || !strings.Contains(err.Error(), "grpc-status") {
		t.Errorf("grpc-web: expected a non-zero grpc-status for a missing game, got %v", err)
	}
}

// agni#1627 serves Connect, gRPC-Web and the rest over TLS from one handler.
// This runs all four transports through gohttp.WithTLS, including a
// gRPC-Web client over TLS, which servicekit#60's tests don't cover.
func TestTransportsReturnSameJSONOverTLS(t *testing.T) {
	s, err := New(context.Background(), testGames(), Config{ProtoJSON: &hybrid.ProtoJSON{Marshal: protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	certFile, keyFile, roots, err := SelfSigned(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- gohttp.ListenAndServeGraceful(&http.Server{Handler: s.Handler},
			gohttp.WithListener(lis), gohttp.WithTLS(certFile, keyFile),
			gohttp.WithContext(ctx), gohttp.WithDrainTimeout(time.Second))
	}()
	defer func() {
		cancel()
		<-done
	}()

	p, err := FetchTLS("https://"+lis.Addr().String(), "g1", roots)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"game_id": "g1", "display_name": "Opening night", "score": float64(0), "players": []any{}}
	for name, got := range map[string]map[string]any{"gateway": p.Gateway, "connect": p.Connect, "grpc-web": p.GRPCWeb, "grpcws": p.GRPCWS} {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}

	if p.GRPCWebProto != "HTTP/2.0" {
		t.Errorf("gRPC-Web over TLS used %s, want HTTP/2.0", p.GRPCWebProto)
	}

	if _, err := Fetch("http://"+lis.Addr().String(), "g1"); err == nil {
		t.Error("plain HTTP to the TLS port should fail")
	}
}
