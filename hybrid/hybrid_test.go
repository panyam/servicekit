package hybrid

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

func TestConnectJSONRejectsNonProto(t *testing.T) {
	c := connectJSON{}
	if c.Name() != "json" {
		t.Fatalf("codec name %q, want json (Connect looks it up by name)", c.Name())
	}
	if err := c.MarshalWrite(context.Background(), io.Discard, struct{}{}); err == nil {
		t.Error("MarshalWrite of a non-proto value should fail")
	}
	if err := c.UnmarshalRead(context.Background(), strings.NewReader(`{}`), &struct{}{}); err == nil {
		t.Error("UnmarshalRead into a non-proto value should fail")
	}
}

func TestMuxGatewayPrefixMustEndInSlash(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for a prefix without a trailing slash")
		}
	}()
	NewMux().Gateway("/v1", runtime.NewServeMux())
}

func TestMuxGatewayDoesNotSwallowOtherRoutes(t *testing.T) {
	m := NewMux()
	m.Gateway("/v1/", runtime.NewServeMux())
	m.Handle("/pkg.Service/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("connect")) }))
	m.Handle("GET /ws/watch", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ws")) }))
	h := m.Handler()
	for path, want := range map[string]string{"/pkg.Service/Get": "connect", "/ws/watch": "ws"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Body.String() != want {
			t.Errorf("%s: got %q (status %d), want %q", path, rec.Body.String(), rec.Code, want)
		}
	}
}

func TestMuxHandlerAppliesMiddlewareOutermostFirst(t *testing.T) {
	var order []string
	mw := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	m := NewMux()
	m.Handle("/", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "handler") }))
	m.Handler(mw("outer"), mw("inner")).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if got := strings.Join(order, ","); got != "outer,inner,handler" {
		t.Fatalf("order %s", got)
	}
}
