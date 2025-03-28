package badgerstorage

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/dgraph-io/badger/v4"
)

var (
	ErrIncorrectKey  = errors.New("key format is incorrect")
	ErrUnknownRecord = errors.New("record does not exist")
)

type listStorage struct {
	db   *badger.DB
	keys aclKeys
	id   string
	root *consensusproto.RawRecordWithId
}

func newListStorage(spaceId string, db *badger.DB, txn *badger.Txn) (ls oldstorage.ListStorage, err error) {
	keys := newAclKeys(spaceId)
	rootId, err := getTxn(txn, keys.RootIdKey())
	if err != nil {
		return
	}

	stringID := string(rootId)
	value, err := getTxn(txn, keys.RawRecordKey(stringID))
	if err != nil {
		return
	}

	rootWithID := &consensusproto.RawRecordWithId{
		Payload: value,
		Id:      stringID,
	}

	ls = &listStorage{
		db:   db,
		keys: keys,
		id:   stringID,
		root: rootWithID,
	}
	return
}

func createListStorage(spaceID string, db *badger.DB, txn *badger.Txn, root *consensusproto.RawRecordWithId) (ls oldstorage.ListStorage, err error) {
	keys := newAclKeys(spaceID)
	_, err = getTxn(txn, keys.RootIdKey())
	if err != badger.ErrKeyNotFound {
		if err == nil {
			return newListStorage(spaceID, db, txn)
		}
		return
	}

	err = txn.Set(keys.HeadIdKey(), []byte(root.Id))
	if err != nil {
		return
	}

	err = txn.Set(keys.RawRecordKey(root.Id), root.Payload)
	if err != nil {
		return
	}
	err = txn.Set(keys.RootIdKey(), []byte(root.Id))
	if err != nil {
		return
	}

	ls = &listStorage{
		db:   db,
		keys: keys,
		id:   root.Id,
		root: root,
	}
	return
}

func (l *listStorage) Id() string {
	return l.id
}

func (l *listStorage) Root() (*consensusproto.RawRecordWithId, error) {
	return l.root, nil
}

func (l *listStorage) Head() (head string, err error) {
	bytes, err := getDB(l.db, l.keys.HeadIdKey())
	if err != nil {
		return
	}
	head = string(bytes)
	return
}

func (l *listStorage) GetRawRecord(_ context.Context, id string) (raw *consensusproto.RawRecordWithId, err error) {
	res, err := getDB(l.db, l.keys.RawRecordKey(id))
	if err != nil {
		if err == badger.ErrKeyNotFound {
			err = ErrUnknownRecord
		}
		return
	}

	raw = &consensusproto.RawRecordWithId{
		Payload: res,
		Id:      id,
	}
	return
}

func (l *listStorage) SetHead(headId string) (err error) {
	return putDB(l.db, l.keys.HeadIdKey(), []byte(headId))
}

func (l *listStorage) AddRawRecord(_ context.Context, rec *consensusproto.RawRecordWithId) error {
	return putDB(l.db, l.keys.RawRecordKey(rec.Id), rec.Payload)
}
