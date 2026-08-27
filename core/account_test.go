package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
)

// TestAccountLocalLinkNewChallengeErrorCode pins the challenge RPC's error
// mapping (review H2). The load-bearing row is application.ErrBadInput →
// BAD_INPUT: the §11.7 issuance guards (empty / over-long app name) join
// with that sentinel, and before the row existed the RPC answered code 1
// UNKNOWN_ERROR — a pairing client branching on the code showed "something
// went wrong" instead of "app name is required" for a permanent input
// mistake, while its sibling CreateApp mapped the same error correctly.
func TestAccountLocalLinkNewChallengeErrorCode(t *testing.T) {
	t.Run("the issuance guards' ErrBadInput maps to BAD_INPUT — the H2 row", func(t *testing.T) {
		// the exact error shapes LinkLocalStartNewChallenge produces
		for _, err := range []error{
			errors.Join(application.ErrBadInput, errors.New("app name is required")),
			errors.Join(application.ErrBadInput, errors.New("app name exceeds 128 bytes")),
		} {
			assert.Equal(t, pb.RpcAccountLocalLinkNewChallengeResponseError_BAD_INPUT,
				accountLocalLinkNewChallengeErrorCode(err), "error %v", err)
		}
	})

	t.Run("the sibling rows still map", func(t *testing.T) {
		assert.Equal(t, pb.RpcAccountLocalLinkNewChallengeResponseError_TOO_MANY_REQUESTS,
			accountLocalLinkNewChallengeErrorCode(session.ErrTooManyChallengeRequests))
		assert.Equal(t, pb.RpcAccountLocalLinkNewChallengeResponseError_BAD_INPUT,
			accountLocalLinkNewChallengeErrorCode(session.ErrInvalidScope))
		assert.Equal(t, pb.RpcAccountLocalLinkNewChallengeResponseError_ACCOUNT_IS_NOT_RUNNING,
			accountLocalLinkNewChallengeErrorCode(application.ErrApplicationIsNotRunning))
	})

	t.Run("an unrecognized error stays UNKNOWN", func(t *testing.T) {
		assert.Equal(t, pb.RpcAccountLocalLinkNewChallengeResponseError_UNKNOWN_ERROR,
			accountLocalLinkNewChallengeErrorCode(errors.New("boom")))
	})

	t.Run("nil is NULL", func(t *testing.T) {
		assert.Equal(t, pb.RpcAccountLocalLinkNewChallengeResponseError_NULL,
			accountLocalLinkNewChallengeErrorCode(nil))
	})
}
