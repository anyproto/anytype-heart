package space

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/techspace"
)

type techSpace struct {
	*space
	techspace.TechSpace
}

func (s *techSpace) Close(ctx context.Context) error {
	err := s.space.Close(ctx)
	if err != nil {
		log.Error("close tech space", zap.Error(err))
	}
	err = s.TechSpace.Close(ctx)
	if err != nil {
		log.Error("close tech space", zap.Error(err))
	}
	return nil
}

func (s *service) initTechSpace() error {
	s.techSpace = techspace.New()
	techCoreSpace, err := s.spaceCore.Derive(context.Background(), spacecore.TechSpaceType)
	if err != nil {
		return fmt.Errorf("derive tech space: %w", err)
	}
	sp := &techSpace{
		space: &space{
			service:                s,
			Space:                  techCoreSpace,
			loadMandatoryObjectsCh: make(chan struct{}),
			installer:              s.bundledObjectsInstaller,
		},
		TechSpace: s.techSpace,
	}
	sp.Cache = objectcache.New(s.accountService, s.objectFactory, s.personalSpaceID, sp)

	err = s.techSpace.Run(techCoreSpace, sp.Cache)

	s.preLoad(sp)
	if err != nil {
		return fmt.Errorf("run tech space: %w", err)
	}
	return nil
}
