package domain

import (
	"fmt"
)

type FullID struct {
	ObjectID string
	SpaceID  string
}

type ObjectTypeKey string

const ParticipantPrefix = "_participant_"

func NewParticipantId(spaceId, identity string) string {
	return fmt.Sprintf("%s%s_%s", ParticipantPrefix, spaceId, identity)
}
