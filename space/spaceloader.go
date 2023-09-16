package space

import (
	"context"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"go.uber.org/zap"
)

type spaceLoader struct {
	spaceService *service
	execPool     *streampool.ExecPool
	ctx          context.Context
	cancel       context.CancelFunc
}

func (s *spaceLoader) prepareIndexes() (err error) {
	err = s.spaceService.indexer.PrepareFlags()
	if err != nil {
		return
	}
	return s.spaceService.indexer.RemoveIndexes()
}

func (s *spaceLoader) initTechSpace(ctx context.Context) (err error) {
	// derive tech space
	payload := commonspace.SpaceDerivePayload{
		SigningKey: s.spaceService.wallet.GetAccountPrivkey(),
		MasterKey:  s.spaceService.wallet.GetMasterKey(),
		SpaceType:  TechSpaceType,
	}
	spaceID, err := s.spaceService.commonSpace.DeriveSpace(ctx, payload)
	if err != nil {
		return
	}
	sp, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return
	}
	s.spaceService.techSpace = newTechSpace(sp.(*clientSpace), s.spaceService)
	return
}

func (s *spaceLoader) LoadSpaces(ctx context.Context) (err error) {
	err = s.prepareIndexes()
	if err != nil {
		return
	}
	err = s.initTechSpace(ctx)
	if err != nil {
		return
	}
	err = s.spaceService.indexer.ReindexBundledObjects()
	if err != nil {
		return
	}
	// derive personal space
	obj, err := s.spaceService.techSpace.DerivePersonalSpace(ctx, false)
	if err != nil {
		return
	}
	s.cacheDerivedIDs(obj)
	// load all spaces asynchronously, so they can reindex themselves
	storedIds := s.spaceService.techSpace.StoredIds()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.execPool = streampool.NewExecPool(10, len(storedIds))
	for _, id := range storedIds {
		_ = s.execPool.Add(s.ctx, func() {
			obj, err := s.spaceService.objectCache.PickBlock(s.ctx, id)
			if err != nil {
				log.Debug("failed to load block", zap.Error(err), zap.String("id", id))
			}
			s.cacheDerivedIDs(obj.(spacecore.SpaceObject))
		})
	}
	return
}

func (s *spaceLoader) cacheDerivedIDs(spaceObject spacecore.SpaceObject) {
	spaceID := spaceObject.SpaceID()
	_, err := s.spaceService.techSpace.SpaceDerivedIDs(s.ctx, spaceID)
	if err != nil {
		log.Debug("failed to get derived ids", zap.Error(err), zap.String("spaceID", spaceID))
	}
	return
}

func (s *spaceLoader) CreateSpaces(ctx context.Context) (err error) {
	err = s.prepareIndexes()
	if err != nil {
		return
	}
	err = s.initTechSpace(ctx)
	if err != nil {
		return
	}
	err = s.spaceService.indexer.ReindexBundledObjects()
	if err != nil {
		return
	}
	// derive personal space
	obj, err := s.spaceService.techSpace.DerivePersonalSpace(ctx, true)
	if err != nil {
		return
	}
	s.cacheDerivedIDs(obj.(spacecore.SpaceObject))
	return
}

func (s *spaceLoader) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.execPool != nil {
		s.execPool.Close()
	}
}
