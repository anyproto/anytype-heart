//go:build !noauth
// +build !noauth

package core

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
	"github.com/gogo/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) Authorize(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	_, d := descriptor.ForMessage(req.(descriptor.Message))
	noAuth := proto.GetBoolExtension(d.GetOptions(), pb.E_NoAuth, false)
	if noAuth {
		resp, err = handler(ctx, req)
		return
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("missing metadata")
	}
	v := md.Get("token")
	if len(v) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing token")
	}
	tok := v[0]

	err = mw.applicationService.ValidateSessionToken(tok)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	resp, err = handler(ctx, req)
	return
}
