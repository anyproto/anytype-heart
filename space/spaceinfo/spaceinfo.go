package spaceinfo

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

type LocalStatus int

const (
	LocalStatusUnknown = LocalStatus(model.SpaceStatus_Unknown)
	LocalStatusLoading = LocalStatus(model.SpaceStatus_Loading)
	LocalStatusOk      = LocalStatus(model.SpaceStatus_Ok)
	LocalStatusMissing = LocalStatus(model.SpaceStatus_Missing)
)

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

type AccountStatus int

const (
	AccountStatusUnknown = AccountStatus(model.SpaceStatus_Unknown)
	AccountStatusDeleted = AccountStatus(model.SpaceStatus_SpaceDeleted)
)

type SpaceInfo struct {
	SpaceID       string
	LocalStatus   LocalStatus
	RemoteStatus  RemoteStatus
	AccountStatus AccountStatus
}

func (s SpaceInfo) IsDeleted() bool {
	return s.RemoteStatus.IsDeleted() && s.LocalStatus == LocalStatusMissing && s.AccountStatus == AccountStatusDeleted
}
