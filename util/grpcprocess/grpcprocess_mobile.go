//go:build gomobile

package grpcprocess

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
)

// ProcessInfo holds details about the client process.
type ProcessInfo struct {
	PID  int32
	Name string
	Path string
}

// FromContext retrieves the ProcessInfo stored by the interceptor.
func FromContext(ctx context.Context) (*ProcessInfo, bool) {
	return nil, false
}

// ProcessInfoInterceptor returns an interceptor that *only* runs for the
// gRPC methods listed in allowedMethods (exact match on info.FullMethod).
func ProcessInfoInterceptor(allowedMethods ...string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		return handler(ctx, req)
	}
}
func ResolveProcess(remoteIP, remotePort string) (*ProcessInfo, error) {
	return nil, fmt.Errorf("not supported in gomobile")
}
