package main

import (
	"log"
	"net"
	"os"

	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/lib-debug"
	"google.golang.org/grpc"
)

const defaultAddr = "127.0.0.1:9999"

// make sure to build with `-tags debug`
func main(){
	var addr string
	if len(os.Args) > 1 {
		addr = os.Args[len(os.Args)-1]
	} else if env := os.Getenv("ANYTYPE_GRPC_ADDR"); env != "" {
		addr = env
	} else {
		addr = defaultAddr
	}

	var mw = &core.Middleware{}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	lib.RegisterClientCommandsServer(grpcServer, mw)

	grpcServer.Serve(lis)
}
