// +build !nogrpcserver,!_test

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"

	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/lib-server"
)

const defaultAddr = "127.0.0.1:31007"
const defaultWebAddr = "127.0.0.1:31008"

// do not change this, js client relies on this msg to ensure that server is up
const grpcWebStartedMessagePrefix = "gRPC Web proxy started at: "

func main() {
	var addr string
	var webaddr string

	if len(os.Args) > 1 {
		addr = os.Args[1]
		if len(os.Args) > 2 {
			webaddr = os.Args[2]
		}
	}

	if addr == "" {
		if env := os.Getenv("ANYTYPE_GRPC_ADDR"); env != "" {
			addr = env
		} else {
			addr = defaultAddr
		}
	}

	if webaddr == "" {
		if env := os.Getenv("ANYTYPE_GRPCWEB_ADDR"); env != "" {
			webaddr = env
		} else {
			webaddr = defaultWebAddr
		}
	}

	var stopChan = make(chan os.Signal, 2)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	var mw = &core.Middleware{}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	lib.RegisterClientCommandsServer(server, mw)

	webrpc := grpcweb.WrapServer(
		server,
		grpcweb.WithOriginFunc(func(origin string) bool {
			return true
		}),
		grpcweb.WithWebsockets(true),
		grpcweb.WithWebsocketOriginFunc(func(req *http.Request) bool {
			return true
		}))

	proxy := &http.Server{
		Addr: webaddr,
	}
	proxy.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if webrpc.IsGrpcWebRequest(r) ||
			webrpc.IsAcceptableGrpcCorsRequest(r) ||
			webrpc.IsGrpcWebSocketRequest(r) {
			webrpc.ServeHTTP(w, r)
		}
	})

	fmt.Println("gRPC server started at: " + addr)
	go func() {
		server.Serve(lis)
	}()

	// do not change this, js client relies on this msg to ensure that server is up
	fmt.Println(grpcWebStartedMessagePrefix + webaddr)
	go func() {
		if err := proxy.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("proxy error: %v", err)
		}
	}()

	select {
	case <-stopChan:
		server.Stop()
		proxy.Close()
		mw.Shutdown(&pb.RpcShutdownRequest{})
		return
	}
}
