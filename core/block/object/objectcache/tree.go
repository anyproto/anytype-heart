package objectcache

import (
	"context"
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
	Key      domain.UniqueKey
	InitFunc InitFunc
}

// TreeCreationParams is a struct for creating a tree
type TreeCreationParams struct {
	Time           time.Time
	SmartblockType coresb.SmartBlockType
	InitFunc       InitFunc
}

// CreateTreePayload creates a tree payload for a given space and smart block type
func (c *objectCache) CreateTreePayload(ctx context.Context, params payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
	changePayload, err := createChangePayload(params.SmartblockType, nil)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	treePayload, err := createPayload(c.space.Id(), c.accountService.Account().SignKey, changePayload, params.Time.Unix())
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return c.space.TreeBuilder().CreateTree(ctx, treePayload)
}

// CreateTreeObject creates a tree object
func (c *objectCache) CreateTreeObject(ctx context.Context, params TreeCreationParams) (sb smartblock.SmartBlock, err error) {
	payload, err := c.CreateTreePayload(ctx, payloadcreator.PayloadCreationParams{
		Time:           params.Time,
		SmartblockType: params.SmartblockType,
	})
	if err != nil {
		return nil, err
	}
	return c.CreateTreeObjectWithPayload(ctx, payload, params.InitFunc)
}

// CreateTreeObjectWithPayload creates a tree object with a given payload and object init func
func (c *objectCache) CreateTreeObjectWithPayload(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	tr, err := c.space.TreeBuilder().PutTree(ctx, payload, nil)
	if err != nil {
		fmt.Printf("-- c.space: %s\n", c.space.Id())
		return nil, fmt.Errorf("put tree: %w", err)
	}
	if tr != nil {
		tr.Close()
	}
	ctx = ContextWithCreateOption(ctx, initFunc)
	objectId := payload.RootRawChange.Id
	return c.GetObject(ctx, objectId)
}

// DeriveTreePayload derives a tree payload for a given space and smart block type
// it takes into account whether it is for personal space and if so uses old derivation logic
// to maintain backward compatibility
func (c *objectCache) DeriveTreePayload(ctx context.Context, params payloadcreator.PayloadDerivationParams) (storagePayload treestorage.TreeStorageCreatePayload, err error) {
	changePayload, err := createChangePayload(params.Key.SmartblockType(), params.Key)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	accountKeys := c.accountService.Account()
	if c.space.TreeBuilder() == nil {
		return treestorage.TreeStorageCreatePayload{}, fmt.Errorf("can't derive in virtual space")
	}
	// we have to derive ids differently for personal space
	if c.personalSpaceId == c.space.Id() || params.UseAccountSignature {
		treePayload := derivePersonalPayload(c.space.Id(), accountKeys.SignKey, changePayload)
		create, err := c.space.TreeBuilder().CreateTree(context.Background(), treePayload)
		if err != nil {
			return storagePayload, err
		}
		return create, err
	}
	treePayload := derivePayload(c.space.Id(), changePayload)
	create, err := c.space.TreeBuilder().DeriveTree(context.Background(), treePayload)
	if err != nil {
		return storagePayload, err
	}
	return create, err
}

// DeriveTreeObject derives a tree object for a given space and smart block type
func (c *objectCache) DeriveTreeObject(ctx context.Context, params TreeDerivationParams) (sb smartblock.SmartBlock, err error) {
	payload, err := c.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{
		Key: params.Key,
	})
	if err != nil {
		return nil, err
	}
	// TODO: [MR] rewrite to use any-sync derivation
	return c.CreateTreeObjectWithPayload(ctx, payload, params.InitFunc)
}

// DeriveTreeObjectWithAccountSignature derives a tree object for a given space and smart block type using account signature
func (c *objectCache) DeriveTreeObjectWithAccountSignature(ctx context.Context, params TreeDerivationParams) (sb smartblock.SmartBlock, err error) {
	payload, err := c.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{
		Key:                 params.Key,
		UseAccountSignature: true,
	})
	if err != nil {
		return nil, err
	}
	// TODO: [MR] rewrite to use any-sync derivation
	return c.CreateTreeObjectWithPayload(ctx, payload, params.InitFunc)
}

func (c *objectCache) DeriveObjectID(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error) {
	payload, err := c.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{
		Key: uniqueKey,
	})
	if err != nil {
		return "", err
	}
	return payload.RootRawChange.Id, nil
}

func (c *objectCache) DeriveObjectIdWithAccountSignature(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error) {
	payload, err := c.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{
		Key:                 uniqueKey,
		UseAccountSignature: true,
	})
	if err != nil {
		return "", err
	}
	return payload.RootRawChange.Id, nil
}
