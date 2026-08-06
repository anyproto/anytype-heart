//go:build !noauth
// +build !noauth

package core

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/gogo/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

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
	"AccountSelect":              {}, // need to replace with other method to get info
	"ListenSessionEvents":        {},
	"ObjectSearchSubscribe":      {},
	"ObjectCreateRelationOption": {},
	"BlockLinkCreateWithObject":  {},
	"ObjectCollectionAdd":        {},
}

// noAuthMethods run without a session token. Only add a method here if a client
// must call it before a session can exist (wallet/account bootstrap, the
// local-link pairing handshake, startup parameters). Anything a client calls
// after WalletCreateSession has a token and must go through scope validation
// instead — see limitedScopeMethods.
var noAuthMethods = map[string]struct{}{
	"AppGetVersion":                  {},
	"WalletCreate":                   {},
	"WalletRecover":                  {},
	"WalletCreateSession":            {},
	"AccountCreate":                  {},
	"AccountMigrate":                 {},
	"AccountMigrateCancel":           {},
	"AccountRecoverFromLegacyExport": {},
	"AccountLocalLinkNewChallenge":   {},
	"AccountLocalLinkSolveChallenge": {},
	"DebugAccountSelectTrace":        {},
	"InitialSetParameters":           {},
}

func (mw *Middleware) Authorize(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	_, noAuth := noAuthMethods[path.Base(info.FullMethod)]
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
	if err = checkScopeAllowsMethod(scope, info.FullMethod); err != nil {
		return nil, err
	}
	resp, err = handler(ctx, req)
	return
}

// checkScopeAllowsMethod is the interceptor's scope decision: Full passes
// everywhere, Limited only through its allowlist, and every other scope
// (JsonAPI included — its holders live on the HTTP surface) is denied all
// gRPC methods. Returns the gRPC PermissionDenied error or nil.
func checkScopeAllowsMethod(scope model.AccountAuthLocalApiScope, fullMethod string) error {
	switch scope {
	case model.AccountAuth_Full:
	case model.AccountAuth_Limited:
		methodTrimmed := strings.TrimPrefix(fullMethod, "/anytype.ClientCommands/")
		if _, ok := limitedScopeMethods[methodTrimmed]; !ok {
			return status.Error(codes.PermissionDenied, fmt.Sprintf("method %s not allowed for %s", methodTrimmed, scope.String()))
		}
	default:
		return status.Error(codes.PermissionDenied, fmt.Sprintf("method %s not allowed for %s scope", fullMethod, scope.String()))
	}
	return nil
}
