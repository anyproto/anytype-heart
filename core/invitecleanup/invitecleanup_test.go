package invitecleanup

import (
	"testing"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testSpaceId = "spaceId"

// inviteKey is an invite keypair, plus the acl key as it is marshalled into the invite file.
type inviteKey struct {
	pub crypto.PubKey
	raw []byte
}

func newInviteKey(t *testing.T) inviteKey {
	t.Helper()
	privKey, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	raw, err := privKey.Marshall()
	require.NoError(t, err)
	return inviteKey{pub: pubKey, raw: raw}
}

func (k inviteKey) payload() *model.InvitePayload {
	return &model.InvitePayload{
		SpaceId:    testSpaceId,
		AclKey:     k.raw,
		InviteType: model.InviteType_WithoutApprove,
	}
}

// revokedAcl is an acl that carries the invite's record but no longer honours it.
func revokedAcl(keys ...inviteKey) aclView {
	view := aclView{isOwner: true}
	for _, k := range keys {
		view.known = append(view.known, k.pub)
	}
	return view
}

// liveAcl is an acl that still honours the invite.
func liveAcl(k inviteKey) aclView {
	return aclView{isOwner: true, live: []crypto.PubKey{k.pub}, known: []crypto.PubKey{k.pub}}
}

func TestDecide(t *testing.T) {
	const (
		detailsSayLive = true
		detailsSayGone = false
	)

	t.Run("revoked invite is deleted", func(t *testing.T) {
		// given an invite whose record the acl carries but no longer honours, and which the workspace
		// has dropped: the two agree it is revoked
		key := newInviteKey(t)

		got, reason := decide(key.payload(), testSpaceId, revokedAcl(key), detailsSayGone)

		assert.Equal(t, dispositionDelete, got)
		assert.NoError(t, reason)
	})

	t.Run("revoked invite is deleted while another one is live", func(t *testing.T) {
		revoked, live := newInviteKey(t), newInviteKey(t)
		acl := revokedAcl(revoked, live)
		acl.live = []crypto.PubKey{live.pub}

		got, _ := decide(revoked.payload(), testSpaceId, acl, detailsSayGone)

		assert.Equal(t, dispositionDelete, got)
	})

	t.Run("live invite is left alone", func(t *testing.T) {
		// given an invite the acl still honours and the workspace still points at: an invite in use
		key := newInviteKey(t)

		got, reason := decide(key.payload(), testSpaceId, liveAcl(key), detailsSayLive)

		assert.Equal(t, dispositionResolved, got, "nothing to clean up, and the space may be certified")
		assert.ErrorIs(t, reason, errInviteLive)
	})

	t.Run("live invite the workspace has forgotten is deferred", func(t *testing.T) {
		// the acl honours it, so its file stays; the workspace disagrees, so the space is not clean
		// either
		key := newInviteKey(t)

		got, reason := decide(key.payload(), testSpaceId, liveAcl(key), detailsSayGone)

		assert.Equal(t, dispositionDefer, got)
		assert.ErrorIs(t, reason, errInviteLive)
	})

	t.Run("invite the acl has never seen is deferred, not deleted", func(t *testing.T) {
		// given an invite whose acl record this device has not got. Absence is not proof of revocation:
		// the node may have accepted the record without us hearing about it, in which case the invite is
		// still in use.
		key, other := newInviteKey(t), newInviteKey(t)
		acl := revokedAcl(other) // the invite's own key is not among the known ones

		got, reason := decide(key.payload(), testSpaceId, acl, detailsSayGone)

		assert.Equal(t, dispositionDefer, got)
		assert.ErrorIs(t, reason, errInviteUnknown)
	})

	t.Run("invite the acl calls revoked but the workspace calls current is deferred", func(t *testing.T) {
		// one of the two logs is behind and there is no way to tell which. Deleting could remove the
		// file of an invite in use; certifying the space would hide a revoked one forever.
		key := newInviteKey(t)

		got, reason := decide(key.payload(), testSpaceId, revokedAcl(key), detailsSayLive)

		assert.Equal(t, dispositionDefer, got)
		assert.ErrorIs(t, reason, errInviteDisputed)
	})

	t.Run("guest invite is skipped", func(t *testing.T) {
		// a guest invite is never revoked
		key := newInviteKey(t)
		payload := &model.InvitePayload{
			SpaceId:    testSpaceId,
			GuestKey:   key.raw,
			InviteType: model.InviteType_Guest,
		}

		got, reason := decide(payload, testSpaceId, revokedAcl(key), detailsSayGone)

		assert.Equal(t, dispositionSkip, got)
		assert.ErrorIs(t, reason, errGuestInvite)
	})

	t.Run("invite without an acl key is skipped", func(t *testing.T) {
		payload := &model.InvitePayload{SpaceId: testSpaceId, InviteType: model.InviteType_WithoutApprove}

		got, reason := decide(payload, testSpaceId, aclView{}, detailsSayGone)

		assert.Equal(t, dispositionSkip, got)
		assert.ErrorIs(t, reason, errGuestInvite)
	})

	t.Run("invite of another space is skipped", func(t *testing.T) {
		key := newInviteKey(t)
		payload := key.payload()
		payload.SpaceId = "otherSpaceId"

		got, reason := decide(payload, testSpaceId, revokedAcl(key), detailsSayGone)

		assert.Equal(t, dispositionSkip, got)
		assert.Error(t, reason)
	})

	t.Run("malformed acl key is skipped", func(t *testing.T) {
		payload := &model.InvitePayload{
			SpaceId:    testSpaceId,
			AclKey:     []byte("not a key"),
			InviteType: model.InviteType_WithoutApprove,
		}

		got, reason := decide(payload, testSpaceId, aclView{isOwner: true}, detailsSayGone)

		assert.Equal(t, dispositionSkip, got)
		assert.Error(t, reason)
	})

	t.Run("a request-to-join invite is deleted too", func(t *testing.T) {
		// every revoked invite is cleaned up, not only the ones anyone could join with
		key := newInviteKey(t)
		payload := key.payload()
		payload.InviteType = model.InviteType_Member

		got, _ := decide(payload, testSpaceId, revokedAcl(key), detailsSayGone)

		assert.Equal(t, dispositionDelete, got)
	})
}

// TestDecideKeepsInvitesInUse is the invariant the whole design exists to hold: decide must not say
// delete for any invite the acl still honours, whatever the workspace happens to say.
func TestDecideKeepsInvitesInUse(t *testing.T) {
	key := newInviteKey(t)

	for _, detailsSayLive := range []bool{true, false} {
		got, _ := decide(key.payload(), testSpaceId, liveAcl(key), detailsSayLive)

		assert.NotEqual(t, dispositionDelete, got, "detailsSayLive=%v", detailsSayLive)
	}
}
