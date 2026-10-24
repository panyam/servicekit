// Package demo is the hybrid example: one GameService served as gateway JSON,
// Connect and a grpcws stream from one handler. hybrid/cmd/demo runs it, and
// its tests check that the three transports return the same JSON.
package demo

import (
	"context"
	"net"
	"net/http"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/panyam/servicekit/grpcws"
	gohttp "github.com/panyam/servicekit/http"
	"github.com/panyam/servicekit/hybrid"
	"github.com/panyam/servicekit/hybrid/connectbridge"
	"github.com/panyam/servicekit/hybrid/demo/gen"
	"github.com/panyam/servicekit/hybrid/demo/gen/genconnect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Games is the service implementation, written once against the gRPC
// interfaces. The gateway calls it in-process, Connect reaches it through
// connectbridge, and grpcws through an in-memory gRPC connection.
type Games struct {
	gen.UnimplementedGameServiceServer
	Games map[string]*gen.Game
}

// GetGame returns the game, or NotFound.
func (g *Games) GetGame(ctx context.Context, req *gen.GetGameRequest) (*gen.Game, error) {
	game, ok := g.Games[req.GetGameId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no game %q", req.GetGameId())
	}
	return game, nil
}

// WatchGame sends the game's current state once and ends the stream.
func (g *Games) WatchGame(req *gen.GetGameRequest, stream grpc.ServerStreamingServer[gen.Game]) error {
	game, err := g.GetGame(stream.Context(), req)
	if err != nil {
		return err
	}
	return stream.Send(game)
}

// connectGames is the Connect (v2) face of Games. GetGame already has the
// signature Connect v2 wants; WatchGame needs connectbridge to turn Connect's
// generated stream into the grpc.ServerStreamingServer the shared code takes.
type connectGames struct{ *Games }

func (c connectGames) WatchGame(ctx context.Context, req *gen.GetGameRequest, stream genconnect.GameServiceWatchGameServerStream) error {
	return c.Games.WatchGame(req, connectbridge.NewConnectStreamBridge[gen.Game](ctx, stream))
}

var _ genconnect.GameServiceHandler = connectGames{}

// Config picks the JSON options. A nil ProtoJSON leaves each transport on its
// own default, which is what a server wired up by hand gets, and the JSON then
// differs between them.
type Config struct {
	ProtoJSON *hybrid.ProtoJSON
}

// Server is a running demo: Handler serves all three transports, and Close
// stops the in-memory gRPC server behind the grpcws route.
type Server struct {
	Handler http.Handler
	Close   func()
}

// Paths the demo serves, relative to the server's base URL.
const (
	GatewayPath = "/v1/games/"
	ConnectPath = "/servicekit.hybrid.demo.v1.GameService/GetGame"
	WatchPath   = "/ws/v1/games/"
)

// New wires games into a hybrid.Mux.
func New(ctx context.Context, games *Games, cfg Config) (*Server, error) {
	var gwOpts []runtime.ServeMuxOption
	var connectOpts []connecthttp.Option
	var pj hybrid.ProtoJSON
	if cfg.ProtoJSON != nil {
		pj = *cfg.ProtoJSON
		gwOpts = append(gwOpts, pj.GatewayOption())
		connectOpts = append(connectOpts, pj.ConnectOption())
	}

	gw := runtime.NewServeMux(gwOpts...)
	if err := gen.RegisterGameServiceHandlerServer(ctx, gw, games); err != nil {
		return nil, err
	}

	lis := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	gen.RegisterGameServiceServer(grpcServer, games)
	go grpcServer.Serve(lis)
	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		grpcServer.Stop()
		return nil, err
	}
	client := gen.NewGameServiceClient(cc)

	watch := grpcws.NewServerStreamHandler[*gen.GetGameRequest, *gen.Game, grpc.ServerStreamingClient[gen.Game]](
		func(ctx context.Context, req *gen.GetGameRequest) (grpc.ServerStreamingClient[gen.Game], error) {
			return client.WatchGame(ctx, req)
		},
		func(r *http.Request) (*gen.GetGameRequest, error) {
			return &gen.GetGameRequest{GameId: r.PathValue("game_id")}, nil
		},
	)
	watch.MarshalOptions, watch.UnmarshalOptions = pj.Marshal, pj.Unmarshal
	watch.CloseOnStreamEnd = true

	m := hybrid.NewMux()
	m.Gateway("/v1/", gw)
	cs := connect.NewServer()
	genconnect.RegisterGameServiceHandler(cs, connectGames{games})
	m.Connect(cs, connectOpts...)
	m.Handle("GET "+WatchPath+"{game_id}/watch", gohttp.WSServe(watch, nil))

	return &Server{
		Handler: m.Handler(),
		Close: func() {
			cc.Close()
			grpcServer.Stop()
		},
	}, nil
}
