//go:build !noauth
// +build !noauth

package core

import (
	"testing"

	"github.com/gogo/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

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
// The JsonAPI cases guard the H1 fix (docs/ApiKeyScopingResearch.md §2.2): a
// pairing session minted for a JsonAPI challenge must be denied every gRPC
// method — before the fix it was minted as Limited and ObjectSearch (any
// space) passed.
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
