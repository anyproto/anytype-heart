package editor

import (
	"github.com/anyproto/any-sync/commonspace/object/acl/list"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A space invite lives in exactly one of two objects. An invite shared within the space sits in the
// workspace, which every member syncs, so any member can hand the link out. An invite held by the
// owner sits in their space view, which only their own devices sync; the workspace is left with
// nothing but the spaceInviteHeldByOwner marker, which is how members learn there is an invite and
// that the owner is the one to ask for it.
//
// Both objects read and write the invite through the details below.

var inviteDetailKeys = []domain.RelationKey{
	bundle.RelationKeySpaceInviteFileCid,
	bundle.RelationKeySpaceInviteFileKey,
	bundle.RelationKeySpaceInviteType,
	bundle.RelationKeySpaceInvitePermissions,
}

func setInviteDetails(st *state.State, info domain.InviteInfo) {
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInvitePermissions, domain.Int64(domain.ConvertAclPermissions(info.Permissions)))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteType, domain.Int64(info.InviteType))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileCid, domain.String(info.InviteFileCid))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileKey, domain.String(info.InviteFileKey))
}

func getInviteDetails(details *domain.Details) (info domain.InviteInfo) {
	info.InviteType = domain.InviteType(details.GetInt64(bundle.RelationKeySpaceInviteType))
	// nolint: gosec
	info.Permissions = domain.ConvertParticipantPermissions(model.ParticipantPermissions(details.GetInt64(bundle.RelationKeySpaceInvitePermissions)))
	info.InviteFileCid = details.GetString(bundle.RelationKeySpaceInviteFileCid)
	info.InviteFileKey = details.GetString(bundle.RelationKeySpaceInviteFileKey)
	if info.InviteType == domain.InviteTypeDefault {
		info.Permissions = list.AclPermissionsNone
	}
	return
}

func removeInviteDetails(st *state.State) {
	st.RemoveDetail(inviteDetailKeys...)
}
