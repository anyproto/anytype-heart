package badgerhelper

import (
	"errors"

	"github.com/dgraph-io/badger/v3"
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
