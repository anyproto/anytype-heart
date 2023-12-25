package objectstore

import (
	"github.com/dgraph-io/badger/v4"

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

func iterateKeysByPrefixBatched(
	db *badger.DB,
	prefix []byte,
	limit int,
	processKeysFn func(keys [][]byte) error,
) error {
	return db.View(func(txn *badger.Txn) error {
		return iterateKeysByPrefixBatchedTx(txn, prefix, limit, processKeysFn)
	})
}

func iterateKeysByPrefixBatchedTx(
	txn *badger.Txn,
	prefix []byte,
	batchSize int,
	processKeysFn func(keys [][]byte) error,
) error {
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	opts.Prefix = prefix
	iter := txn.NewIterator(opts)
	defer iter.Close()

	var batch [][]byte
	count := 0

	for iter.Rewind(); iter.Valid(); iter.Next() {
		key := iter.Item().KeyCopy(nil)
		batch = append(batch, key)
		count++

		if count == batchSize {
			err := processKeysFn(batch)
			if err != nil {
				return err
			}
			count = 0
			batch = nil
		}
	}

	if count > 0 {
		err := processKeysFn(batch)
		if err != nil {
			return err
		}
	}

	return nil
}

func iterateKeysByPrefixTx(txn *badger.Txn, prefix []byte, processKeyFn func(key []byte)) error {
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	opts.Prefix = prefix
	iter := txn.NewIterator(opts)
	defer iter.Close()

	for iter.Rewind(); iter.Valid(); iter.Next() {
		key := iter.Item().KeyCopy(nil)
		processKeyFn(key)
	}
	return nil
}
