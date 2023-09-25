package objectcache

import (
	"context"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"sync"
)

type resolverStorage interface {
	StoreIDs(spaceID string, objectIDs []string) (err error)
	ResolveSpaceID(objectID string) (spaceID string, err error)
	StoreSpaceID(spaceID, objectID string) (err error)
}

type anySpaceGetter interface {
	Get(ctx context.Context, id string) (*spacecore.AnySpace, error)
}

type resolver struct {
	spaceGetter    anySpaceGetter
	storage        resolverStorage
	resolvedSpaces map[string]struct{}
	sync.Mutex
}

func newResolver(spaceGetter anySpaceGetter, storage resolverStorage) *resolver {
	res := &resolver{
		spaceGetter:    spaceGetter,
		storage:        storage,
		resolvedSpaces: make(map[string]struct{}),
	}
	res.resolvedSpaces["_anytype_marketplace"] = struct{}{}
	return res
}

func (r *resolver) StoreCurrentIDs(ctx context.Context, spaceID string) (err error) {
	r.Lock()
	if _, exists := r.resolvedSpaces[spaceID]; exists {
		r.Unlock()
		return nil
	}
	r.Unlock()
	space, err := r.spaceGetter.Get(ctx, spaceID)
	if err != nil {
		return err
	}
	err = r.storage.StoreIDs(spaceID, space.StoredIds())
	if err != nil {
		return err
	}
	r.Lock()
	defer r.Unlock()
	r.resolvedSpaces[spaceID] = struct{}{}
	return nil
}

func (r *resolver) ResolveSpaceID(objectID string) (string, error) {
	if addr.IsBundledId(objectID) {
		return addr.AnytypeMarketplaceWorkspace, nil
	}
	return r.storage.ResolveSpaceID(objectID)
}

func (r *resolver) StoreSpaceID(spaceID, objectID string) error {
	return r.storage.StoreSpaceID(spaceID, objectID)
}
