package block

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func (s *Service) CreateTreePayload(ctx context.Context, spaceID string, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return s.CreateTreePayloadWithSpaceAndCreatedTime(ctx, space, tp, createdTime)
}

func (s *Service) CreateTreePayloadWithSpace(ctx context.Context, space commonspace.Space, tp coresb.SmartBlockType) (treestorage.TreeStorageCreatePayload, error) {
	return s.CreateTreePayloadWithSpaceAndCreatedTime(ctx, space, tp, time.Now())
}

func (s *Service) CreateTreePayloadWithSpaceAndCreatedTime(ctx context.Context, space commonspace.Space, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error) {
	changePayload, err := createChangePayload(tp, nil)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	treePayload, err := createPayload(space.Id(), s.commonAccount.Account().SignKey, changePayload, createdTime.Unix())
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return space.TreeBuilder().CreateTree(ctx, treePayload)
}

func (s *Service) CreateTreeObjectWithPayload(ctx context.Context, spaceID string, payload treestorage.TreeStorageCreatePayload, initFunc objectcache.InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	id := domain.FullID{
		SpaceID:  spaceID,
		ObjectID: payload.RootRawChange.Id,
	}
	tr, err := space.TreeBuilder().PutTree(ctx, payload, nil)
	if err != nil {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	if tr != nil {
		tr.Close()
	}
	return s.cacheCreatedObject(ctx, id, initFunc)
}

func (s *Service) CreateTreeObject(ctx context.Context, spaceID string, tp coresb.SmartBlockType, initFunc objectcache.InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	payload, err := s.CreateTreePayloadWithSpace(ctx, space, tp)
	if err != nil {
		return nil, err
	}
	return s.CreateTreeObjectWithPayload(ctx, spaceID, payload, initFunc)
}

func (s *Service) cacheCreatedObject(ctx context.Context, id domain.FullID, initFunc objectcache.InitFunc) (sb smartblock.SmartBlock, err error) {
	ctx = objectcache.ContextWithCreateOption(ctx, initFunc)
	return s.objectCache.GetObject(ctx, id)
}
