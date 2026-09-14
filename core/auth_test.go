//go:build !noauth
// +build !noauth

package core

import (
	"context"
	"testing"

	"github.com/gogo/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestNoAuthMethods_ExcludesSessionBackedMethods guards the security-relevant
// membership of noAuthMethods: a method a client only calls after it holds a
// session token must not be reachable without one.
func TestNoAuthMethods_ExcludesSessionBackedMethods(t *testing.T) {
	// AccountSelect is always preceded by WalletCreateSession, so the client has
	// a token; it stays reachable through limited scope instead.
	assert.NotContains(t, noAuthMethods, "AccountSelect")
	assert.Contains(t, limitedScopeMethods, "AccountSelect")

	// ObjectImport operates on an existing space, which only exists after
	// account selection, so the caller already has a (full-scope) token.
	assert.NotContains(t, noAuthMethods, "ObjectImport")

	// DebugRunProfiler runs a heavy profiler; requiring a token keeps an
	// anonymous caller from triggering it.
	assert.NotContains(t, noAuthMethods, "DebugRunProfiler")

	// AccountLocalLinkUpdateApp edits a live key's grant — including CLEARING
	// it, which turns a scoped key unscoped. It must stay Full-only: its
	// pairing-flow neighbours (NewChallenge/SolveChallenge) are legitimately
	// pre-session, but UpdateApp reachable without a token (or via Limited)
	// would let any local process widen any app key by hash — and ListApps
	// hands the hashes out.
	assert.NotContains(t, noAuthMethods, "AccountLocalLinkUpdateApp")
	assert.NotContains(t, limitedScopeMethods, "AccountLocalLinkUpdateApp")

	// The genuine pre-session bootstrap methods must stay exempt.
	for _, method := range []string{
		"WalletCreateSession",
		"AccountCreate",
		"AccountLocalLinkNewChallenge",
		"AccountLocalLinkSolveChallenge",
		"InitialSetParameters",
	} {
		assert.Contains(t, noAuthMethods, method)
	}
}

// TestCheckScopeAllowsMethod pins the Authorize interceptor's scope decision.
// The JsonAPI cases guard P0.1 in
// docs/superpowers/specs/2026-08-06-api-key-scoping-design.md: a pairing
// session minted for a JsonAPI challenge must be denied every gRPC method —
// before the fix it was minted as Limited and ObjectSearch (any space) passed.
func TestCheckScopeAllowsMethod(t *testing.T) {
	tests := []struct {
		name       string
		scope      model.AccountAuthLocalApiScope
		fullMethod string
		wantDenied bool
	}{
		{
			name:       "JsonAPI denied ObjectSearch",
			scope:      model.AccountAuth_JsonAPI,
			fullMethod: "/anytype.ClientCommands/ObjectSearch",
			wantDenied: true,
		},
		{
			name:       "JsonAPI denied ObjectShow",
			scope:      model.AccountAuth_JsonAPI,
			fullMethod: "/anytype.ClientCommands/ObjectShow",
			wantDenied: true,
		},
		{
			name:       "JsonAPI denied even Limited-allowlisted methods",
			scope:      model.AccountAuth_JsonAPI,
			fullMethod: "/anytype.ClientCommands/BlockPreview",
			wantDenied: true,
		},
		{
			name:       "Limited allowed allowlisted ObjectSearch",
			scope:      model.AccountAuth_Limited,
			fullMethod: "/anytype.ClientCommands/ObjectSearch",
			wantDenied: false,
		},
		{
			name:       "Limited denied non-allowlisted ObjectImport",
			scope:      model.AccountAuth_Limited,
			fullMethod: "/anytype.ClientCommands/ObjectImport",
			wantDenied: true,
		},
		{
			name:       "Full allowed everything",
			scope:      model.AccountAuth_Full,
			fullMethod: "/anytype.ClientCommands/ObjectSearch",
			wantDenied: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// when
			err := checkScopeAllowsMethod(tt.scope, tt.fullMethod)

			// then
			if tt.wantDenied {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, codes.PermissionDenied, st.Code())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestApproveChallengeRequiresFullScope pins the one membership the local-link
// approval flow depends on. Approving a pairing is the user's decision, made in
// the desktop UI, which already holds a full-scope session. Absence from both
// maps is what enforces that: Authorize falls through to the scope switch, which
// admits AccountAuth_Full alone and refuses Limited and JsonAPI.
//
// Listing it in noAuthMethods would let any local process — including the one
// asking to be paired — approve itself, which is weaker than the flow it
// replaced. Listing it in limitedScopeMethods would hand the same power to every
// app the user paired earlier.
func TestApproveChallengeRequiresFullScope(t *testing.T) {
	assert.NotContains(t, noAuthMethods, "AccountLocalLinkApproveChallenge")
	assert.NotContains(t, limitedScopeMethods, "AccountLocalLinkApproveChallenge")
}

// TestLocalAPISecretGatedMethods pins the set of methods that require the
// parent-delivered secret. Adding a bootstrap method to noAuthMethods without
// adding it here would reopen the self-mint path this gate closes.
func TestLocalAPISecretGatedMethods(t *testing.T) {
	want := []string{
		"WalletCreate",
		"WalletRecover",
		"AccountCreate",
		"AccountMigrate",
		"AccountMigrateCancel",
		"AccountRecoverFromLegacyExport",
		"InitialSetParameters",
		"DebugAccountSelectTrace",
	}

	assert.Len(t, localAPISecretMethods, len(want))
	for _, method := range want {
		assert.Contains(t, localAPISecretMethods, method)
		// Every gated method is a pre-session bootstrap method: the gate is
		// what replaces the missing token check for them.
		assert.Contains(t, noAuthMethods, method)
	}

	// Carve-outs, per the design spec: a liveness probe that leaks nothing, and
	// the deprecated pairing handshake (gating it would break all new pairing).
	// WalletCreateSession is absent because it is gated per branch instead —
	// see TestAuthorizeLocalAPISecretWalletCreateSession.
	for _, method := range []string{
		"AppGetVersion",
		"AccountLocalLinkNewChallenge",
		"AccountLocalLinkSolveChallenge",
		"WalletCreateSession",
	} {
		assert.NotContains(t, localAPISecretMethods, method)
	}
}

func TestAuthorizeLocalAPISecret(t *testing.T) {
	const secret = "parent-delivered-secret"

	newMiddleware := func(enforced bool) *Middleware {
		mw := New()
		if enforced {
			mw.applicationService.SetLocalAPISecret(secret)
		}
		return mw
	}

	authorize := func(t *testing.T, mw *Middleware, method string, md metadata.MD) (bool, error) {
		t.Helper()
		ctx := context.Background()
		if md != nil {
			ctx = metadata.NewIncomingContext(ctx, md)
		}
		handlerCalled := false
		_, err := mw.Authorize(ctx, nil,
			&grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/" + method},
			func(context.Context, interface{}) (interface{}, error) {
				handlerCalled = true
				return nil, nil
			})
		return handlerCalled, err
	}

	t.Run("gated method passes with the correct secret", func(t *testing.T) {
		// given
		mw := newMiddleware(true)

		// when
		called, err := authorize(t, mw, "WalletCreate", metadata.Pairs(localAPISecretMetadataKey, secret))

		// then
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("gated method is rejected with a wrong secret", func(t *testing.T) {
		// given
		mw := newMiddleware(true)

		// when
		called, err := authorize(t, mw, "WalletCreate", metadata.Pairs(localAPISecretMetadataKey, "guessed"))

		// then
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.False(t, called)
	})

	t.Run("gated method is rejected without a secret", func(t *testing.T) {
		// given
		mw := newMiddleware(true)

		// when
		called, err := authorize(t, mw, "AccountCreate", metadata.Pairs("token", "whatever"))

		// then
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.False(t, called)
	})

	t.Run("gated method is rejected without any metadata", func(t *testing.T) {
		// given
		mw := newMiddleware(true)

		// when
		called, err := authorize(t, mw, "InitialSetParameters", nil)

		// then
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.False(t, called)
	})

	t.Run("carve-out methods pass without a secret", func(t *testing.T) {
		for _, method := range []string{
			"AppGetVersion",
			"AccountLocalLinkNewChallenge",
			"AccountLocalLinkSolveChallenge",
		} {
			t.Run(method, func(t *testing.T) {
				// given
				mw := newMiddleware(true)

				// when
				called, err := authorize(t, mw, method, nil)

				// then
				require.NoError(t, err)
				assert.True(t, called)
			})
		}
	})

	t.Run("gated method passes without a secret when enforcement is off", func(t *testing.T) {
		// given
		mw := newMiddleware(false)

		// when
		called, err := authorize(t, mw, "WalletCreate", nil)

		// then
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("token-gated methods still need a token, secret or not", func(t *testing.T) {
		// given
		mw := newMiddleware(true)

		// when
		called, err := authorize(t, mw, "ObjectSearch", metadata.Pairs(localAPISecretMetadataKey, secret))

		// then
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.False(t, called)
	})
}

// TestAuthorizeLocalAPISecretWalletCreateSession covers the branch-level gate.
// WalletCreateSession is the self-mint path: its mnemonic/accountKey branches
// hand out a Full-scope token with no prior credential, so they need the
// secret. Its appKey/token branches already require a credential of their own
// and stay reachable without it.
func TestAuthorizeLocalAPISecretWalletCreateSession(t *testing.T) {
	const secret = "parent-delivered-secret"

	authorize := func(t *testing.T, enforced bool, auth pb.IsRpcWalletCreateSessionRequestAuth, md metadata.MD) (bool, error) {
		t.Helper()
		mw := New()
		if enforced {
			mw.applicationService.SetLocalAPISecret(secret)
		}
		ctx := context.Background()
		if md != nil {
			ctx = metadata.NewIncomingContext(ctx, md)
		}
		handlerCalled := false
		_, err := mw.Authorize(ctx, &pb.RpcWalletCreateSessionRequest{Auth: auth},
			&grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/WalletCreateSession"},
			func(context.Context, interface{}) (interface{}, error) {
				handlerCalled = true
				return nil, nil
			})
		return handlerCalled, err
	}

	mnemonic := &pb.RpcWalletCreateSessionRequestAuthOfMnemonic{Mnemonic: "some words"}
	accountKey := &pb.RpcWalletCreateSessionRequestAuthOfAccountKey{AccountKey: "some key"}
	appKey := &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: "app key"}
	token := &pb.RpcWalletCreateSessionRequestAuthOfToken{Token: "session token"}

	t.Run("mnemonic branch is rejected without the secret", func(t *testing.T) {
		// when
		called, err := authorize(t, true, mnemonic, nil)

		// then
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.False(t, called)
	})

	t.Run("accountKey branch is rejected without the secret", func(t *testing.T) {
		// when
		called, err := authorize(t, true, accountKey, metadata.Pairs(localAPISecretMetadataKey, "guessed"))

		// then
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.False(t, called)
	})

	t.Run("mnemonic branch passes with the secret", func(t *testing.T) {
		// when
		called, err := authorize(t, true, mnemonic, metadata.Pairs(localAPISecretMetadataKey, secret))

		// then
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("appKey branch is unaffected", func(t *testing.T) {
		// when
		called, err := authorize(t, true, appKey, nil)

		// then
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("token branch is unaffected", func(t *testing.T) {
		// when
		called, err := authorize(t, true, token, nil)

		// then
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("mnemonic branch passes when enforcement is off", func(t *testing.T) {
		// when
		called, err := authorize(t, false, mnemonic, nil)

		// then
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("an empty request is treated as a self-mint attempt", func(t *testing.T) {
		// A request with no auth branch set falls through CreateSession to the
		// mnemonic path, so it must not be a way around the gate.
		// when
		called, err := authorize(t, true, nil, nil)

		// then
		require.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
		assert.False(t, called)
	})
}
