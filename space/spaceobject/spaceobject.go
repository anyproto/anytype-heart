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

type spaceObject struct {
}

func (s *spaceObject) Id() string {
	panic("implement me")
}

func (s *spaceObject) SpaceID() string {
	panic("implement me")
}

func (s *spaceObject) Space() *spacecore.AnySpace {
	panic("implement me")
}

func (s *spaceObject) DerivedIDs() threads.DerivedSmartblockIds {
	panic("implement me")
}

func (s *spaceObject) WaitLoad() error {
	panic("implement me")
}

func NewSpaceObject() SpaceObject {
	return &spaceObject{}
}
