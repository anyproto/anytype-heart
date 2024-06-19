package spaceinfo

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

type ParticipantAclInfo struct {
	Id          string
	SpaceId     string
	Identity    string
	Permissions model.ParticipantPermissions
	Status      model.ParticipantStatus
}
