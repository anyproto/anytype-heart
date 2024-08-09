package domain

import (
	"fmt"
	"strings"
)

type FullID struct {
	ObjectID string
	SpaceID  string
}

const ParticipantPrefix = "_participant_"

func NewParticipantId(spaceId, identity string) string {
	// Replace dots with underscores to avoid issues on Desktop client
	spaceId = strings.Replace(spaceId, ".", "_", 1)
	return fmt.Sprintf("%s%s_%s", ParticipantPrefix, spaceId, identity)
}
