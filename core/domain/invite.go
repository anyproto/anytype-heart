package domain

type InviteView struct {
	SpaceId      string
	SpaceName    string
	SpaceIconCid string
	CreatorName  string
	InviteKey    []byte
}

type InviteInfo struct {
	InviteFileCid string
	InviteFileKey string
}

type InviteObject interface {
	SetInviteFileInfo(fileCid string, fileKey string) (err error)
	GetExistingInviteInfo() (fileCid string, fileKey string)
	RemoveExistingInviteInfo() (fileCid string, err error)
}
