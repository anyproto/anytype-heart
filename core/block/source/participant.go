package source

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

func NewParticipantId(spaceId, identity string) string {
	return fmt.Sprintf("%s%s_%s", addr.ParticipantPrefix, spaceId, identity)
}
