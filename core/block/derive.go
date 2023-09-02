package block

import (
	"context"
	"errors"
	"fmt"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"go.uber.org/zap"
)

// DeriveTreeCreatePayload creates payload for the tree of derived object.
// Method should be called before DeriveObject to prepare payload
func (s *Service) DeriveTreeCreatePayload(
	ctx context.Context,
	spaceID string,
	key domain.UniqueKey,
) (treestorage.TreeStorageCreatePayload, error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	changePayload, err := createChangePayload(key.SmartblockType(), key)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	treePayload := derivePayload(space.Id(), s.commonAccount.Account().SignKey, changePayload)
	create, err := space.TreeBuilder().CreateTree(context.Background(), treePayload)
	return create, err
}

// DeriveObject derives the object with id specified in the payload and triggers cache.Get
// DeriveTreeCreatePayload should be called first to prepare the payload and derive the tree
func (s *Service) DeriveObject(
	ctx context.Context, spaceID string, payload treestorage.TreeStorageCreatePayload, newAccount bool,
) (err error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	_, err = s.getDerivedObject(ctx, space, &payload, newAccount, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{Ctx: ctx, SpaceID: spaceID, State: state.NewDoc(id, nil).(*state.State)}
	})
	if err != nil {
		log.With(zap.Error(err)).Debug("derived object with error")
		return
	}
	return nil
}

func (s *Service) DeriveTreeObjectWithUniqueKey(ctx context.Context, spaceID string, key domain.UniqueKey, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	payload, err := s.DeriveTreeCreatePayload(ctx, spaceID, key)
	if err != nil {
		return nil, err
	}

	tr, err := space.TreeBuilder().PutTree(ctx, payload, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	if tr != nil {
		tr.Close()
	}
	id := domain.FullID{
		SpaceID:  spaceID,
		ObjectID: payload.RootRawChange.Id,
	}
	return s.cacheCreatedObject(ctx, id, initFunc)
}

func (s *Service) getDerivedObject(
	ctx context.Context, space commonspace.Space, payload *treestorage.TreeStorageCreatePayload, newAccount bool, initFunc InitFunc,
) (sb smartblock.SmartBlock, err error) {
	id := domain.FullID{
		SpaceID:  space.Id(),
		ObjectID: payload.RootRawChange.Id,
	}
	if newAccount {
		var tr objecttree.ObjectTree
		tr, err = space.TreeBuilder().PutTree(ctx, *payload, nil)
		s.predefinedObjectWasMissing = true
		if err != nil {
			if !errors.Is(err, treestorage.ErrTreeExists) {
				err = fmt.Errorf("failed to put tree: %w", err)
				return
			}
			s.predefinedObjectWasMissing = false
			// the object exists locally
			return s.GetObjectWithTimeout(ctx, id)
		}
		tr.Close()
		return s.cacheCreatedObject(ctx, id, initFunc)
	}

	// timing out when getting objects from remote
	// here we set very long timeout, because we must load these documents
	ctx, cancel := context.WithTimeout(ctx, derivedObjectLoadTimeout)
	ctx = context.WithValue(ctx,
		optsKey,
		cacheOpts{
			buildOption: source.BuildOptions{
				// TODO: revive p2p (right now we are not ready to load from local clients due to the fact that we need to know when local peers connect)
			},
		},
	)
	defer cancel()

	sb, err = s.GetObjectWithTimeout(ctx, id)
	if err != nil {
		if errors.Is(err, treechangeproto.ErrGetTree) {
			err = spacesyncproto.ErrSpaceMissing
		}
		err = fmt.Errorf("failed to get object from node: %w", err)
		return
	}
	return
}
