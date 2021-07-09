package localstore

import (
	"crypto/sha256"
	"fmt"
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

func UpdateIndexWithTxn(index Index, ds ds.Txn, oldVal interface{}, newVal interface{}, newValPrimary string) error {
	oldKeys := index.JoinedKeys(oldVal)
	hasKey := func(key string) (exists bool, err error) {
		return ds.Has(IndexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(key))
	}

	if len(oldKeys) > 0 {
		exists, err := hasKey(oldKeys[0])
		if err != nil {
			return err
		}

		if !exists {
			oldKeys = []string{}
		}
	}

	newKeys := index.JoinedKeys(newVal)

	removed, added := slice.DifferenceRemovedAdded(oldKeys, newKeys)

	for _, removedKey := range removed {
		key := IndexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(removedKey).ChildString(newValPrimary)
		exists, err := ds.Has(key)
		if err != nil {
			return err
		}

		if !exists {
			continue
		}
		log.Debugf("update(remove) index at %s", key.String())

		err = ds.Delete(key)
		if err != nil {
			return err
		}
	}

	for _, addedKey := range added {
		key := IndexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(addedKey).ChildString(newValPrimary)
		exists, err := ds.Has(key)
		if err != nil {
			return err
		}

		if exists {
			continue
		}
		log.Debugf("update(add) index at %s", key.String())

		err = ds.Put(key, []byte{})
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

func EraseIndex(index Index, datastore ds.TxnDatastore) error {
	return RunLargeOperationWithinMultipleTxs(datastore, func(txn ds.Txn) error {
		return EraseIndexWithTxn(index, txn)
	})
}

// EraseIndexWithTxn deletes the whole index
func EraseIndexWithTxn(index Index, txn ds.Txn) error {
	key := IndexBase.ChildString(index.Prefix).ChildString(index.Name)
	res, err := GetKeys(txn, key.String(), 0)
	if err != nil {
		return err
	}

	keys, err := ExtractKeysFromResults(res)
	for _, key := range keys {
		err = txn.Delete(ds.NewKey(key))
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
		p, err := CarveKeyParts(res.Key, from, to)
		if err != nil {
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

// RunLargeOperationWithinMultipleTxs performs large operations. In case it faces ErrTxnTooBig it commits the txn and runs it again within the new txn
// underlying op func MUST be aware of ds change from previous retries – e.g. it should rebuild the list of pending operations at start instead of passing the fixed list from outside
func RunLargeOperationWithinMultipleTxs(datastore ds.TxnDatastore, op func(txn ds.Txn) error) (err error) {
	var txn ds.Txn
	for {
		txn, err = datastore.NewTransaction(false)
		if err != nil {
			return err
		}

		err = op(txn)
		if err != nil {
			// lets commit the current TXN and create another one
			if err.Error() == errTxnTooBig.Error() {
				err = txn.Commit()
				if err != nil {
					return err
				}
				continue
			}
			txn.Discard()
			return
		} else {
			err = txn.Commit()
			if err != nil {
				return err
			}
			return
		}
	}
}
