// Command demo serves the hybrid GameService over gateway JSON, Connect,
// gRPC-Web and grpcws on one port.
//
//	go run ./cmd/demo                # serve on :8080
//	go run ./cmd/demo -check         # serve on a free port, fetch g1 four ways, compare
//	go run ./cmd/demo -check -tls    # the same over TLS, with a throwaway self-signed cert
//	go run ./cmd/demo -check -shared=false   # with each transport's own defaults
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"time"

	gohttp "github.com/panyam/servicekit/http"
	"github.com/panyam/servicekit/hybrid"
	"github.com/panyam/servicekit/hybrid/demo"
	"github.com/panyam/servicekit/hybrid/demo/gen"
	"github.com/panyam/servicekit/middleware"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	check := flag.Bool("check", false, "fetch game g1 over every transport, print the JSON, exit non-zero if it differs")
	shared := flag.Bool("shared", true, "use one hybrid.ProtoJSON for every transport")
	useTLS := flag.Bool("tls", false, "with -check, serve HTTPS through gohttp.WithTLS and a throwaway self-signed cert")
	flag.Parse()

	cfg := demo.Config{}
	if *shared {
		cfg.ProtoJSON = &hybrid.ProtoJSON{Marshal: protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}}
	}
	games := &demo.Games{Games: map[string]*gen.Game{"g1": {GameId: "g1", DisplayName: "Opening night"}}}
	s, err := demo.New(context.Background(), games, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	handler := middleware.NewRequestID().Middleware(s.Handler)

	if !*check {
		log.Printf("serving on %s: GET %sg1, POST %s, ws %sg1/watch", *addr, demo.GatewayPath, demo.ConnectPath, demo.WatchPath)
		log.Fatal(http.ListenAndServe(*addr, handler))
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	var p demo.Payloads
	if *useTLS {
		dir, err := os.MkdirTemp("", "hybrid-demo-tls")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)
		certFile, keyFile, roots, err := demo.SelfSigned(dir)
		if err != nil {
			log.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- gohttp.ListenAndServeGraceful(&http.Server{Handler: handler},
				gohttp.WithListener(lis), gohttp.WithTLS(certFile, keyFile),
				gohttp.WithContext(ctx), gohttp.WithDrainTimeout(time.Second))
		}()
		p, err = demo.FetchTLS("https://"+lis.Addr().String(), "g1", roots)
		cancel()
		<-done
		if err != nil {
			log.Fatal(err)
		}
	} else {
		go http.Serve(lis, handler)
		if p, err = demo.Fetch("http://"+lis.Addr().String(), "g1"); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("grpc-web over %s\n", p.GRPCWebProto)
	for _, row := range []struct {
		name string
		v    map[string]any
	}{{"gateway ", p.Gateway}, {"connect ", p.Connect}, {"grpc-web", p.GRPCWeb}, {"grpcws  ", p.GRPCWS}} {
		b, _ := json.Marshal(row.v)
		fmt.Printf("%s %s\n", row.name, b)
	}
	if reflect.DeepEqual(p.Gateway, p.Connect) && reflect.DeepEqual(p.Gateway, p.GRPCWeb) && reflect.DeepEqual(p.Gateway, p.GRPCWS) {
		fmt.Println("identical")
		return
	}
	fmt.Println("DIFFERENT")
	os.Exit(1)
}
