package anystorage

import (
	"context"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
)

// TODO: [storage] add mark space created etc
//  add tree root

type ClientSpaceStorage interface {
	spacestorage.SpaceStorage
	HasTree(ctx context.Context, id string) (has bool, err error)
	TreeRoot(ctx context.Context, id string) (root *treechangeproto.RawTreeChangeWithId, err error)
	MarkSpaceCreated(ctx context.Context) error
	IsSpaceCreated(ctx context.Context) (created bool, err error)
	UnmarkSpaceCreated(ctx context.Context) error
}

type clientStorage struct {
	spacestorage.SpaceStorage
	cont *storageContainer
}

func (r *clientStorage) Close(ctx context.Context) (err error) {
	defer r.cont.Release()
	return r.SpaceStorage.Close(ctx)
}
