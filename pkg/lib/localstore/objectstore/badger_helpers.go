package objectstore

import (
	"github.com/dgraph-io/badger/v3"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

func (s *dsObjectStore) updateTxn(f func(txn *badger.Txn) error) error {
	return badgerhelper.RetryOnConflict(func() error {
		return s.db.Update(f)
	})
}

func iterateKeysByPrefix(db *badger.DB, prefix []byte, processKeyFn func(key []byte)) error {
	return db.View(func(txn *badger.Txn) error {
		return iterateKeysByPrefixTx(txn, prefix, processKeyFn)
	})
}

func iterateKeysByPrefixTx(txn *badger.Txn, prefix []byte, processKeyFn func(key []byte)) error {
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	opts.Prefix = prefix
	iter := txn.NewIterator(opts)
	defer iter.Close()

	for iter.Rewind(); iter.Valid(); iter.Next() {
		key := iter.Item().Key()
		processKeyFn(key)
	}
	return nil
}
