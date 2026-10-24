// Package hybrid serves one protobuf service over grpcws, grpc-gateway JSON and
// Connect from a single http.Handler, with the same JSON on every transport.
//
// It is its own Go module (github.com/panyam/servicekit/hybrid) so that
// grpc-gateway and Connect never enter the root servicekit module's dependency
// graph. Only servers that import this package pay for them.
//
// The part that is easy to get wrong by hand is JSON shape. Each transport
// brings its own protojson defaults, which disagree on whether zero-valued
// fields are emitted:
//
//   - grpc-gateway emits them (EmitUnpopulated: true).
//   - Connect's JSON codec and grpcws omit them.
//
// So the same Game{score: 0} comes back as {"score":0, ...} from the gateway and
// without "score" from the other two. ProtoJSON holds one set of options and
// hands each transport its own form of it.
//
//	pj := hybrid.ProtoJSON{Marshal: protojson.MarshalOptions{EmitUnpopulated: true}}
//
//	gw := runtime.NewServeMux(pj.GatewayOption())
//	gen.RegisterGameServiceHandlerServer(ctx, gw, impl)
//
//	watch := grpcws.NewServerStreamHandler(createStream, parseRequest)
//	watch.MarshalOptions, watch.UnmarshalOptions = pj.Marshal, pj.Unmarshal
//
//	m := hybrid.NewMux()
//	m.Gateway("/v1/", gw)
//	m.Connect(genconnect.NewGameServiceHandler(connectImpl, pj.ConnectOption()))
//	m.Handle("/ws/v1/games/{game_id}/watch", gohttp.WSServe(watch, nil))
//	http.ListenAndServe(addr, m.Handler(middleware.NewRequestID().Middleware))
//
// hybrid/cmd/demo is a runnable version of the above, and `make -C hybrid
// exercise` checks that all three transports return the same JSON.
package hybrid

import (
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ProtoJSON is the protojson configuration shared by every transport a hybrid
// server exposes. The zero value uses protojson's defaults (camelCase names,
// zero values omitted, unknown fields rejected) everywhere, which is already
// consistent, but differs from grpc-gateway's own default.
type ProtoJSON struct {
	Marshal   protojson.MarshalOptions
	Unmarshal protojson.UnmarshalOptions
}

// GatewayOption installs these options as the grpc-gateway marshaler for every
// content type, replacing the gateway's default (which emits unpopulated
// fields and discards unknown ones). google.api.HttpBody responses still pass
// through as raw bytes, as with the gateway's default.
func (p ProtoJSON) GatewayOption() runtime.ServeMuxOption {
	return runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
		Marshaler: &runtime.JSONPb{MarshalOptions: p.Marshal, UnmarshalOptions: p.Unmarshal},
	})
}

// ConnectOption replaces Connect's "json" codec with one using these options.
// Pass it to the generated New<Service>Handler (or to a Connect client). The
// binary "proto" codec is untouched.
func (p ProtoJSON) ConnectOption() connect.Option {
	return connect.WithCodec(connectJSON{p})
}

type connectJSON struct{ p ProtoJSON }

func (connectJSON) Name() string { return "json" }

func (c connectJSON) Marshal(v any) ([]byte, error) {
	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("hybrid: %T is not a proto.Message", v)
	}
	return c.p.Marshal.Marshal(msg)
}

func (c connectJSON) Unmarshal(data []byte, v any) error {
	msg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("hybrid: %T is not a proto.Message", v)
	}
	return c.p.Unmarshal.Unmarshal(data, msg)
}

// Mux routes grpcws endpoints, a grpc-gateway ServeMux and Connect handlers
// on one http.ServeMux, so one middleware chain covers all of them.
//
// The point of it over a bare http.ServeMux is the gateway: a runtime.ServeMux
// matches on its own templates and answers 404 for anything else, so mounting
// it at "/" would swallow the Connect and grpcws routes. Mux mounts it under a
// prefix instead. Patterns use net/http's Go 1.22 syntax, and registering the
// same pattern twice panics, as with http.ServeMux.
type Mux struct {
	mux *http.ServeMux
}

// NewMux returns an empty Mux.
func NewMux() *Mux {
	return &Mux{mux: http.NewServeMux()}
}

// Handle registers any handler, typically gohttp.WSServe for a grpcws stream.
func (m *Mux) Handle(pattern string, h http.Handler) {
	m.mux.Handle(pattern, h)
}

// Gateway mounts a grpc-gateway ServeMux under prefix ("/v1/"). The gateway
// sees the full request path, since its templates (from google.api.http
// annotations) include the prefix. prefix must end in "/".
func (m *Mux) Gateway(prefix string, gw *runtime.ServeMux) {
	if !strings.HasSuffix(prefix, "/") {
		panic(fmt.Sprintf("hybrid: gateway prefix %q must end in /", prefix))
	}
	m.mux.Handle(prefix, gw)
}

// Connect mounts a Connect handler. Its arguments match what a generated
// New<Service>Handler returns: the service path ("/pkg.Service/") and the
// handler.
func (m *Mux) Connect(path string, h http.Handler) {
	m.mux.Handle(path, h)
}

// Handler returns the routes wrapped in mw, with the first middleware
// outermost. Middleware that wraps the ResponseWriter must forward Hijack for
// the grpcws routes to upgrade; servicekit's middleware package does.
func (m *Mux) Handler(mw ...func(http.Handler) http.Handler) http.Handler {
	var h http.Handler = m.mux
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}
