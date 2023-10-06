package storage

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/dgraph-io/badger/v3"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

type storageService struct {
	keys     storageServiceKeys
	provider datastore.Datastore
	db       *badger.DB
}

type ClientStorage interface {
	spacestorage.SpaceStorageProvider
	app.ComponentRunnable
	AllSpaceIds() (ids []string, err error)
	GetSpaceID(objectID string) (spaceID string, err error)
	BindSpaceID(spaceID, objectID string) (err error)
}

func New() ClientStorage {
	return &storageService{}
}

func (s *storageService) Init(a *app.App) (err error) {
	s.provider = a.MustComponent(datastore.CName).(datastore.Datastore)
	s.keys = newStorageServiceKeys()
	return
}

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) SpaceStorage(id string) (spacestorage.SpaceStorage, error) {
	return newSpaceStorage(s.db, id)
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (spacestorage.SpaceStorage, error) {
	return newSpaceStorage(s.db, id)
}

func (s *storageService) SpaceExists(id string) bool {
	return s.db.View(func(txn *badger.Txn) error {
		_, err := getTxn(txn, newSpaceKeys(id).HeaderKey())
		if err != nil {
			return err
		}
		return nil
	}) == nil
}

func (s *storageService) CreateSpaceStorage(payload spacestorage.SpaceStorageCreatePayload) (spacestorage.SpaceStorage, error) {
	return createSpaceStorage(s.db, payload)
}

func (s *storageService) GetSpaceID(objectID string) (spaceID string, err error) {
	return badgerhelper.GetValue(s.db, s.keys.BindObjectIDKey(objectID), func(bytes []byte) (string, error) {
		return string(bytes), nil
	})
}

func (s *storageService) BindSpaceID(spaceID, objectID string) (err error) {
	return badgerhelper.SetValue(s.db, s.keys.BindObjectIDKey(objectID), []byte(spaceID))
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = s.keys.SpacePrefix()

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			id := item.Key()
			if len(id) <= len(s.keys.SpacePrefix())+1 {
				continue
			}
			id = id[len(s.keys.SpacePrefix())+1:]
			ids = append(ids, string(id))
		}
		return nil
	})
	return
}

func (s *storageService) Run(ctx context.Context) (err error) {
	s.db, err = s.provider.SpaceStorage()
	if err != nil {
		return
	}
	return
}

func (s *storageService) Close(ctx context.Context) (err error) {
	return
}
