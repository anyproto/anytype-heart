package badgerstorage

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/dgraph-io/badger/v4"
	"golang.org/x/exp/slices"
)

type spaceStorage struct {
	spaceId         string
	spaceSettingsId string
	objDb           *badger.DB
	keys            spaceKeys
	aclStorage      oldstorage.ListStorage
	header          *spacesyncproto.RawSpaceHeaderWithId
	service         *storageService
}

func (s *spaceStorage) Run(_ context.Context) (err error) {
	return nil
}

func (s *spaceStorage) Init(_ *app.App) (err error) {
	return nil
}

func (s *spaceStorage) Name() (name string) {
	return spacestorage.CName
}

func newSpaceStorage(objDb *badger.DB, spaceId string, service *storageService) (store oldstorage.SpaceStorage, err error) {
	keys := newSpaceKeys(spaceId)
	err = objDb.View(func(txn *badger.Txn) error {
		header, err := getTxn(txn, keys.HeaderKey())
		if err != nil {
			return err
		}

		aclStorage, err := newListStorage(spaceId, objDb, txn)
		if err != nil {
			return err
		}

		spaceSettingsId, err := getTxn(txn, keys.SpaceSettingsId())
		if err != nil {
			return err
		}
		store = &spaceStorage{
			spaceId:         spaceId,
			spaceSettingsId: string(spaceSettingsId),
			objDb:           objDb,
			keys:            keys,
			service:         service,
			header: &spacesyncproto.RawSpaceHeaderWithId{
				RawHeader: header,
				Id:        spaceId,
			},
			aclStorage: aclStorage,
		}
		return nil
	})
	if err == badger.ErrKeyNotFound {
		err = spacestorage.ErrSpaceStorageMissing
	}
	return
}

func createSpaceStorage(db *badger.DB, payload spacestorage.SpaceStorageCreatePayload, service *storageService) (store oldstorage.SpaceStorage, err error) {
	keys := newSpaceKeys(payload.SpaceHeaderWithId.Id)
	if hasDB(db, keys.HeaderKey()) {
		err = spacestorage.ErrSpaceStorageExists
		return
	}

	spaceStore := &spaceStorage{
		spaceId:         payload.SpaceHeaderWithId.Id,
		objDb:           db,
		keys:            keys,
		service:         service,
		spaceSettingsId: payload.SpaceSettingsWithId.Id,
		header:          payload.SpaceHeaderWithId,
	}
	_, err = forceCreateTreeStorage(spaceStore.objDb, spaceStore.spaceId, treestorage.TreeStorageCreatePayload{
		RootRawChange: payload.SpaceSettingsWithId,
		Changes:       []*treechangeproto.RawTreeChangeWithId{payload.SpaceSettingsWithId},
		Heads:         []string{payload.SpaceSettingsWithId.Id},
	})
	if err != nil {
		return
	}
	err = db.Update(func(txn *badger.Txn) error {
		err = txn.Set(keys.SpaceSettingsId(), []byte(payload.SpaceSettingsWithId.Id))
		if err != nil {
			return err
		}
		aclStorage, err := createListStorage(payload.SpaceHeaderWithId.Id, db, txn, payload.AclWithId)
		if err != nil {
			return err
		}

		err = txn.Set(keys.HeaderKey(), payload.SpaceHeaderWithId.RawHeader)
		if err != nil {
			return err
		}

		spaceStore.aclStorage = aclStorage
		return nil
	})
	store = spaceStore
	return
}

func (s *spaceStorage) Id() string {
	return s.spaceId
}

func (s *spaceStorage) SpaceSettingsId() string {
	return s.spaceSettingsId
}

func (s *spaceStorage) HasTree(id string) (bool, error) {
	keys := newTreeKeys(s.spaceId, id)
	return hasDB(s.objDb, keys.RootIdKey()), nil
}

func (s *spaceStorage) TreeStorage(id string) (oldstorage.TreeStorage, error) {
	return newTreeStorage(s.objDb, s.spaceId, id)
}

func (s *spaceStorage) CreateTreeStorage(payload treestorage.TreeStorageCreatePayload) (ts oldstorage.TreeStorage, err error) {
	return createTreeStorage(s.objDb, s.spaceId, payload)
}

func (s *spaceStorage) AclStorage() (oldstorage.ListStorage, error) {
	return s.aclStorage, nil
}

func (s *spaceStorage) SpaceHeader() (header *spacesyncproto.RawSpaceHeaderWithId, err error) {
	return s.header, nil
}

func (s *spaceStorage) WriteSpaceHash(hash string) error {
	return s.objDb.Update(func(txn *badger.Txn) error {
		return txn.Set(s.keys.SpaceHash(), []byte(hash))
	})
}

