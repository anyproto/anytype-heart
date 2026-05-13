package domain

import (
	"errors"
	"fmt"
	"strings"
)

type FullID struct {
	ObjectID string
	SpaceID  string
}

func (i FullID) IsEmpty() bool {
	return i.ObjectID == ""
}

const ParticipantPrefix = "_participant_"

const PersonalWidgetsPrefix = "_personalWidgets_"

// NewPersonalWidgetsId returns the id of the personal widgets virtual widget
// for the given space. The single dot in the spaceId is replaced with an
// underscore (Desktop client constraint, same encoding as participant ids);
// the encoding round-trips only for spaceIds with exactly one dot and no
// underscores in either half.
func NewPersonalWidgetsId(spaceId string) string {
	spaceId = strings.Replace(spaceId, ".", "_", 1)
	return PersonalWidgetsPrefix + spaceId
}

func ParsePersonalWidgetsId(id string) (spaceId string, err error) {
	if !strings.HasPrefix(id, PersonalWidgetsPrefix) {
		return "", fmt.Errorf("personal widgets id must start with %s", PersonalWidgetsPrefix)
	}
	rest := strings.TrimPrefix(id, PersonalWidgetsPrefix)
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("can't extract space id from %s", id)
	}
	return parts[0] + "." + parts[1], nil
}

var ErrParseLongId = errors.New("failed to parse object id")

func NewParticipantId(spaceId, identity string) string {
	// Replace dots with underscores to avoid issues on Desktop client
	spaceId = strings.Replace(spaceId, ".", "_", 1)
	return fmt.Sprintf("%s%s_%s", ParticipantPrefix, spaceId, identity)
}

func ParseParticipantId(participantId string) (spaceId string, identity string, err error) {
	if !strings.HasPrefix(participantId, ParticipantPrefix) {
		return "", "", fmt.Errorf("participant id must start with _participant_")
	}
	parts := strings.Split(participantId, "_")
	if len(parts) != 5 {
		return "", "", fmt.Errorf("can't extract space id")
	}

	return fmt.Sprintf("%s.%s", parts[2], parts[3]), parts[4], nil
}

func ParseLongId(id string) (FullID, error) {
	if id == "" {
		return FullID{}, ErrParseLongId
	}
	// TODO: support spaceId in long ids
	return FullID{ObjectID: id}, nil
}
