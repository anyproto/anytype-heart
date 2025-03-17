//go:build !noauth
// +build !noauth

package core

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var limitedScopeMethods = map[string]struct{}{
	"ObjectSearch":               {},
	"ObjectShow":                 {},
	"ObjectCreate":               {},
	"ObjectCreateFromUrl":        {},
	"BlockPreview":               {},
	"BlockPaste":                 {},
	"BroadcastPayloadEvent":      {},
	"AccountSelect":              {},
	"ListenSessionEvents":        {},
	"ObjectSearchSubscribe":      {},
	"ObjectCreateRelationOption": {},
	// need to replace with other method to get info
}

func (mw *Middleware) Authorize(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	d := req.(protoreflect.ProtoMessage).ProtoReflect().Descriptor()

	opts := d.Options().(protoreflect.ProtoMessage)

	noAuth := proto.GetExtension(opts, pb.E_NoAuth).(bool)
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
		methodTrimmed := strings.TrimPrefix(info.FullMethod, "/anytype.ClientCommands/")
		if _, ok := limitedScopeMethods[methodTrimmed]; !ok {
			return nil, status.Error(codes.PermissionDenied, fmt.Sprintf("method %s not allowed for %s", methodTrimmed, scope.String()))
		}
	default:
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf("method %s not allowed for %s scope", info.FullMethod, scope.String()))
	}
	resp, err = handler(ctx, req)
	return
}
