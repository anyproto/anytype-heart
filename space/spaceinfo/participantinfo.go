package spaceinfo

import (
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ParticipantAclInfo struct {
	Id          string
	SpaceId     string
	Identity    string
	Permissions model.ParticipantPermissions
	Status      model.ParticipantStatus
}

type OneToOneParticipantData struct {
	Identity           crypto.PubKey
	RequestMetadataKey crypto.SymKey // a.k.a RequestMetadata, symKey
}
