package invitecleanup

import (
	"fmt"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// disposition is what cleaning up one invite means for the space it belongs to.
type disposition int

const (
	// dispositionDefer: come back to this space later. Either something transient got in the way, or
	// the local state is not consistent enough to act on. The space must not be certified: there is,
	// or may yet be, a file to delete.
	//
	// It is first so that it is the zero value. A disposition nobody set must cost an extra pass, not
	// a certificate that silences the cleanup on every device.
	dispositionDefer disposition = iota
	// dispositionResolved: the file is gone, or there was never one to delete, or the invite is live
	// and both the acl and the workspace agree that it is. Nothing outstanding.
	dispositionResolved
	// dispositionSkip: this invite can never be cleaned up, and nothing will change that. It does not
	// keep the space from being certified clean, otherwise the pass would run forever.
	dispositionSkip
	// dispositionDelete: the invite is provably revoked and its file may be deleted. Only decide()
	// hands this out.
	dispositionDelete
)

// decide says whether an invite file may be deleted.
//
// Deleting one cannot be undone, so it errs on the side of keeping it: it says delete only when the
// acl positively shows the invite was revoked AND the workspace agrees the invite is gone. Where the
// two disagree, or where the acl has never heard of the invite, it defers rather than guess — they
// are separately synced logs and either of them can be behind.
func decide(payload *model.InvitePayload, spaceId string, acl aclView, detailsSayLive bool) (disposition, error) {
	if payload.InviteType == model.InviteType_Guest || len(payload.AclKey) == 0 {
		// a guest invite is never revoked
		return dispositionSkip, errGuestInvite
	}
	if payload.SpaceId != spaceId {
		return dispositionSkip, fmt.Errorf("invite belongs to space %s", payload.SpaceId)
	}
	inviteKey, err := crypto.UnmarshalEd25519PrivateKeyProto(payload.AclKey)
	if err != nil {
		return dispositionSkip, fmt.Errorf("unmarshal invite acl key: %w", err)
	}
	key := inviteKey.GetPublic()

	// Deleting is the one thing here that cannot be undone, so it is spelled out as its own condition
	// rather than left to fall out of the bottom of a switch: it takes all three of these, and
	// anything else defers.
	if acl.isKnown(key) && !acl.isLive(key) && !detailsSayLive {
		return dispositionDelete, nil
	}

	switch {
	case acl.isLive(key) && detailsSayLive:
		// the invite is still in use and the space knows it. Nothing to clean up: revoking it will append
		// a record to the acl, which voids the space's certificate and brings this pass back.
		return dispositionResolved, errInviteLive
	case acl.isLive(key):
		// the acl honours an invite the workspace has already forgotten
		return dispositionDefer, errInviteLive
	case !acl.isKnown(key):
		// this device has never seen a record for this invite. Absence is not proof of revocation: the
		// record may have been accepted by the node and simply not synced back yet, in which case the
		// invite is still in use.
		return dispositionDefer, errInviteUnknown
	default:
		// the acl says revoked, the workspace says current. One of the two is behind and there is no
		// way to tell which, so neither delete the file nor certify the space.
		return dispositionDefer, errInviteDisputed
	}
}
