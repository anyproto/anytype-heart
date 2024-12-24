package badgerstorage

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/dgraph-io/badger/v4"
)

type treeStorage struct {
	db   *badger.DB
	keys treeKeys
	id   string
	root *treechangeproto.RawTreeChangeWithId
}

func newTreeStorage(db *badger.DB, spaceId, treeId string) (ts oldstorage.TreeStorage, err error) {
	keys := newTreeKeys(spaceId, treeId)
	err = db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(keys.RootIdKey())
		if err != nil {
			return err
		}

		root, err := getTxn(txn, keys.RawChangeKey(treeId))
		if err != nil {
			return err
		}

		rootWithId := &treechangeproto.RawTreeChangeWithId{
			RawChange: root,
			Id:        treeId,
		}

		ts = &treeStorage{
			db:   db,
			keys: keys,
			id:   treeId,
			root: rootWithId,
		}
		return nil
	})
	if err == badger.ErrKeyNotFound {
		err = treestorage.ErrUnknownTreeId
	}
	return
}

func createTreeStorage(db *badger.DB, spaceId string, payload treestorage.TreeStorageCreatePayload) (ts oldstorage.TreeStorage, err error) {
	keys := newTreeKeys(spaceId, payload.RootRawChange.Id)
	if hasDB(db, keys.RootIdKey()) {
		err = treestorage.ErrTreeExists
		return
	}
	return forceCreateTreeStorage(db, spaceId, payload)
}

func forceCreateTreeStorage(db *badger.DB, spaceId string, payload treestorage.TreeStorageCreatePayload) (ts oldstorage.TreeStorage, err error) {
	keys := newTreeKeys(spaceId, payload.RootRawChange.Id)
	err = db.Update(func(txn *badger.Txn) error {
		err = txn.Set(keys.RawChangeKey(payload.RootRawChange.Id), payload.RootRawChange.GetRawChange())
		if err != nil {
			return err
		}
		err = txn.Set(keys.HeadsKey(), treestorage.CreateHeadsPayload([]string{payload.RootRawChange.Id}))
		if err != nil {
			return err
		}
		err = txn.Set(keys.RootIdKey(), nil)
		if err != nil {
			return err
		}
		ts = &treeStorage{
			db:   db,
			keys: keys,
			id:   payload.RootRawChange.Id,
			root: payload.RootRawChange,
		}
		return nil
	})
	if err != nil {
		return
	}
	err = ts.AddRawChangesSetHeads(payload.Changes, payload.Heads)
	return
}

func (t *treeStorage) Id() string {
	return t.id
}

func (t *treeStorage) Root() (raw *treechangeproto.RawTreeChangeWithId, err error) {
	return t.root, nil
}

func (t *treeStorage) Heads() (heads []string, err error) {
	headsBytes, err := getDB(t.db, t.keys.HeadsKey())
	if err != nil {
		if err == badger.ErrKeyNotFound {
			err = treestorage.ErrUnknownTreeId
		}
		return
	}
	heads = treestorage.ParseHeads(headsBytes)
	return
}

func (t *treeStorage) GetAllChangeIds() (chs []string, err error) {
	return nil, fmt.Errorf("get all change ids should not be called")
}

func (t *treeStorage) SetHeads(heads []string) (err error) {
	payload := treestorage.CreateHeadsPayload(heads)
	return putDB(t.db, t.keys.HeadsKey(), payload)
}

func (t *treeStorage) AddRawChange(change *treechangeproto.RawTreeChangeWithId) (err error) {
	return putDB(t.db, t.keys.RawChangeKey(change.Id), change.RawChange)
}

func (t *treeStorage) AddRawChangesSetHeads(changes []*treechangeproto.RawTreeChangeWithId, heads []string) error {
	for _, ch := range changes {
		err := t.AddRawChange(ch)
		if err != nil {
			return err
		}
	}
	return t.SetHeads(heads)
}

func (t *treeStorage) GetRawChange(ctx context.Context, id string) (raw *treechangeproto.RawTreeChangeWithId, err error) {
	res, err := getDB(t.db, t.keys.RawChangeKey(id))
	if err != nil {
		if err == badger.ErrKeyNotFound {
			err = treestorage.ErrUnknownTreeId
		}
		return
	}

	raw = &treechangeproto.RawTreeChangeWithId{
		RawChange: res,
		Id:        id,
	}
	return
}

func (t *treeStorage) GetAppendRawChange(ctx context.Context, buf []byte, id string) (raw *treechangeproto.RawTreeChangeWithId, err error) {
	return t.GetRawChange(ctx, id)
}

func (t *treeStorage) HasChange(ctx context.Context, id string) (bool, error) {
	return hasDB(t.db, t.keys.RawChangeKey(id)), nil
}

func (t *treeStorage) Delete() (err error) {
	storedKeys, err := t.storedKeys()
	if err != nil {
		return
	}
	err = t.db.Update(func(txn *badger.Txn) error {
		for _, k := range storedKeys {
			err = txn.Delete(k)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return
}

func (t *treeStorage) storedKeys() (keys [][]byte, err error) {
	err = t.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		// this will get all raw changes and also "heads"
		opts.Prefix = t.keys.RawChangePrefix()

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			keyCopy := make([]byte, 0, len(key))
			keyCopy = item.KeyCopy(keyCopy)
			keys = append(keys, keyCopy)
		}
		return nil
	})
	if err != nil {
		return
	}
	keys = append(keys, t.keys.RootIdKey())
	return
}
