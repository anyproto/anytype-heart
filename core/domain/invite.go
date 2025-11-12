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
}

type InviteObject interface {
	SetInviteFileInfo(inviteInfo InviteInfo) (err error)
	GetExistingInviteInfo() InviteInfo
	RemoveExistingInviteInfo() (InviteInfo, error)

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
