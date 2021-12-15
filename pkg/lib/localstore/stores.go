package localstore

import (
	"crypto/sha256"
	"fmt"
	datastore2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/dgtony/collections/polymorph"
	"github.com/dgtony/collections/slices"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/multiformats/go-base32"
)

var (
	ErrDuplicateKey = fmt.Errorf("duplicate key")
	ErrNotFound     = fmt.Errorf("not found")
	errTxnTooBig    = fmt.Errorf("Txn is too big to fit into one request")
)

var (
	log       = logging.Logger("anytype-localstore")
	IndexBase = ds.NewKey("/idx")
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

func AddIndex(index Index, ds ds.TxnDatastore, newVal interface{}, newValPrimary string) error {
	txn, err := ds.NewTransaction(false)
	if err != nil {
		return err
	}

	defer txn.Discard()

	err = AddIndexWithTxn(index, txn, newVal, newValPrimary)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func UpdateIndexWithTxn(index Index, txn ds.Txn, oldVal interface{}, newVal interface{}, newValPrimary string) error {
	oldKeys := index.JoinedKeys(oldVal)
	getFullKey := func(key string) ds.Key {
		return IndexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(key).ChildString(newValPrimary)
	}

	newKeys := index.JoinedKeys(newVal)

	removed, added := slice.DifferenceRemovedAdded(oldKeys, newKeys)
	if len(oldKeys) > 0 {
		exists, err := txn.Has(getFullKey(oldKeys[0]))
		if err != nil {
			return err
		}

		if !exists {
			// inconsistency – lets add all keys, not only the new ones
			added = newKeys
		}
	}

	for _, removedKey := range removed {
		key := getFullKey(removedKey)
		exists, err := txn.Has(key)
		if err != nil {
			return err
		}

		if !exists {
			continue
		}
		log.Debugf("update(remove) index at %s", key.String())

		err = txn.Delete(key)
		if err != nil {
			return err
		}
	}

	for _, addedKey := range added {
		key := getFullKey(addedKey)
		exists, err := txn.Has(key)
		if err != nil {
			return err
		}

		if exists {
			continue
		}
		log.Debugf("update(add) index at %s", key.String())

		err = txn.Put(key, []byte{})
		if err != nil {
			return err
		}
	}
	return nil
}

func AddIndexWithTxn(index Index, ds ds.Txn, newVal interface{}, newValPrimary string) error {
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
			exists, err := ds.Has(key)
			if err != nil {
				return err
			}
			if exists {
				return ErrDuplicateKey
			}
		}

		key = key.ChildString(newValPrimary)
		log.Debugf("add index at %s", key.String())
		err := ds.Put(key, []byte{})
		if err != nil {
			return err
		}
	}
	return nil
}

// EraseIndex deletes the whole index
func EraseIndex(index Index, datastore datastore2.DSTxnBatching) error {
	key := IndexBase.ChildString(index.Prefix).ChildString(index.Name)
	txn, err := datastore.NewTransaction(true)
	if err != nil {
		return err
	}

	res, err := GetKeys(txn, key.String(), 0)
	if err != nil {
		return err
	}

	keys, err := ExtractKeysFromResults(res)
	b, err := datastore.Batch()
	if err != nil {
		return err
	}
	for _, key := range keys {
		err = b.Delete(ds.NewKey(key))
		if err != nil {
			return err
		}
	}
	return nil
}

func RemoveIndexWithTxn(index Index, txn ds.Txn, val interface{}, valPrimary string) error {
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

		exists, err := txn.Has(key.ChildString(valPrimary))
		if err != nil {
			return err
		}

		if !exists {
			return nil
		}

		err = txn.Delete(key.ChildString(valPrimary))
		if err != nil {
			return err
		}
	}
	return nil
}

func UpdateIndexesWithTxn(store Indexable, txn ds.Txn, oldVal interface{}, newVal interface{}, newValPrimary string) error {
	for _, index := range store.Indexes() {
		err := UpdateIndexWithTxn(index, txn, oldVal, newVal, newValPrimary)
		if err != nil {
			return err
		}
	}

	return nil
}

func AddIndexesWithTxn(store Indexable, txn ds.Txn, newVal interface{}, newValPrimary string) error {
	for _, index := range store.Indexes() {
		err := AddIndexWithTxn(index, txn, newVal, newValPrimary)
		if err != nil {
			return err
		}
	}

	return nil
}

