package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/localorigin"
)

// TestAccountLocalLinkApproveChallenge_RefusesBrowserCallers checks the second
// half of the approval flow's access control. Scope handling in Authorize keeps
// out unauthenticated and limited callers; this keeps out browser contexts,
// which the gRPC-Web proxy admits for the Webclipper's extension origins.
//
// The Middleware here has no applicationService: reaching it would panic, so the
// test also pins that the origin check runs before any work.
func TestAccountLocalLinkApproveChallenge_RefusesBrowserCallers(t *testing.T) {
	for _, origin := range []string{
		"chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf", // the trusted Webclipper is still a browser
		"http://localhost:3000",
		"https://evil.com",
	} {
		t.Run(origin, func(t *testing.T) {
			// given
			mw := &Middleware{}
			ctx := localorigin.WithOrigin(context.Background(), origin)

			// when
			resp := mw.AccountLocalLinkApproveChallenge(ctx, &pb.RpcAccountLocalLinkApproveChallengeRequest{
				Origin: origin,
				Allow:  true,
			})

			// then
			require.NotNil(t, resp.Error)
			assert.Equal(t, pb.RpcAccountLocalLinkApproveChallengeResponseError_BAD_INPUT, resp.Error.Code)
			assert.Empty(t, resp.Challenge, "a refused call must never carry a code")
		})
	}
}

// TestLocalLinkErrorCodesAreReachable pins the wire vocabulary of the pairing
// flow. Every error below existed and was documented, but none had a mapping
// row, so all three answered UNKNOWN_ERROR (1):
//
//   - CHALLENGE_NOT_APPROVED was declared in the proto and promised by
//     LocalLinkPairingApproval.md, and nothing could ever return it. A gRPC
//     client could not tell "waiting for the human" — now a state lasting up
//     to pendingChallengeTTL by design — from a real failure.
//   - PendingApproval and Denied are the two most common post-launch
//     outcomes (asked twice; previously refused). Both were documented as
//     TOO_MANY_REQUESTS, and a client seeing UNKNOWN_ERROR can only retry,
//     which burns budget and re-prompts.
//
// The table asserts on the mapper, which is where the rows live; the join to
// the real errors is by errors.Is on the sentinels the session package
// returns, so a renamed or unwrapped sentinel fails here.
func TestLocalLinkErrorCodesAreReachable(t *testing.T) {
	t.Run("solve", func(t *testing.T) {
		tests := []struct {
			name string
			err  error
			want pb.RpcAccountLocalLinkSolveChallengeResponseErrorCode
		}{
			{"pending approval is not a wrong answer", session.ErrChallengeNotApproved,
				pb.RpcAccountLocalLinkSolveChallengeResponseError_CHALLENGE_NOT_APPROVED},
			{"a wrong answer still says so", session.ErrChallengeSolutionWrong,
				pb.RpcAccountLocalLinkSolveChallengeResponseError_INCORRECT_ANSWER},
			{"an unknown id still says so", session.ErrChallengeIdNotFound,
				pb.RpcAccountLocalLinkSolveChallengeResponseError_INVALID_CHALLENGE_ID},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.want, accountLocalLinkSolveChallengeErrorCode(tt.err))
			})
		}
	})

	t.Run("new challenge", func(t *testing.T) {
		tests := []struct {
			name string
			err  error
			want pb.RpcAccountLocalLinkNewChallengeResponseErrorCode
		}{
			{"a prompt is already up for this caller", session.ErrChallengePendingApproval,
				pb.RpcAccountLocalLinkNewChallengeResponseError_TOO_MANY_REQUESTS},
			{"the user already said no", session.ErrChallengeDenied,
				pb.RpcAccountLocalLinkNewChallengeResponseError_TOO_MANY_REQUESTS},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.want, accountLocalLinkNewChallengeErrorCode(tt.err))
			})
		}
	})
}
