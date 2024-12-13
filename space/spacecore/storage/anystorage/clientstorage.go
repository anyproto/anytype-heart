package anystorage

import (
	"context"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
)

// TODO: add mark space created etc
//  add tree root

type clientStorage struct {
	spacestorage.SpaceStorage
	cont *storageContainer
}

func (r *clientStorage) Close(ctx context.Context) (err error) {
	defer r.cont.Release()
	return r.SpaceStorage.Close(ctx)
}
