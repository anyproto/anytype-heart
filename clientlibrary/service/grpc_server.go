//go:build !nogrpcserver
// +build !nogrpcserver

package service

import (
	"context"
	"net"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/metrics"
	pb_service "github.com/anyproto/anytype-heart/pb/service"
)

var grpcSrv *grpc.Server

// StartGrpcServer starts a gRPC server on the given address.
// Pass "127.0.0.1:0" to let the OS pick a free port.
// Returns the actual bound address (e.g. "127.0.0.1:54321"), or empty string on failure.
func StartGrpcServer(addr string) string {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return ""
	}

	// Switch from mobile callback model to gRPC event streaming.
	// Must be called before any gRPC client connects.
	mw.SetEventSender(event.NewGrpcSender())

	server := grpc.NewServer(
		grpc.MaxRecvMsgSize(20*1024*1024),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			metrics.UnaryTraceInterceptor,
			func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
				handler grpc.UnaryHandler) (interface{}, error) {
				return mw.Authorize(ctx, req, info, handler)
			},
		)),
	)
	pb_service.RegisterClientCommandsServer(server, mw)

	grpcSrv = server
	go server.Serve(lis)

	return lis.Addr().String()
}

// StopGrpcServer gracefully stops the gRPC server.
// Safe to call if the server was never started.
func StopGrpcServer() {
	if grpcSrv != nil {
		grpcSrv.Stop()
		grpcSrv = nil
	}
}
