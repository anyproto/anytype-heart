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
