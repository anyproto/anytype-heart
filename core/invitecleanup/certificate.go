package invitecleanup

import (
	"context"

	"github.com/anyproto/anytype-heart/space/techspace"
)

// certificate reads the revocation a space was last certified clean for. An empty value means it has
// never been certified.
func (s *service) certificate(ctx context.Context, spaceId string) (coveredRevocation string, err error) {
	err = s.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(view techspace.SpaceView) error {
		coveredRevocation = view.GetInviteCleanupDone()
		return nil
	})
	return coveredRevocation, err
}

// certify records that the space holds no invite file that was revoked on or before coveredRevocation.
//
// Naming the revocation rather than the time is what makes the certificate reliable. Revoking an
// invite appends a record to the acl, which moves the fingerprint and so voids the certificate by
// itself, on every device, with nothing to withdraw. A certificate written by a device that had not
// yet seen a revocation names an older record than the acl now carries, and is simply disregarded —
// so the worst a stale or racing device can do is cause one more pass, never miss a file.
func (s *service) certify(ctx context.Context, spaceId string, coveredRevocation string) error {
	return s.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(view techspace.SpaceView) error {
		if view.GetInviteCleanupDone() == coveredRevocation {
			return nil
		}
		return view.SetInviteCleanupDone(coveredRevocation)
	})
}
