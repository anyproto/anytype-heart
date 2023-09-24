package objectcache

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/dgraph-io/badger/v3"

	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

type resolverKeys struct{}

func (r resolverKeys) ObjectIDKey(objectID string) []byte {
	return treestorage.JoinStringsToBytes("resolver", objectID)
}

type resStorage struct {
	db   *badger.DB
	keys resolverKeys
}

func newResolverStorage(db *badger.DB) resolverStorage {
	return &resStorage{
		db: db,
	}
}

func (s *resStorage) StoreIDs(spaceID string, objectIDs []string) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		for _, objectID := range objectIDs {
			if err := badgerhelper.SetValueTxn(txn, s.keys.ObjectIDKey(objectID), []byte(spaceID)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *resStorage) ResolveSpaceID(objectID string) (spaceID string, err error) {
	return badgerhelper.GetValue(s.db, s.keys.ObjectIDKey(objectID), func(bytes []byte) (string, error) {
		return string(bytes), nil
	})
}

func (s *resStorage) StoreSpaceID(spaceID, objectID string) (err error) {
	return badgerhelper.SetValue(s.db, s.keys.ObjectIDKey(objectID), []byte(spaceID))
}
