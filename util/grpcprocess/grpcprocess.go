//go:build !gomobile

package grpcprocess

import (
	"context"
	"fmt"
	"net"
	"os"

	gnet "github.com/shirou/gopsutil/v4/net"
	gproc "github.com/shirou/gopsutil/v4/process"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

// ProcessInfo holds details about the client process.
type ProcessInfo struct {
	PID  int32
	Name string
	Path string
}

// FromContext retrieves the ProcessInfo stored by the interceptor.
func FromContext(ctx context.Context) (*ProcessInfo, bool) {
	pi, ok := ctx.Value(processInfoKey).(*ProcessInfo)
	return pi, ok
}

// WithProcessInfo stores resolved process details on the context. The gRPC
// interceptor does this itself; the JSON API needs it because its requests
// never pass through an interceptor.
func WithProcessInfo(ctx context.Context, pi *ProcessInfo) context.Context {
	if pi == nil {
		return ctx
	}
	return context.WithValue(ctx, processInfoKey, pi)
}

// ProcessInfoInterceptor returns an interceptor that *only* runs for the
// gRPC methods listed in allowedMethods (exact match on info.FullMethod).
func ProcessInfoInterceptor(allowedMethods ...string) grpc.UnaryServerInterceptor {
	allow := make(map[string]struct{}, len(allowedMethods))
	for _, m := range allowedMethods {
		allow[m] = struct{}{}
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if _, ok := allow[info.FullMethod]; !ok {
			return handler(ctx, req)
		}
		if _, ok := ctx.Value(processInfoKey).(*ProcessInfo); ok {
			// already set
			return handler(ctx, req)
		}
		if p, ok := peer.FromContext(ctx); ok {
			if host, port, err := net.SplitHostPort(p.Addr.String()); err == nil {
				ip := net.ParseIP(host)
				if ip.IsLoopback() {
					if pi, err := ResolveProcess(host, port); err == nil {
						ctx = context.WithValue(ctx, processInfoKey, pi)
					}
				}
			}
		}
		return handler(ctx, req)
	}
}

type ctxKey string

const processInfoKey ctxKey = "processInfo"

func ResolveProcess(remoteIP, remotePort string) (*ProcessInfo, error) {
	conns, err := gnet.Connections("tcp")
	if err != nil {
		return nil, err
	}

	self := int32(os.Getpid())
	for _, c := range conns {
		if c.Pid == self || c.Status != "ESTABLISHED" {
			continue
		}

		if fmt.Sprint(c.Laddr.IP) == remoteIP &&
			fmt.Sprint(c.Laddr.Port) == remotePort {

			proc, err := gproc.NewProcess(c.Pid)
			if err != nil {
				// The process went away between listing and lookup, or we may
				// not inspect it. Keep scanning: another entry may match.
				continue
			}
			name, _ := proc.Name()
			exe, _ := proc.Exe()
			return &ProcessInfo{PID: c.Pid, Name: name, Path: exe}, nil
		}
	}
	return nil, fmt.Errorf("process for %s:%s not found", remoteIP, remotePort)
}
