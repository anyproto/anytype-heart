package domain

import (
	"fmt"
	"strings"
)

type FullID struct {
	ObjectID string
	SpaceID  string
}

type ObjectTypeKey string

const ParticipantPrefix = "_participant_"

func NewParticipantId(spaceId, identity string) string {
	spaceId = strings.Replace(spaceId, ".", "_", 1)
	return fmt.Sprintf("%s%s_%s", ParticipantPrefix, spaceId, identity)
}
