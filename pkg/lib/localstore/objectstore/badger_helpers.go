package objectstore

import (
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"
)

func setValue(db *badger.DB, key []byte, value any) error {
	var (
		raw []byte
		err error
	)
	switch v := value.(type) {
	case proto.Message:
		raw, err = proto.Marshal(v)
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}
	return db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, raw)
	})
}

func deleteValue(db *badger.DB, key []byte) error {
	return db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func getValue[T any](db *badger.DB, key []byte, unmarshaler func([]byte) (T, error)) (T, error) {
	var res T
	txErr := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return fmt.Errorf("get item: %w", err)
		}
		return item.Value(func(val []byte) error {
			res, err = unmarshaler(val)
			return err
		})
	})
	return res, txErr
}
