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

type SpaceInfo struct {
	SpaceID      string
	LocalStatus  LocalStatus
	RemoteStatus RemoteStatus
}