func (s *spaceStorage) ReadSpaceHash() (hash string, err error) {
	err = s.objDb.View(func(txn *badger.Txn) error {
		res, err := getTxn(txn, s.keys.SpaceHash())
		if err != nil {
			return err
		}
		hash = string(res)
		return nil
	})
	return
}

func (s *spaceStorage) AllDeletedTreeIds() (ids []string, err error) {
	err = s.objDb.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = s.keys.TreeDeletedPrefix()

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			id := make([]byte, 0, len(item.Key()))
			id = item.KeyCopy(id)
			if len(id) <= len(s.keys.TreeDeletedPrefix())+1 {
				continue
			}

			var isDeleted bool
			err = item.Value(func(val []byte) error {
				if bytes.Equal(val, []byte(oldstorage.TreeDeletedStatusDeleted)) {
					isDeleted = true
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("read value: %w", err)
			}

			if isDeleted {
				id = id[len(s.keys.TreeDeletedPrefix())+1:]
				ids = append(ids, string(id))
			}
		}
		return nil
	})
	return
}

func (s *spaceStorage) StoredIds() (ids []string, err error) {
	err = s.objDb.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = s.keys.TreeRootPrefix()

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			id := make([]byte, 0, len(item.Key()))
			id = item.KeyCopy(id)
			if len(id) <= len(s.keys.TreeRootPrefix())+1 {
				continue
			}
			id = id[len(s.keys.TreeRootPrefix())+1:]
			ids = append(ids, string(id))
		}
		return nil
	})
	return
}

func (s *spaceStorage) TreeRoot(id string) (root *treechangeproto.RawTreeChangeWithId, err error) {
	keys := newTreeKeys(s.spaceId, id)
	err = s.objDb.View(func(txn *badger.Txn) error {
		bytes, err := getTxn(txn, keys.RawChangeKey(id))
		if err != nil {
			return err
		}
		root = &treechangeproto.RawTreeChangeWithId{
			RawChange: bytes,
			Id:        id,
		}
		return nil
	})
	return
}

func (s *spaceStorage) SetTreeDeletedStatus(id, status string) (err error) {
	return s.objDb.Update(func(txn *badger.Txn) error {
		return txn.Set(s.keys.TreeDeletedKey(id), []byte(status))
	})
}

func (s *spaceStorage) SetSpaceDeleted() error {
	return s.objDb.Update(func(txn *badger.Txn) error {
		return txn.Set(s.keys.SpaceDeletedKey(), s.keys.SpaceDeletedKey())
	})
}

func (s *spaceStorage) IsSpaceDeleted() (res bool, err error) {
	err = s.objDb.View(func(txn *badger.Txn) error {
		_, err = getTxn(txn, s.keys.SpaceDeletedKey())
		return err
	})
	if err != badger.ErrKeyNotFound {
		return false, err
	}
	return err == nil, nil
}

func (s *spaceStorage) TreeDeletedStatus(id string) (status string, err error) {
	err = s.objDb.View(func(txn *badger.Txn) error {
		res, err := getTxn(txn, s.keys.TreeDeletedKey(id))
		if err != nil {
			return err
		}
		status = string(res)
		return nil
	})
	if err == badger.ErrKeyNotFound {
		err = nil
	}
	return
}

func (s *spaceStorage) Close(_ context.Context) (err error) {
	s.service.unlockSpaceStorage(s.spaceId)
	return nil
}

func deleteSpace(spaceId string, db *badger.DB) (err error) {
	keys := newSpaceKeys(spaceId)
	var toBeDeleted [][]byte
	if db.IsClosed() {
		return badger.ErrDBClosed
	}
	txn := db.NewTransaction(true)
	defer txn.Discard()

	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	opts.Prefix = keys.TreePrefix()

	it := txn.NewIterator(opts)
	for it.Rewind(); it.Valid(); it.Next() {
		key := slices.Clone(it.Item().Key())
		toBeDeleted = append(toBeDeleted, key)
	}
	it.Close()
	toBeDeleted = append(toBeDeleted, keys.HeaderKey())
	toBeDeleted = append(toBeDeleted, keys.SpaceHash())
	for _, key := range toBeDeleted {
		err := txn.Delete(key)
		if errors.Is(err, badger.ErrTxnTooBig) {
			err = txn.Commit()
			if err != nil {
				return fmt.Errorf("commit big transaction: %w", err)
			}
			txn = db.NewTransaction(true)
			err = txn.Delete(key)
		}
		if err != nil {
			return fmt.Errorf("delete key %s: %w", key, err)
		}
	}
	return txn.Commit()
}
