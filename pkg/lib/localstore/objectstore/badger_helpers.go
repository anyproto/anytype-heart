package objectstore

import (
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"
)

func (s *dsObjectStore) updateTxn(f func(txn *badger.Txn) error) error {
	return retryOnConflict(func() error {
		return s.db.Update(f)
	})
}

func retryOnConflict(proc func() error) error {
	for {
		err := proc()
		if err == nil {
			return nil
		}
		if errors.Is(err, badger.ErrConflict) {
			continue
		}
		return err
	}
}

func setValue(db *badger.DB, key []byte, value any) error {
	return db.Update(func(txn *badger.Txn) error {
		return setValueTxn(txn, key, value)
	})
}

func setValueTxn(txn *badger.Txn, key []byte, value any) error {
	raw, err := marshalValue(value)
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}
	return txn.Set(key, raw)
}

func marshalValue(value any) ([]byte, error) {
	if value != nil {
		switch v := value.(type) {
		case proto.Message:
			return proto.Marshal(v)
		case string:
			return []byte(v), nil
		default:
			return nil, fmt.Errorf("unsupported type %T", v)
		}
	}
	return nil, nil
}

func deleteValue(db *badger.DB, key []byte) error {
	return db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func getValue[T any](db *badger.DB, key []byte, unmarshaler func([]byte) (T, error)) (T, error) {
	var res T
	txErr := db.View(func(txn *badger.Txn) error {
		var err error
		res, err = getValueTxn(txn, key, unmarshaler)
		return err
	})
	return res, txErr
}

func getValueTxn[T any](txn *badger.Txn, key []byte, unmarshaler func([]byte) (T, error)) (T, error) {
	var res T
	item, err := txn.Get(key)
	if err != nil {
		return res, fmt.Errorf("get item: %w", err)
	}
	err = item.Value(func(val []byte) error {
		res, err = unmarshaler(val)
		return err
	})
	return res, err
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

func isNotFound(err error) bool {
	return errors.Is(err, badger.ErrKeyNotFound)
}
