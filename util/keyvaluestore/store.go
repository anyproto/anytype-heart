package keyvaluestore

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"

	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

var ErrNotFound = fmt.Errorf("not found")

type Store[T any] interface {
	Get(key string) (T, error)
	Set(key string, value T) error
	Delete(key string) error
	Has(key string) (bool, error)
}

type store[T any] struct {
	prefix []byte
	db     *badger.DB

	marshaller   func(T) ([]byte, error)
	unmarshaller func([]byte) (T, error)
}

func New[T any](
	db *badger.DB,
	prefix []byte,
	marshaller func(T) ([]byte, error),
	unmarshaller func([]byte) (T, error),
) Store[T] {
	return &store[T]{
		prefix:       prefix,
		db:           db,
		marshaller:   marshaller,
		unmarshaller: unmarshaller,
	}
}

func (s *store[T]) Get(key string) (T, error) {
	val, err := badgerhelper.GetValue(s.db, s.makeKey(key), s.unmarshaller)
	if badgerhelper.IsNotFound(err) {
		return val, ErrNotFound
	}
	return val, err
}

func (s *store[T]) Has(key string) (bool, error) {
	var ok bool
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		ok, err = badgerhelper.Has(txn, s.makeKey(key))
		return err
	})
	return ok, err
}

func (s *store[T]) Set(key string, value T) error {
	return s.db.Update(func(txn *badger.Txn) error {
		raw, err := s.marshaller(value)
		if err != nil {
			return fmt.Errorf("marhsal: %w", err)
		}
		return txn.Set(s.makeKey(key), raw)
	})
}

func (s *store[T]) Delete(key string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(s.makeKey(key))
	})
}

func (s *store[T]) makeKey(key string) []byte {
	return append(s.prefix, []byte(key)...)
}
