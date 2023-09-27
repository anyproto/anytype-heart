package objectcache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// TreeDerivationParams is a struct for deriving a tree
type TreeDerivationParams struct {
	Key           domain.UniqueKey
	InitFunc      InitFunc
	TargetSpaceID string
}

// TreeCreationParams is a struct for creating a tree
type TreeCreationParams struct {
	Time           time.Time
	SmartblockType coresb.SmartBlockType
	InitFunc       InitFunc
	TargetSpaceID  string
}

// CreateTreePayload creates a tree payload for a given space and smart block type
func (c *objectCache) CreateTreePayload(ctx context.Context, spaceID string, params payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
	space, err := c.spaceService.Get(ctx, spaceID)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	changePayload, err := createChangePayload(params.SmartblockType, nil, params.TargetSpaceID)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	treePayload, err := createPayload(space.Id(), c.accountService.Account().SignKey, changePayload, params.Time.Unix())
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return space.TreeBuilder().CreateTree(ctx, treePayload)
}

// CreateTreeObject creates a tree object
func (c *objectCache) CreateTreeObject(ctx context.Context, spaceID string, params TreeCreationParams) (sb smartblock.SmartBlock, err error) {
	payload, err := c.CreateTreePayload(ctx, spaceID, payloadcreator.PayloadCreationParams{
		Time:           params.Time,
		SmartblockType: params.SmartblockType,
		TargetSpaceID:  params.TargetSpaceID,
	})
	if err != nil {
		return nil, err
	}
	return c.CreateTreeObjectWithPayload(ctx, spaceID, payload, params.InitFunc)
}

// CreateTreeObjectWithPayload creates a tree object with a given payload and object init func
func (c *objectCache) CreateTreeObjectWithPayload(ctx context.Context, spaceID string, payload treestorage.TreeStorageCreatePayload, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	space, err := c.spaceService.Get(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	id := domain.FullID{
		SpaceID:  spaceID,
		ObjectID: payload.RootRawChange.Id,
	}
	tr, err := space.TreeBuilder().PutTree(ctx, payload, nil)
	if errors.Is(err, treestorage.ErrTreeExists) {
		return c.ResolveObject(ctx, payload.RootRawChange.Id)
	}
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	if tr != nil {
		tr.Close()
	}
	ctx = ContextWithCreateOption(ctx, initFunc)
	return c.GetObject(ctx, id)
}

// DeriveTreePayload derives a tree payload for a given space and smart block type
// it takes into account whether it is for personal space and if so uses old derivation logic
// to maintain backward compatibility
func (c *objectCache) DeriveTreePayload(ctx context.Context, spaceID string, params payloadcreator.PayloadDerivationParams) (storagePayload treestorage.TreeStorageCreatePayload, err error) {
	space, err := c.spaceService.Get(ctx, spaceID)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	changePayload, err := createChangePayload(params.Key.SmartblockType(), params.Key, params.TargetSpaceID)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	accountKeys := c.accountService.Account()
	// we have to derive ids differently for personal space
	if c.provider.PersonalSpaceID() == spaceID {
		treePayload := derivePersonalPayload(space.Id(), accountKeys.SignKey, changePayload)
		create, err := space.TreeBuilder().CreateTree(context.Background(), treePayload)
		if err != nil {
			return storagePayload, err
		}
		return create, err
	}
	treePayload := derivePayload(space.Id(), changePayload)
	create, err := space.TreeBuilder().DeriveTree(context.Background(), treePayload)
	if err != nil {
		return storagePayload, err
	}
	return create, err
}

// DeriveTreeObject derives a tree object for a given space and smart block type
func (c *objectCache) DeriveTreeObject(ctx context.Context, spaceID string, params TreeDerivationParams) (sb smartblock.SmartBlock, err error) {
	payload, err := c.DeriveTreePayload(ctx, spaceID, payloadcreator.PayloadDerivationParams{
		Key:           params.Key,
		TargetSpaceID: params.TargetSpaceID,
	})
	if err != nil {
		return nil, err
	}
	// TODO: [MR] rewrite to use any-sync derivation
	return c.CreateTreeObjectWithPayload(ctx, spaceID, payload, params.InitFunc)
}

func (c *objectCache) DeriveObjectID(ctx context.Context, spaceID string, uniqueKey domain.UniqueKey) (id string, err error) {
	payload, err := c.DeriveTreePayload(ctx, spaceID, payloadcreator.PayloadDerivationParams{
		Key:           uniqueKey,
		TargetSpaceID: spaceID,
	})
	if err != nil {
		return "", err
	}
	return payload.RootRawChange.Id, nil
}
