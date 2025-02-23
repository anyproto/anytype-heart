package sqliteanystorage

import (
	"context"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/headsync/statestorage"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
)

type spaceStorage struct {
}

func (s spaceStorage) Init(a *app.App) (err error) {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) Name() (name string) {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) Run(ctx context.Context) (err error) {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) Close(ctx context.Context) (err error) {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) Id() string {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) HeadStorage() headstorage.HeadStorage {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) StateStorage() statestorage.StateStorage {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) AclStorage() (list.Storage, error) {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) TreeStorage(ctx context.Context, id string) (objecttree.Storage, error) {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) CreateTreeStorage(ctx context.Context, payload treestorage.TreeStorageCreatePayload) (objecttree.Storage, error) {
	// TODO implement me
	panic("implement me")
}

func (s spaceStorage) AnyStore() anystore.DB {
	// TODO implement me
	panic("implement me")
}
