package spacecore

import (
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

type SpaceObject interface {
	SpaceID() string
	DerivedIDs() threads.DerivedSmartblockIds
}
