//go:build !noauth
// +build !noauth

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
