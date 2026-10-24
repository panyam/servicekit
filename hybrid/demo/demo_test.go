package demo

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"

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
	for name, got := range map[string]map[string]any{"gateway": p.Gateway, "connect": p.Connect, "grpcws": p.GRPCWS} {
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
	for name, got := range map[string]map[string]any{"connect": p.Connect, "grpcws": p.GRPCWS} {
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
	if _, err := getJSON(base + GatewayPath + "nope"); err == nil {
		t.Error("gateway: expected an error for a missing game")
	}
	if _, err := postJSON(base+ConnectPath, `{"gameId":"nope"}`); err == nil {
		t.Error("connect: expected an error for a missing game")
	}
}
