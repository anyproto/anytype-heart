package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
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

// TestAccountLocalLinkApproveChallengeErrorCode pins the approval RPC's error
// mapping, the sibling of the NewChallenge pin above. The grant rows are
// load-bearing: every grant refusal from the approve path wraps
// wallet.ErrInvalidGrant — a missing grant on a JsonAPI approval included —
// and must answer BAD_INPUT, a permanent input mistake the desktop can show,
// never code 1 UNKNOWN_ERROR.
func TestAccountLocalLinkApproveChallengeErrorCode(t *testing.T) {
	t.Run("grant refusals map to BAD_INPUT", func(t *testing.T) {
		// the exact error shapes session.ApproveChallenge produces
		for _, err := range []error{
			fmt.Errorf("validate approval grant: %w", fmt.Errorf("%w: a JsonAPI approval requires a grant", walletComp.ErrInvalidGrant)),
			fmt.Errorf("validate approval grant: validate grant: %w", fmt.Errorf("%w: spaces must be non-empty", walletComp.ErrInvalidGrant)),
			fmt.Errorf("validate approval grant: validate grant: %w", fmt.Errorf("%w: allSpaces and an explicit space list are mutually exclusive", walletComp.ErrInvalidGrant)),
		} {
			assert.Equal(t, pb.RpcAccountLocalLinkApproveChallengeResponseError_BAD_INPUT,
				accountLocalLinkApproveChallengeErrorCode(err), "error %v", err)
		}
	})

	t.Run("the sibling rows still map", func(t *testing.T) {
		assert.Equal(t, pb.RpcAccountLocalLinkApproveChallengeResponseError_NO_PENDING_CHALLENGE,
			accountLocalLinkApproveChallengeErrorCode(session.ErrNoPendingChallenge))
		assert.Equal(t, pb.RpcAccountLocalLinkApproveChallengeResponseError_BAD_INPUT,
			accountLocalLinkApproveChallengeErrorCode(errBrowserCallerNotAllowed))
		assert.Equal(t, pb.RpcAccountLocalLinkApproveChallengeResponseError_ACCOUNT_IS_NOT_RUNNING,
			accountLocalLinkApproveChallengeErrorCode(application.ErrApplicationIsNotRunning))
	})

	t.Run("an unrecognized error stays UNKNOWN", func(t *testing.T) {
		assert.Equal(t, pb.RpcAccountLocalLinkApproveChallengeResponseError_UNKNOWN_ERROR,
			accountLocalLinkApproveChallengeErrorCode(errors.New("boom")))
	})
}
