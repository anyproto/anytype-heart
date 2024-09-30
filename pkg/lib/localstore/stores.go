package localstore

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgtony/collections/polymorph"
	"github.com/dgtony/collections/slices"
	dsCtx "github.com/ipfs/go-datastore"
	"github.com/multiformats/go-base32"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

var (
	ErrDuplicateKey = fmt.Errorf("duplicate key")
	ErrNotFound     = fmt.Errorf("not found")
)

var (
	log       = logging.Logger("anytype-localstore")
	IndexBase = dsCtx.NewKey("/idx")
)

type Indexable interface {
	Indexes() []Index
}

type Index struct {
	Prefix             string
	Name               string
	Keys               func(val interface{}) []IndexKeyParts
	Unique             bool
	Hash               bool
	SplitIndexKeyParts bool // split IndexKeyParts using slash
}

type IndexKeyParts []string

func (i Index) JoinedKeys(val interface{}) []string {
	var keys []string
	var sep string
	if i.SplitIndexKeyParts {
		sep = "/"
	}

	var keyStr string
	for _, key := range i.Keys(val) {
		keyStr = strings.Join(key, sep)
		if i.Hash {
			keyBytesF := sha256.Sum256([]byte(keyStr))
			keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
		}
		keys = append(keys, keyStr)
	}
	return keys
}

func AddIndexWithTxn(index Index, txn *badger.Txn, newVal interface{}, newValPrimary string) error {
	for _, keyParts := range index.Keys(newVal) {
		var sep string
		if index.SplitIndexKeyParts {
			sep = "/"
		}
		keyStr := strings.Join(keyParts, sep)
		if index.Hash {
			keyBytesF := sha256.Sum256([]byte(keyStr))
			keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
		}

		key := IndexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(keyStr)
		if index.Unique {
			exists, err := badgerhelper.Has(txn, key.Bytes())
			if err != nil {
				return err
			}
			if exists {
				return ErrDuplicateKey
			}
		}

		key = key.ChildString(newValPrimary)
		log.Debugf("add index at %s", key.String())
		err := txn.Set(key.Bytes(), []byte{})
		if err != nil {
			return err
		}
	}
	return nil
}

func RemoveIndexWithTxn(index Index, txn *badger.Txn, val interface{}, valPrimary string) error {
	for _, keyParts := range index.Keys(val) {
		var sep string
		if index.SplitIndexKeyParts {
			sep = "/"
		}
		keyStr := strings.Join(keyParts, sep)
		if index.Hash {
			keyBytesF := sha256.Sum256([]byte(keyStr))
			keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
		}

		key := IndexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(keyStr)

		exists, err := badgerhelper.Has(txn, key.ChildString(valPrimary).Bytes())
		if err != nil {
			return err
		}

		if !exists {
			return nil
		}

		err = txn.Delete(key.ChildString(valPrimary).Bytes())
		if err != nil {
			return err
		}
	}
	return nil
}

func AddIndexesWithTxn(store Indexable, txn *badger.Txn, newVal interface{}, newValPrimary string) error {
	for _, index := range store.Indexes() {
		err := AddIndexWithTxn(index, txn, newVal, newValPrimary)
		if err != nil {
			return err
		}
	}

	return nil
}

func RemoveIndexesWithTxn(store Indexable, txn *badger.Txn, val interface{}, valPrimary string) error {
	for _, index := range store.Indexes() {
		err := RemoveIndexWithTxn(index, txn, val, valPrimary)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetKeyByIndex(index Index, txn *badger.Txn, val interface{}) (string, error) {
	keys, err := GetKeysByIndex(index, txn, val, 1)
	if err != nil {
		return "", err
	}

	if len(keys) == 0 {
		return "", ErrNotFound
	}

	key := dsCtx.RawKey(keys[0])
	keyParts := key.List()

	return keyParts[len(keyParts)-1], nil
}

func getDsKeyByIndexParts(prefix string, keyIndexName string, keyIndexValue []string, separator string, hash bool) dsCtx.Key {
	key := IndexBase.ChildString(prefix).ChildString(keyIndexName)
	if len(keyIndexValue) == 0 {
		return key
	}

	keyStr := strings.Join(keyIndexValue, separator)
	if hash {
		keyBytesF := sha256.Sum256([]byte(keyStr))
		keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
	}

	return key.ChildString(keyStr)
}

func GetKeysByIndexParts(txn *badger.Txn, prefix string, keyIndexName string, keyIndexValue []string, separator string, hash bool, limit int) ([]string, error) {
	key := getDsKeyByIndexParts(prefix, keyIndexName, keyIndexValue, separator, hash)

	return GetKeys(txn, key.String(), limit), nil
}

func GetLeavesFromResults(keys []string) ([]string, error) {
	var leaves = make([]string, len(keys))
	for i, key := range keys {
		leaf, err := CarveKeyParts(key, -1, 0)
		if err != nil {
			return nil, err
		}
		leaves[i] = leaf
	}

	return leaves, nil
}

func CarveKeyParts(key string, from, to int) (string, error) {
	var keyParts = dsCtx.RawKey(key).List()

	carved, err := slices.Carve(polymorph.FromStrings(keyParts), from, to)
	if err != nil {
		return "", err
	}

	return strings.Join(polymorph.ToStrings(carved), "/"), nil
}

func GetKeysByIndex(index Index, txn *badger.Txn, val interface{}, limit int) ([]string, error) {
	indexKeyValues := index.Keys(val)
	if indexKeyValues == nil {
		return nil, fmt.Errorf("failed to get index key values – may be incorrect val interface")
	}

	keys := index.Keys(val)
	if len(keys) > 1 {
		return nil, fmt.Errorf("multiple keys index not supported – use GetKeysByIndexParts instead")
	}

	var sep string
	if index.SplitIndexKeyParts {
		sep = "/"
	}

	keyStr := strings.Join(keys[0], sep)
	if index.Hash {
		keyBytesF := sha256.Sum256([]byte(keyStr))
		keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
	}

	key := IndexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(keyStr)
	if index.Unique {
		limit = 1
	}

	return GetKeys(txn, key.String(), limit), nil
}

func GetKeys(txn *badger.Txn, prefix string, limit int) []string {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("badger iterator panic: %v", r)
		}
	}()
	iter := txn.NewIterator(badger.IteratorOptions{
		Prefix:         []byte(prefix),
		PrefetchValues: false,
	})
	defer iter.Close()

	var (
		keys  []string
		count int
	)
	for iter.Rewind(); iter.Valid(); iter.Next() {
		count++
		if limit > 0 && count > limit {
			break
		}
		key := iter.Item().KeyCopy(nil)
		keys = append(keys, string(key))
	}
	return keys
}

// EraseIndex deletes the whole index
func EraseIndex(index Index, db *badger.DB, txn *badger.Txn) (*badger.Txn, error) {
	indexKey := IndexBase.ChildString(index.Prefix).ChildString(index.Name)
	keys := GetKeys(txn, indexKey.String(), 0)

	for _, key := range keys {
		key := dsCtx.NewKey(key).Bytes()
		err := txn.Delete(key)
		if errors.Is(err, badger.ErrTxnTooBig) {
			err = txn.Commit()
			if err != nil {
				return txn, fmt.Errorf("commit big transaction: %w", err)
			}
			txn = db.NewTransaction(true)
			err = txn.Delete(key)
		}
		if err != nil {
			return txn, fmt.Errorf("delete key %s: %w", key, err)
		}
	}
	return txn, nil
}
