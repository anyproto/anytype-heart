package spaceinfo

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type LocalStatus int

const (
	LocalStatusUnknown = LocalStatus(model.SpaceStatus_Unknown)
	LocalStatusLoading = LocalStatus(model.SpaceStatus_Loading)
	LocalStatusOk      = LocalStatus(model.SpaceStatus_Ok)
	LocalStatusMissing = LocalStatus(model.SpaceStatus_Missing)
)

func (l LocalStatus) String() string {
	switch l {
	case LocalStatusUnknown:
		return "Unknown"
	case LocalStatusLoading:
		return "Loading"
	case LocalStatusOk:
		return "Ok"
	case LocalStatusMissing:
		return "Missing"
	}
	return ""
}

type RemoteStatus int

const (
	RemoteStatusUnknown         = RemoteStatus(model.SpaceStatus_Unknown)
	RemoteStatusOk              = RemoteStatus(model.SpaceStatus_Ok)
	RemoteStatusWaitingDeletion = RemoteStatus(model.SpaceStatus_RemoteWaitingDeletion)
	RemoteStatusDeleted         = RemoteStatus(model.SpaceStatus_RemoteDeleted)
	RemoteStatusError           = RemoteStatus(model.SpaceStatus_Error)
)

func (r RemoteStatus) IsDeleted() bool {
	return r == RemoteStatusDeleted || r == RemoteStatusWaitingDeletion
}

func (r RemoteStatus) String() string {
	switch r {
	case RemoteStatusUnknown:
		return "Unknown"
	case RemoteStatusOk:
		return "Ok"
	case RemoteStatusWaitingDeletion:
		return "WaitingDeletion"
	case RemoteStatusDeleted:
		return "Deleted"
	case RemoteStatusError:
		return "Error"
	}
	return ""
}

type AccountStatus int

const (
	AccountStatusUnknown  = AccountStatus(model.SpaceStatus_Unknown)
	AccountStatusDeleted  = AccountStatus(model.SpaceStatus_SpaceDeleted)
	AccountStatusJoining  = AccountStatus(model.SpaceStatus_SpaceJoining)
	AccountStatusActive   = AccountStatus(model.SpaceStatus_SpaceActive)
	AccountStatusRemoving = AccountStatus(model.SpaceStatus_SpaceRemoving)
)

func (a AccountStatus) String() string {
	switch a {
	case AccountStatusUnknown:
		return "Unknown"
	case AccountStatusDeleted:
		return "Deleted"
	case AccountStatusJoining:
		return "Joining"
	case AccountStatusActive:
		return "Active"
	case AccountStatusRemoving:
		return "Removing"
	}
	return ""
}

type ShareableStatus int

const (
	ShareableStatusUnknown      = ShareableStatus(model.SpaceShareableStatus_StatusUnknown)
	ShareableStatusShareable    = ShareableStatus(model.SpaceShareableStatus_StatusShareable)
	ShareableStatusNotShareable = ShareableStatus(model.SpaceShareableStatus_StatusNotShareable)
)

func (s ShareableStatus) String() string {
	switch s {
	case ShareableStatusUnknown:
		return "Unknown"
	case ShareableStatusShareable:
		return "Shareable"
	case ShareableStatusNotShareable:
		return "NotShareable"
	}
	return ""
}

type SpaceRemoteStatusInfo struct {
	IsOwned   bool
	LocalInfo SpaceLocalInfo
}

type AccessType int

const (
	AccessTypePrivate  = AccessType(model.SpaceAccessType_Private)
	AccessTypePersonal = AccessType(model.SpaceAccessType_Personal)
	AccessTypeShared   = AccessType(model.SpaceAccessType_Shared)
)

type SpaceDescription struct {
	Name             string
	IconImage        string
	SpaceUxType      model.SpaceUxType
	OneToOneIdentity string
}

func (s *SpaceDescription) UpdateDetails(st *state.State) {
	if s == nil {
		return
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeyName, domain.String(s.Name))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceUxType, domain.Int64(s.SpaceUxType))
	st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, domain.String(s.IconImage))
}