func AddIndexes(store Indexable, ds ds.TxnDatastore, newVal interface{}, newValPrimary string) error {
	txn, err := ds.NewTransaction(false)
	if err != nil {
		return err
	}
	defer txn.Discard()

	err = AddIndexesWithTxn(store, txn, newVal, newValPrimary)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func RemoveIndexes(store Indexable, ds ds.TxnDatastore, val interface{}, valPrimary string) error {
	txn, err := ds.NewTransaction(false)
	if err != nil {
		return err
	}
	defer txn.Discard()

	err = RemoveIndexesWithTxn(store, txn, val, valPrimary)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func RemoveIndexesWithTxn(store Indexable, txn ds.Txn, val interface{}, valPrimary string) error {
	for _, index := range store.Indexes() {
		err := RemoveIndexWithTxn(index, txn, val, valPrimary)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetKeyByIndex(index Index, txn ds.Txn, val interface{}) (string, error) {
	results, err := GetKeysByIndex(index, txn, val, 1)
	if err != nil {
		return "", err
	}

	defer results.Close()
	res, ok := <-results.Next()
	if !ok {
		return "", ErrNotFound
	}

	if res.Error != nil {
		return "", res.Error
	}

	key := ds.RawKey(res.Key)
	keyParts := key.List()

	return keyParts[len(keyParts)-1], nil
}

func getDsKeyByIndexParts(prefix string, keyIndexName string, keyIndexValue []string, separator string, hash bool) ds.Key {
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

func GetKeysByIndexParts(txn ds.Txn, prefix string, keyIndexName string, keyIndexValue []string, separator string, hash bool, limit int) (query.Results, error) {
	key := getDsKeyByIndexParts(prefix, keyIndexName, keyIndexValue, separator, hash)

	return GetKeys(txn, key.String(), limit)
}

func QueryByIndexParts(txn ds.Txn, prefix string, keyIndexName string, keyIndexValue []string, separator string, hash bool, limit int) (query.Results, error) {
	key := getDsKeyByIndexParts(prefix, keyIndexName, keyIndexValue, separator, hash)

	return txn.Query(query.Query{
		Prefix:   key.String(),
		Limit:    limit,
		KeysOnly: false,
	})
}

func HasPrimaryKeyByIndexParts(txn ds.Txn, prefix string, keyIndexName string, keyIndexValue []string, separator string, hash bool, primaryIndex string) (exists bool, err error) {
	key := getDsKeyByIndexParts(prefix, keyIndexName, keyIndexValue, separator, hash).ChildString(primaryIndex)

	return txn.Has(key)
}

func CountAllKeysFromResults(results query.Results) (int, error) {
	var count int
	for {
		res, ok := <-results.Next()
		if !ok {
			break
		}
		if res.Error != nil {
			return -1, res.Error
		}

		count++
	}

	return count, nil
}

func GetLeavesFromResults(results query.Results) ([]string, error) {
	keys, err := ExtractKeysFromResults(results)
	if err != nil {
		return nil, err
	}

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

func GetKeyPartFromResults(results query.Results, from, to int, removeDuplicates bool) ([]string, error) {
	var keyParts []string
	for res := range results.Next() {
		if res.Error != nil {
			return nil, res.Error
		}
		p, err := CarveKeyParts(res.Key, from, to)
		if err != nil {
			// should not happen, lets early-close iterator and return error
			_ = results.Close()
			return nil, err
		}
		if removeDuplicates {
			if slice.FindPos(keyParts, p) >= 0 {
				continue
			}
		}
		keyParts = append(keyParts, p)
	}

	return keyParts, nil
}

func ExtractKeysFromResults(results query.Results) ([]string, error) {
	var keys []string
	for res := range results.Next() {
		if res.Error != nil {
			return nil, res.Error
		}
		keys = append(keys, res.Key)
	}

	return keys, nil
}

func CarveKeyParts(key string, from, to int) (string, error) {
	var keyParts = ds.RawKey(key).List()

	carved, err := slices.Carve(polymorph.FromStrings(keyParts), from, to)
	if err != nil {
		return "", err
	}

	return strings.Join(polymorph.ToStrings(carved), "/"), nil
}

func GetKeysByIndex(index Index, txn ds.Txn, val interface{}, limit int) (query.Results, error) {
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

	return GetKeys(txn, key.String(), limit)
}

func GetKeys(tx ds.Txn, prefix string, limit int) (query.Results, error) {
	return tx.Query(query.Query{
		Prefix:   prefix,
		Limit:    limit,
		KeysOnly: true,
	})
}
