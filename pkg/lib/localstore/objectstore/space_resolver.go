package objectstore

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/dgraph-io/badger/v3"
)

var (
	shortSpaceToLongPrefix = []byte("/space/short_to_long/")
	longSpaceToShortPrefix = []byte("/space/long_to_short/")
	spaceMappingPrefix     = []byte("/space/id/")
)

func spaceMappingKey(objectID string) []byte {
	return append(spaceMappingPrefix, []byte(objectID)...)
}

func shortSpaceToLongKey(shortSpaceID string) []byte {
	return append(shortSpaceToLongPrefix, []byte(shortSpaceID)...)
}

func longSpaceToShortKey(longSpaceID string) []byte {
	return append(longSpaceToShortPrefix, []byte(longSpaceID)...)
}

func (s *dsObjectStore) ResolveSpaceID(objectID string) (string, error) {
	var spaceID string
	if addr.IsBundledId(objectID) {
		return addr.AnytypeMarketplaceWorkspace, nil
	}
	err := s.db.View(func(txn *badger.Txn) error {
		shortSpaceID, err := s.getObjectShortSpaceID(txn, objectID)
		if err != nil {
			return fmt.Errorf("get short space ID: %w", err)
		}
		spaceID, err = s.getLongSpaceID(txn, shortSpaceID)
		if err != nil {
			return fmt.Errorf("get long space ID: %w", err)
		}
		return nil
	})
	if isNotFound(err) {
		return "", nil
	}
	return spaceID, err
}

func (s *dsObjectStore) StoreSpaceID(objectID, spaceID string) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		_, err := txn.Get(spaceMappingKey(objectID))
		if isNotFound(err) {
			shortSpaceID, err := s.getSpaceShortID(txn, spaceID)
			if isNotFound(err) {
				shortSpaceID, err = s.createAndStoreSpaceMapping(txn, spaceID)
				if err != nil {
					return fmt.Errorf("store short space ID: %w", err)
				}
			} else if err != nil {
				return fmt.Errorf("get short space ID: %w", err)
			}
			return setValueTxn(txn, spaceMappingKey(objectID), shortSpaceID)
		} else if err != nil {
			return fmt.Errorf("get space mapping: %w", err)
		}
		return nil
	})
}

func (s *dsObjectStore) getObjectShortSpaceID(txn *badger.Txn, objectID string) (string, error) {
	return s.getStringForSpaceResolver(txn, spaceMappingKey(objectID))
}

func (s *dsObjectStore) getLongSpaceID(txn *badger.Txn, shortSpaceID string) (string, error) {
	return s.getStringForSpaceResolver(txn, shortSpaceToLongKey(shortSpaceID))
}

func (s *dsObjectStore) getSpaceShortID(txn *badger.Txn, spaceID string) (string, error) {
	return s.getStringForSpaceResolver(txn, longSpaceToShortKey(spaceID))
}

func (s *dsObjectStore) createAndStoreSpaceMapping(txn *badger.Txn, spaceID string) (string, error) {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = shortSpaceToLongPrefix
	iter := txn.NewIterator(opts)
	defer iter.Close()

	last := -1
	for iter.Rewind(); iter.Valid(); iter.Next() {
		key := iter.Item().Key()
		rawShortSpaceID := bytes.TrimPrefix(key, shortSpaceToLongPrefix)
		integer, err := strconv.Atoi(string(rawShortSpaceID))
		if err != nil {
			return "", fmt.Errorf("convert short space ID %s to integer: %w", rawShortSpaceID, err)
		}
		if integer > last {
			last = integer
		}
	}

	shortSpaceID := strconv.Itoa(last + 1)
	err := s.storeSpaceMapping(txn, shortSpaceID, spaceID)
	if err != nil {
		return "", fmt.Errorf("store spaceID mapping: %w", err)
	}
	return shortSpaceID, nil
}

func (s *dsObjectStore) storeSpaceMapping(txn *badger.Txn, shortSpaceID, spaceID string) error {
	err := s.storeStringForSpaceResolver(txn, shortSpaceToLongKey(shortSpaceID), spaceID)
	if err != nil {
		return fmt.Errorf("store short->long space ID: %w", err)
	}
	err = s.storeStringForSpaceResolver(txn, longSpaceToShortKey(spaceID), shortSpaceID)
	if err != nil {
		return fmt.Errorf("store long->short space ID: %w", err)
	}
	return nil
}

func (s *dsObjectStore) getStringForSpaceResolver(txn *badger.Txn, key []byte) (string, error) {
	if val, ok := s.spaceResolverCache.Get(key); ok {
		return val.(string), nil
	}
	val, err := getValueTxn(txn, key, bytesToString)
	if err != nil {
		return "", err
	}
	s.spaceResolverCache.Set(key, val, int64(len(val)))
	return val, nil
}

func (s *dsObjectStore) storeStringForSpaceResolver(txn *badger.Txn, key []byte, value string) error {
	err := setValueTxn(txn, key, value)
	if err != nil {
		return fmt.Errorf("store string for space resolver: %w", err)
	}
	s.spaceResolverCache.Set(key, value, int64(len(value)))
	return nil
}
