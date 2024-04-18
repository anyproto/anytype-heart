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
