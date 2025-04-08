package domain

type InviteView struct {
	SpaceId      string
	SpaceName    string
	SpaceIconCid string
	CreatorName  string
	AclKey       []byte
	GuestKey     []byte
}

func (i InviteView) IsGuestUserInvite() bool {
	if len(i.GuestKey) > 0 {
		return true
	}
	return false
}

type InviteInfo struct {
	InviteFileCid string
	InviteFileKey string
}

type InviteObject interface {
	SetInviteFileInfo(fileCid string, fileKey string) (err error)
	GetExistingInviteInfo() (fileCid string, fileKey string)
	RemoveExistingInviteInfo() (fileCid string, err error)

	SetGuestInviteFileInfo(fileCid string, fileKey string) (err error)
	GetExistingGuestInviteInfo() (fileCid string, fileKey string)
}
