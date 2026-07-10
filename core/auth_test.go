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
