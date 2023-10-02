package spaceinfo

type LocalStatus int

const (
	LocalStatusUnknown LocalStatus = iota
	LocalStatusLoading
	LocalStatusOk
	LocalStatusMissing
)

type RemoteStatus int

const (
	RemoteStatusUnknown RemoteStatus = iota
	RemoteStatusOk
	RemoteStatusWaitingDeletion
	RemoteStatusDeleted
	RemoteStatusError
)

type SpaceInfo struct {
	SpaceID      string
	ViewID       string
	LocalStatus  LocalStatus
	RemoteStatus RemoteStatus
}
