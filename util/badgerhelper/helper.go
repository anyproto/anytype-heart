package badgerhelper

import (
	"errors"

	"github.com/dgraph-io/badger/v3"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"encoding/binary"
)

func RetryOnConflict(proc func() error) error {
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

func Has(txn *badger.Txn, key []byte) (bool, error) {
	_, err := txn.Get(key)
	if err == nil {
		return true, nil
	}
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	return false, err
}

func SetValue(db *badger.DB, key []byte, value any) error {
	return db.Update(func(txn *badger.Txn) error {
		return SetValueTxn(txn, key, value)
	})
}

func SetValueTxn(txn *badger.Txn, key []byte, value any) error {
	raw, err := marshalValue(value)
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}
	return txn.Set(key, raw)
}

func marshalValue(value any) ([]byte, error) {
	if value != nil {
		switch v := value.(type) {
		case int:
			return binary.LittleEndian.AppendUint64(nil, uint64(v)), nil
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

func UnmarshalInt(raw []byte) (int, error) {
	return int(binary.LittleEndian.Uint64(raw)), nil
}

func DeleteValue(db *badger.DB, key []byte) error {
	return db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func GetValue[T any](db *badger.DB, key []byte, unmarshaler func([]byte) (T, error)) (T, error) {
	var res T
	txErr := db.View(func(txn *badger.Txn) error {
		var err error
		res, err = GetValueTxn(txn, key, unmarshaler)
		return err
	})
	return res, txErr
}

func GetValueTxn[T any](txn *badger.Txn, key []byte, unmarshaler func([]byte) (T, error)) (T, error) {
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

func IsNotFound(err error) bool {
	return errors.Is(err, badger.ErrKeyNotFound)
}

func ViewTxnWithResult[T any](db *badger.DB, f func(txn *badger.Txn) (T, error)) (T, error) {
	var res T
	resErr := db.View(func(txn *badger.Txn) error {
		var err error
		res, err = f(txn)
		return err
	})
	return res, resErr
}
