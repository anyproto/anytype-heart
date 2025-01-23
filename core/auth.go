//go:build !noauth
// +build !noauth

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
	"github.com/gogo/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var limitedScopeMethods = map[string]struct{}{
	"ObjectSearch":          {},
	"ObjectShow":            {},
	"ObjectCreate":          {},
	"ObjectCreateFromURL":   {},
	"BlockPreview":          {},
	"BlockPaste":            {},
	"BroadcastPayloadEvent": {},
	"AccountSelect":         {}, // need to replace with other method to get info
}

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

	var scope model.AccountAuthLocalApiScope
	scope, err = mw.applicationService.ValidateSessionToken(tok)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	switch scope {
	case model.AccountAuth_Full:
	case model.AccountAuth_Limited:
		if _, ok := limitedScopeMethods[strings.TrimPrefix(info.FullMethod, "/anytype.ClientCommands/")]; !ok {
			return nil, status.Error(codes.PermissionDenied, "method not allowed for limited scope")
		}
	default:
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf("method not allowed for %s scope", scope.String()))
	}
	resp, err = handler(ctx, req)
	return
}
