# servicekit/hybrid

Serve one protobuf service over **grpcws** (WebSocket streams for browsers), **grpc-gateway** (REST-ish JSON from `google.api.http` annotations) and **Connect**, from one `http.Handler`, with the same JSON on all three.

This is its own Go module, so grpc-gateway and Connect only reach servers that import it:

```
go get github.com/panyam/servicekit/hybrid
```

## Why the JSON needs sharing

Each transport ships its own protojson defaults. For `Game{game_id: "g1", display_name: "Opening night"}` (score and players unset), `make exercise` with `-shared=false` prints:

```
gateway {"displayName":"Opening night","gameId":"g1","players":[],"score":0}
connect {"displayName":"Opening night","gameId":"g1"}
grpcws  {"displayName":"Opening night","gameId":"g1"}
DIFFERENT
```

The gateway emits zero values and the other two omit them. With one `hybrid.ProtoJSON` handed to all three (here `EmitUnpopulated` plus `UseProtoNames`):

```
gateway {"display_name":"Opening night","game_id":"g1","players":[],"score":0}
connect {"display_name":"Opening night","game_id":"g1","players":[],"score":0}
grpcws  {"display_name":"Opening night","game_id":"g1","players":[],"score":0}
identical
```

## Usage

```go
pj := hybrid.ProtoJSON{Marshal: protojson.MarshalOptions{EmitUnpopulated: true}}

gw := runtime.NewServeMux(pj.GatewayOption())
gen.RegisterGameServiceHandlerServer(ctx, gw, impl)

watch := grpcws.NewServerStreamHandler[*gen.GetGameRequest, *gen.Game, grpc.ServerStreamingClient[gen.Game]](create, parse)
watch.MarshalOptions, watch.UnmarshalOptions = pj.Marshal, pj.Unmarshal

m := hybrid.NewMux()
m.Gateway("/v1/", gw)
cs := connect.NewServer()                                 // connectrpc.com/connect/v2
genconnect.RegisterGameServiceHandler(cs, connectImpl)
m.Connect(cs, pj.ConnectOption())
m.Handle("GET /ws/v1/games/{game_id}/watch", gohttp.WSServe(watch, nil))
http.ListenAndServe(":8080", m.Handler(middleware.NewRequestID().Middleware))
```

`demo/server.go` is the complete version. It writes the service once against the gRPC interfaces, and reaches it in-process from the gateway, through `connectbridge` from Connect, and over an in-memory gRPC connection from grpcws.

## Make targets

| Target | What it does |
|---|---|
| `make test` | unit tests plus the demo's interop tests |
| `make exercise` | runs `cmd/demo -check`: fetch one game three ways, print, exit 1 if they differ |
| `make gen` | regenerate `demo/gen` with buf (remote plugins, Connect pinned to v2.0.2) |
