// +build !nogrpcserver,!_test

package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"google.golang.org/grpc"

	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/lib-debug"
)

const defaultAddr = "127.0.0.1:9999"

func main() {
	var addr string
	if len(os.Args) > 1 {
		addr = os.Args[len(os.Args)-1]
	} else if env := os.Getenv("ANYTYPE_GRPC_ADDR"); env != "" {
		addr = env
	} else {
		addr = defaultAddr
	}

	var stopChan = make(chan os.Signal, 2)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	var mw = &core.Middleware{}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	lib.RegisterClientCommandsServer(grpcServer, mw)

	fmt.Println("gRPC server started at: " + addr)
	go func() {
		grpcServer.Serve(lis)
	}()

	select {
	case <-stopChan:
		grpcServer.Stop()
		mw.Shutdown(&pb.RpcShutdownRequest{})
		return
	}
}
