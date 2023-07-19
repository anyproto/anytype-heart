package objectstore

import (
	"bytes"
	"fmt"
	"strconv"

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
	err := s.db.View(func(txn *badger.Txn) error {
		shortSpaceID, err := getValueTxn(txn, spaceMappingKey(objectID), bytesToString)
		if err != nil {
			return fmt.Errorf("get short space ID: %w", err)
		}
		spaceID, err = s.getLongSpaceID(txn, shortSpaceID)
		if err != nil {
			return fmt.Errorf("get long space ID: %w", err)
		}
		return nil
	})
	return spaceID, err
}

func (s *dsObjectStore) getLongSpaceID(txn *badger.Txn, shortSpaceID string) (string, error) {
	return getValueTxn(txn, shortSpaceToLongKey(shortSpaceID), bytesToString)
}

func (s *dsObjectStore) storeSpaceID(txn *badger.Txn, objectID, spaceID string) error {
	_, err := txn.Get(spaceMappingKey(objectID))
	if isNotFound(err) {
		shortSpaceID, err := s.getSpaceShortID(txn, spaceID)
		if isNotFound(err) {
			shortSpaceID, err = s.createAndStoreLongToShortMapping(txn, spaceID)
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
}

func (s *dsObjectStore) getSpaceShortID(txn *badger.Txn, spaceID string) (string, error) {
	return getValueTxn(txn, longSpaceToShortKey(spaceID), bytesToString)
}

func (s *dsObjectStore) createAndStoreLongToShortMapping(txn *badger.Txn, spaceID string) (string, error) {
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
	err := setValueTxn(txn, shortSpaceToLongKey(shortSpaceID), spaceID)
	if err != nil {
		return "", fmt.Errorf("store short->long space ID: %w", err)
	}
	err = setValueTxn(txn, longSpaceToShortKey(spaceID), shortSpaceID)
	if err != nil {
		return "", fmt.Errorf("store long->short space ID: %w", err)
	}
	return shortSpaceID, nil
}
