package invitecleanup

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func aclRecord(id string, contents ...*aclrecordproto.AclContentValue) *list.AclRecord {
	return &list.AclRecord{Id: id, Model: &aclrecordproto.AclData{AclContent: contents}}
}

func inviteContent(key inviteKey) *aclrecordproto.AclContentValue {
	return inviteContentTyped(key, aclrecordproto.AclInviteType_RequestToJoin)
}

func anyoneInviteContent(key inviteKey) *aclrecordproto.AclContentValue {
	return inviteContentTyped(key, aclrecordproto.AclInviteType_AnyoneCanJoin)
}

func inviteContentTyped(key inviteKey, inviteType aclrecordproto.AclInviteType) *aclrecordproto.AclContentValue {
	raw, _ := key.pub.Marshall()
	return &aclrecordproto.AclContentValue{
		Value: &aclrecordproto.AclContentValue_Invite{
			Invite: &aclrecordproto.AclAccountInvite{InviteKey: raw, InviteType: inviteType},
		},
	}
}

// removeContent is the account removal that stopping to share batches together with the revokes.
func removeContent() *aclrecordproto.AclContentValue {
	return &aclrecordproto.AclContentValue{
		Value: &aclrecordproto.AclContentValue_AccountRemove{
			AccountRemove: &aclrecordproto.AclAccountRemove{},
		},
	}
}

func revokeContent(inviteRecordId string) *aclrecordproto.AclContentValue {
	return &aclrecordproto.AclContentValue{
		Value: &aclrecordproto.AclContentValue_InviteRevoke{
			InviteRevoke: &aclrecordproto.AclAccountInviteRevoke{InviteRecordId: inviteRecordId},
		},
	}
}

// TestReadInviteRecords pins what the clean certificate is issued against. A space is certified for
// the last revocation its acl carries, so a revocation must move the fingerprint — and generating an
// invite must not, or every invite the user ever makes would rescan the space and rewrite its
// spaceview for nothing.
func TestReadInviteRecords(t *testing.T) {
	t.Run("an acl with no records at all", func(t *testing.T) {
		got := readInviteRecords(nil)

		assert.Equal(t, noRevocations, got.lastRevocation)
	})

	t.Run("generating an invite does not move the fingerprint", func(t *testing.T) {
		// nothing has been revoked, so there is nothing to delete and nothing to re-certify
		key := newInviteKey(t)
		records := []*list.AclRecord{aclRecord("rec1", inviteContent(key))}

		got := readInviteRecords(records)

		assert.Equal(t, noRevocations, got.lastRevocation)
		require.Len(t, got.known, 1)
		assert.True(t, key.pub.Equals(got.known[0]))
	})

	t.Run("revoking an invite moves the fingerprint", func(t *testing.T) {
		key := newInviteKey(t)
		records := []*list.AclRecord{
			aclRecord("rec1", inviteContent(key)),
			aclRecord("rec2", revokeContent("rec1")),
		}

		got := readInviteRecords(records)

		assert.Equal(t, "rec2", got.lastRevocation)
	})

	t.Run("the last revocation wins", func(t *testing.T) {
		first, second := newInviteKey(t), newInviteKey(t)
		records := []*list.AclRecord{
			aclRecord("rec1", inviteContent(first)),
			aclRecord("rec2", revokeContent("rec1")),
			aclRecord("rec3", inviteContent(second)),
			aclRecord("rec4", revokeContent("rec3")),
		}

		got := readInviteRecords(records)

		assert.Equal(t, "rec4", got.lastRevocation)
		assert.Len(t, got.known, 2, "both invites stay known: absence from the acl is not revocation")
	})

	t.Run("a replace is one record, and one fingerprint", func(t *testing.T) {
		// ReplaceInvite revokes the old invite and adds the new one in a single batch record
		old, fresh := newInviteKey(t), newInviteKey(t)
		records := []*list.AclRecord{
			aclRecord("rec1", inviteContent(old)),
			aclRecord("rec2", revokeContent("rec1"), inviteContent(fresh)),
		}

		got := readInviteRecords(records)

		assert.Equal(t, "rec2", got.lastRevocation)
		assert.Len(t, got.known, 2)
	})

	t.Run("stopping to share moves the fingerprint", func(t *testing.T) {
		// aclSpaceClient.StopSharing builds one batch record carrying the account removals, the read key
		// change AND an InviteRevoke per invite in use. The cleanup leans on that revoke being there:
		// without it the fingerprint would not move, and a space that was un-shared while an invite was
		// in use would keep that invite's file on the node for good.
		key := newInviteKey(t)
		records := []*list.AclRecord{
			aclRecord("rec1", inviteContent(key)),
			aclRecord("rec2", removeContent(), revokeContent("rec1")),
		}

		got := readInviteRecords(records)

		assert.Equal(t, "rec2", got.lastRevocation)
		require.Len(t, got.known, 1, "the invite record stays in the acl, so the invite stays known")
	})

	t.Run("a revoked anyoneCanJoin invite is collected for rotation", func(t *testing.T) {
		// its acl record carries the read key encrypted to the invite key, so a rotation is needed once
		// it is revoked (readAcl decides whether the read key is still current)
		anyone, member := newInviteKey(t), newInviteKey(t)
		records := []*list.AclRecord{
			aclRecord("rec1", anyoneInviteContent(anyone)),
			aclRecord("rec2", inviteContent(member)),
			aclRecord("rec3", revokeContent("rec1")), // revokes the anyoneCanJoin one
			aclRecord("rec4", revokeContent("rec2")), // revokes the request-to-join one
		}

		got := readInviteRecords(records)

		assert.Equal(t, []string{"rec1"}, got.revokedAnyoneRecords,
			"only the revoked anyoneCanJoin invite is collected, not the request-to-join one")
	})

	t.Run("a live anyoneCanJoin invite is not collected", func(t *testing.T) {
		anyone := newInviteKey(t)
		records := []*list.AclRecord{aclRecord("rec1", anyoneInviteContent(anyone))}

		got := readInviteRecords(records)

		assert.Empty(t, got.revokedAnyoneRecords, "a live invite is not up for rotation")
	})

	t.Run("the root record carries no invite", func(t *testing.T) {
		// its Model is an AclRoot, not an AclData, and must not blow up the walk
		key := newInviteKey(t)
		records := []*list.AclRecord{
			{Id: "root", Model: &aclrecordproto.AclRoot{}},
			aclRecord("rec1", inviteContent(key)),
			aclRecord("rec2", revokeContent("rec1")),
		}

		got := readInviteRecords(records)

		assert.Equal(t, "rec2", got.lastRevocation)
	})
}
