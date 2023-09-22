package space

import (
	"context"

	"github.com/anyproto/any-sync/net/streampool"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceobject"
	"github.com/anyproto/anytype-heart/space/spaceobject/objectprovider"
)

type objectDeriver interface {
	deriveSpaceObject(ctx context.Context, spaceID, targetSpaceID string) (spaceobject.SpaceObject, error)
	deriveSpaceObjectId(ctx context.Context, spaceID, targetSpaceID string) (string, error)
}

type spaceLoader struct {
	spaceCore spacecore.SpaceCoreService
	deriver   objectDeriver
	provider  objectprovider.ObjectProvider
	cache     objectcache.Cache
	techSpace *spacecore.AnySpace

	execPool *streampool.ExecPool
	ctx      context.Context
	cancel   context.CancelFunc
}

func (s *spaceLoader) CreateSpaces(ctx context.Context) (err error) {
	if err != nil {
		return
	}
	_, err = s.derivePersonalSpace(ctx)
	return
}

func (s *spaceLoader) LoadSpaces(ctx context.Context) (err error) {
	_, err = s.loadPersonalSpace(ctx)
	if err != nil {
		return
	}
	storedIDs := s.techSpace.StoredIds()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.execPool = streampool.NewExecPool(10, len(storedIDs))
	for _, id := range storedIDs {
		_ = s.execPool.Add(s.ctx, func() {
			err := s.loadSpaceObject(id)
			if err != nil {
				log.Debug("failed to load space object", zap.Error(err), zap.String("id", id))
			}
		})
	}
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

func (s *spaceLoader) derivePersonalSpace(ctx context.Context) (spaceObject spaceobject.SpaceObject, err error) {
	space, err := s.spaceCore.Derive(ctx, spacecore.SpaceType)
	if err != nil {
		return nil, err
	}
	sp, err := s.spaceCore.Get(ctx, space.Id())
	if err != nil {
		return nil, err
	}
	obj, err := s.deriver.deriveSpaceObject(ctx, s.techSpace.Id(), sp.Id())
	if err != nil {
		return nil, err
	}
	return obj, obj.WaitLoad()
}

func (s *spaceLoader) loadPersonalSpace(ctx context.Context) (spaceObject spaceobject.SpaceObject, err error) {
	id, err := s.spaceCore.DeriveID(ctx, spacecore.SpaceType)
	if err != nil {
		return nil, err
	}
	sp, err := s.spaceCore.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	ids, err := s.provider.DeriveObjectIDs(ctx, id, personalSpaceTypes)
	if err != nil {
		return nil, err
	}
	err = s.loadObjects(ctx, id, ids.IDs())
	if err != nil {
		return nil, err
	}
	obj, err := s.getOrDerive(ctx, s.techSpace.Id(), sp.Id())
	if err != nil {
		return nil, err
	}
	return obj, obj.WaitLoad()
}

func (s *spaceLoader) getOrDerive(ctx context.Context, spaceID, targetSpaceID string) (spaceobject.SpaceObject, error) {
	id, err := s.deriver.deriveSpaceObjectId(ctx, spaceID, targetSpaceID)
	if err != nil {
		return nil, err
	}
	obj, err := s.cache.GetObject(ctx, domain.FullID{
		ObjectID: id,
		SpaceID:  spaceID,
	})
	if err != nil {
		return s.deriver.deriveSpaceObject(ctx, spaceID, targetSpaceID)
	}
	return obj.(spaceobject.SpaceObject), nil
}

func (s *spaceLoader) loadObjects(ctx context.Context, spaceID string, objIDs []string) (err error) {
	for _, id := range objIDs {
		_, err = s.cache.GetObject(ctx, domain.FullID{
			ObjectID: id,
			SpaceID:  spaceID,
		})
		if err != nil {
			return err
		}
	}
	return
}

func (s *spaceLoader) loadSpaceObject(id string) (err error) {
	obj, err := s.cache.GetObject(s.ctx, domain.FullID{ObjectID: id, SpaceID: s.techSpace.Id()})
	if err != nil {
		return
	}
	spaceObject := obj.(spaceobject.SpaceObject)
	return spaceObject.WaitLoad()
}
