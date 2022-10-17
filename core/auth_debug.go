//go:build noauth
// +build noauth

package core

import (
	"context"

	"google.golang.org/grpc"
)

func (mw *Middleware) Authorize(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	resp, err = handler(ctx, req)
	return
}
