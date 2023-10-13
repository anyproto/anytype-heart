package space

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

func (s *service) initTechSpace() error {
	techCoreSpace, err := s.spaceCore.Derive(context.Background(), spacecore.TechSpaceType)
	if err != nil {
		return fmt.Errorf("derive tech space: %w", err)
	}

	sp := &space{
		service:                s,
		Space:                  techCoreSpace,
		loadMandatoryObjectsCh: make(chan struct{}),
		installer:              s.bundledObjectsInstaller,
	}
	sp.Cache = objectcache.New(s.accountService, s.objectFactory, s.personalSpaceID, sp)

	err = s.techSpace.Run(techCoreSpace, sp.Cache)

	s.preLoad(sp)
	if err != nil {
		return fmt.Errorf("run tech space: %w", err)
	}
	return nil
}
