package spaceobject

import (
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

type SpaceObject interface {
	Id() string
	SpaceID() string
	Space() *spacecore.AnySpace
	DerivedIDs() threads.DerivedSmartblockIds
	WaitLoad() error
}
