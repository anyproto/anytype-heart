package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
