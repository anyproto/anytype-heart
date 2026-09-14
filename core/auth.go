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

// localAPISecretMetadataKey is the gRPC metadata key carrying the
// parent-delivered shared secret (see core/application/local_api_secret.go).
const localAPISecretMetadataKey = "local-api-secret"

// localAPISecretMethods must present the parent-delivered secret. They are the
// account-bootstrap subset of noAuthMethods — the ones that, left open, let a
// caller who merely reached the port re-key the wallet and mint itself a
// Full-scope token. Requiring a value only the process that spawned us knows
// (it arrives on our stdin pipe, which no other local user and no network
// caller can write to) closes that path; everything downstream is already
// token-gated, so gating the bootstrap protects the whole surface.
//
// Every other noAuthMethod must appear in localAPISecretCarveOuts; one that
// appears in neither is gated anyway (see localAPISecretRequiredFor), so the
// unsafe default is the one that costs a caller an error rather than the one
// that costs the account.
var localAPISecretMethods = map[string]struct{}{
	"WalletCreate":                   {},
	"WalletRecover":                  {},
	"AccountCreate":                  {},
	"AccountMigrate":                 {},
	"AccountMigrateCancel":           {},
	"AccountRecoverFromLegacyExport": {},
	"InitialSetParameters":           {},
	"DebugAccountSelectTrace":        {},
}

// localAPISecretCarveOuts are the methods reachable without a token that
// deliberately do NOT require the secret. Naming them is what lets the gate
// default to fail-closed: together with localAPISecretMethods they must cover
// noAuthMethods exactly, and a method in neither is treated as gated.
//
//   - AppGetVersion: a liveness probe that leaks nothing actionable.
//   - The AccountLocalLink challenge pair: the handshake for callers that have
//     no credential yet, so gating it would break every new pairing. Neither
//     yields a token on its own — the code is minted only by
//     AccountLocalLinkApproveChallenge, which is Full-token-only. Both are
//     deprecated; fold them into the gated set once they are removed.
//   - WalletCreateSession: gated per branch instead of wholesale, because only
//     two of its four branches self-mint. See localAPISecretRequiredFor.
var localAPISecretCarveOuts = map[string]struct{}{
	"AppGetVersion":                  {},
	"AccountLocalLinkNewChallenge":   {},
	"AccountLocalLinkSolveChallenge": {},
	"WalletCreateSession":            {},
}

func (mw *Middleware) Authorize(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	method := path.Base(info.FullMethod)
	if err = mw.checkLocalAPISecretForMethod(ctx, method, req); err != nil {
		return nil, err
	}

	_, noAuth := noAuthMethods[method]
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

// checkLocalAPISecretForMethod is the interceptor's secret decision. It runs
// before the noAuth early-return, since every gated method is a noAuth one.
// Enforcement is off — permissive, for standalone/Docker runs and parents that
// do not yet send a secret — until one is registered from the stdin channel.
func (mw *Middleware) checkLocalAPISecretForMethod(ctx context.Context, method string, req interface{}) error {
	if !localAPISecretRequiredFor(method, req) {
		return nil
	}
	if mw.hasValidLocalAPISecret(ctx) {
		return nil
	}
	return status.Error(codes.Unauthenticated, "missing or invalid local api secret")
}

// hasValidLocalAPISecret verifies the secret carried in the request's incoming
// metadata. It reports true when no secret is registered: standalone/Docker
// runs and parents that do not yet send one stay permissive (see the design
// spec's §3.4 — this flips to fail-closed once the desktop client ships it).
func (mw *Middleware) hasValidLocalAPISecret(ctx context.Context) bool {
	if !mw.applicationService.LocalAPISecretEnforced() {
		return true
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	values := md.Get(localAPISecretMetadataKey)
	if len(values) == 0 {
		return false
	}
	return mw.applicationService.CheckLocalAPISecret(values[0])
}

// localAPISecretRequiredFor reports whether a call must present the secret.
//
// WalletCreateSession is decided per branch rather than by name: its
// mnemonic/accountKey branches are the self-mint path — they hand out a
// Full-scope token to a caller holding no prior credential — while its
// appKey/token branches already require a credential of their own and must
// stay reachable (integrations never see the parent's secret). The branch is
// read exactly the way CreateSession dispatches it, so the two cannot drift:
// a request with neither an appKey nor a token reaches the mnemonic path, an
// empty one included, and is gated. A request of an unexpected type fails
// closed.
func localAPISecretRequiredFor(method string, req interface{}) bool {
	if _, gated := localAPISecretMethods[method]; gated {
		return true
	}
	if method == "WalletCreateSession" {
		sessionReq, ok := req.(*pb.RpcWalletCreateSessionRequest)
		if !ok {
			return true
		}
		return sessionReq.GetAppKey() == "" && sessionReq.GetToken() == ""
	}
	if _, carvedOut := localAPISecretCarveOuts[method]; carvedOut {
		return false
	}
	// A method reachable without a token that nobody classified is gated. New
	// bootstrap methods are opt-out of the secret, not opt-in: forgetting to
	// classify one then costs its caller an error, not the account.
	_, noAuth := noAuthMethods[method]
	return noAuth
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
