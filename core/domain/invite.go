package domain

import (
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type InviteView struct {
	SpaceId         string
	SpaceName       string
	SpaceIconCid    string
	SpaceIconOption int
	SpaceUxType     model.SpaceUxType
	SpaceType       model.SpaceType
	CreatorName     string
	CreatorIconCid  string
	AclKey          []byte
	GuestKey        []byte
	InviteType      InviteType
}

func (i InviteView) IsGuestUserInvite() bool {
	if len(i.GuestKey) > 0 {
		return true
	}
	return false
}

type InviteType int

const (
	InviteTypeDefault InviteType = iota
	InviteTypeGuest
	InviteTypeAnyone
)

type InviteInfo struct {
	InviteFileCid string
	InviteFileKey string
	InviteType    InviteType
	Permissions   list.AclPermissions
	// HeldByOwner is set when the invite is kept in the owner's account instead of the space, so that
	// only the owner can share it. Members see it without the cid and the key: all they can do is ask
	// the owner for the link.
	HeldByOwner bool
}

// InviteInfoObject is an object that can hold the current invite of a space: the workspace when the
// invite is shared within the space, the owner's space view when it is held by the owner.
type InviteInfoObject interface {
	SetInviteFileInfo(inviteInfo InviteInfo) (err error)
	GetExistingInviteInfo() InviteInfo
	RemoveExistingInviteInfo() (InviteInfo, error)
}

// ShareableWithinSpace reports whether an invite may be stored in the workspace, where every member
// of the space reads it and can hand the link out.
//
// An invite anyone can join with needs no approval from anyone: whoever holds the link is in the
// space, with the permissions the invite carries. Read access is as much as a member is trusted to
// give away; a link that grants more than that stays in the owner's account.
func ShareableWithinSpace(inviteType InviteType, permissions list.AclPermissions) bool {
	if inviteType != InviteTypeAnyone {
		return true
	}
	return permissions == list.AclPermissionsReader
}

type InviteObject interface {
	InviteInfoObject

	SetGuestInviteFileInfo(fileCid string, fileKey string) (err error)
	GetExistingGuestInviteInfo() (fileCid string, fileKey string)
}

func ConvertParticipantPermissions(permissions model.ParticipantPermissions) list.AclPermissions {
	switch permissions {
	case model.ParticipantPermissions_Writer:
		return list.AclPermissionsWriter
	case model.ParticipantPermissions_Reader:
		return list.AclPermissionsReader
	case model.ParticipantPermissions_Owner:
		return list.AclPermissionsOwner
	case model.ParticipantPermissions_Admin:
		return list.AclPermissionsAdmin
	default:
		return list.AclPermissionsNone
	}
}

func ConvertAclPermissions(permissions list.AclPermissions) model.ParticipantPermissions {
	switch aclrecordproto.AclUserPermissions(permissions) {
	case aclrecordproto.AclUserPermissions_Writer:
		return model.ParticipantPermissions_Writer
	case aclrecordproto.AclUserPermissions_Reader:
		return model.ParticipantPermissions_Reader
	case aclrecordproto.AclUserPermissions_Owner:
		return model.ParticipantPermissions_Owner
	case aclrecordproto.AclUserPermissions_Admin:
		return model.ParticipantPermissions_Admin
	default:
		return model.ParticipantPermissions_NoPermissions
	}
}

func ConvertAclStatus(status list.AclStatus) model.ParticipantStatus {
	switch status {
	case list.StatusActive:
		return model.ParticipantStatus_Active
	case list.StatusCanceled:
		return model.ParticipantStatus_Canceled
	case list.StatusRemoving:
		return model.ParticipantStatus_Removing
	case list.StatusRemoved:
		return model.ParticipantStatus_Removed
	case list.StatusDeclined:
		return model.ParticipantStatus_Declined
	default:
		return model.ParticipantStatus_Joining
	}
}

func ConvertInviteType(inviteType InviteType) aclrecordproto.AclInviteType {
	switch inviteType {
	case InviteTypeDefault:
		return aclrecordproto.AclInviteType_RequestToJoin
	default:
		return aclrecordproto.AclInviteType_AnyoneCanJoin
	}
}
