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

func ParseParticipantId(participantId string) (string, string, error) {
	if !strings.HasPrefix(participantId, ParticipantPrefix) {
		return "", "", fmt.Errorf("participant id must start with _participant_")
	}
	parts := strings.Split(participantId, "_")
	if len(parts) != 5 {
		return "", "", fmt.Errorf("can't extract space id")
	}

	return fmt.Sprintf("%s.%s", parts[2], parts[3]), parts[4], nil
}
