package persistentqueue

import (
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger/v4"

	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

type FactoryFunc[T Item] func() T

type Storage[T Item] interface {
	Put(item T) error
	Delete(key string) error
	List() ([]T, error)
}

type badgerStorage[T Item] struct {
	db           *badger.DB
	badgerPrefix []byte
	// factoryFunc is used to create new instances of T
	factoryFunc FactoryFunc[T]
}

func NewBadgerStorage[T Item](db *badger.DB, badgerPrefix []byte, factoryFunc FactoryFunc[T]) Storage[T] {
	return &badgerStorage[T]{
		db:           db,
		badgerPrefix: badgerPrefix,
		factoryFunc:  factoryFunc,
	}
}

func (s *badgerStorage[T]) List() ([]T, error) {
	var items []T
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("badger iterator panic: %v", r)
		}
	}()
	err = s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchSize:   100,
			PrefetchValues: true,
			Prefix:         s.badgerPrefix,
		})
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			qItem, err := s.unmarshalItem(item)
			if err != nil {
				return fmt.Errorf("get queue item %s: %w", item.Key(), err)
			}
			items = append(items, qItem)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *badgerStorage[T]) unmarshalItem(item *badger.Item) (T, error) {
	it := s.factoryFunc()
	err := item.Value(func(raw []byte) error {
		return json.Unmarshal(raw, it)
	})
	if err != nil {
		return it, err
	}
	return it, nil
}

func (s *badgerStorage[T]) Put(item T) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("create queue item: %w", err)
	}
	return badgerhelper.SetValue(s.db, s.makeKey(item.Key()), raw)
}

func (s *badgerStorage[T]) Delete(key string) error {
	return badgerhelper.DeleteValue(s.db, s.makeKey(key))
}

func (s *badgerStorage[T]) makeKey(itemKey string) []byte {
	return append(s.badgerPrefix, []byte(itemKey)...)
}
