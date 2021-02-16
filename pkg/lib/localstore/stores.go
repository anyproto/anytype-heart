package localstore

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/dgtony/collections/polymorph"
	"github.com/dgtony/collections/slices"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/multiformats/go-base32"
)

var ErrDuplicateKey = fmt.Errorf("duplicate key")
var ErrNotFound = fmt.Errorf("not found")

var log = logging.Logger("anytype-localstore")

var (
	indexBase = ds.NewKey("/idx")
)

type LocalStore struct {
	Files   FileStore
	Objects ObjectStore
}

type FileStore interface {
	Indexable
	Add(file *storage.FileInfo) error
	AddMulti(upsert bool, files ...*storage.FileInfo) error
	AddFileKeys(fileKeys ...FileKeys) error
	GetFileKeys(hash string) (map[string]string, error)
	GetByHash(hash string) (*storage.FileInfo, error)
	GetBySource(mill string, source string, opts string) (*storage.FileInfo, error)
	GetByChecksum(mill string, checksum string) (*storage.FileInfo, error)
	AddTarget(hash string, target string) error
	RemoveTarget(hash string, target string) error
	ListTargets() ([]string, error)
	ListByTarget(target string) ([]*storage.FileInfo, error)
	Count() (int, error)
	DeleteByHash(hash string) error
	DeleteFileKeys(hash string) error
	List() ([]*storage.FileInfo, error)
}

type ObjectStore interface {
	Indexable
	database.Reader

	CreateObject(id string, details *types.Struct, relations *pbrelation.Relations, links []string, snippet string) error
	UpdateObject(id string, details *types.Struct, relations *pbrelation.Relations, links []string, snippet string) error
	DeleteObject(id string) error
	RemoveRelationFromCache(key string) error

	UpdateRelationsInSet(setId, objTypeBefore, objTypeAfter string, relationsBefore, relationsAfter *pbrelation.Relations) error

	GetWithLinksInfoByID(id string) (*model.ObjectInfoWithLinks, error)
	GetWithOutboundLinksInfoById(id string) (*model.ObjectInfoWithOutboundLinks, error)
	GetDetails(id string) (*model.ObjectDetails, error)
	GetAggregatedOptions(relationKey string, relationFormat pbrelation.RelationFormat, objectType string) (options []*pbrelation.RelationOption, err error)

	GetByIDs(ids ...string) ([]*model.ObjectInfo, error)
	List() ([]*model.ObjectInfo, error)
	ListIds() ([]string, error)

	QueryObjectInfo(q database.Query, objectTypes []smartblock.SmartBlockType) (results []*model.ObjectInfo, total int, err error)
	AddToIndexQueue(id string) error
	IndexForEach(f func(id string, tm time.Time) error) error
	FTSearch() ftsearch.FTSearch
	Close()
}

func NewLocalStore(store ds.Batching, fts ftsearch.FTSearch) LocalStore {
	return LocalStore{
		Files:   NewFileStore(store.(ds.TxnDatastore)),
		Objects: NewObjectStore(store.(ds.TxnDatastore), fts),
	}
}

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

func (ls LocalStore) Close() error {
	if ls.Objects != nil {
		ls.Objects.Close()
	}
	return nil
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
	newKeys := index.JoinedKeys(newVal)

	removed, added := diffSlices(oldKeys, newKeys)

	for _, removedKey := range removed {
		key := indexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(removedKey).ChildString(newValPrimary)
		exists, err := ds.Has(key)
		if err != nil {
			return err
		}

		if !exists {
			continue
		}
		err = ds.Delete(key)
		if err != nil {
			return err
		}
	}

	for _, addedKey := range added {
		key := indexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(addedKey).ChildString(newValPrimary)
		exists, err := ds.Has(key)
		if err != nil {
			return err
		}

		if exists {
			continue
		}
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

		key := indexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(keyStr)
		if index.Unique {
			exists, err := ds.Has(key)
			if err != nil {
				return err
			}
			if exists {
				return ErrDuplicateKey
			}
		}

		log.Debugf("add index at %s", key.ChildString(newValPrimary).String())
		err := ds.Put(key.ChildString(newValPrimary), []byte{})
		if err != nil {
			return err
		}
	}
	return nil
}

func RemoveIndex(index Index, ds ds.TxnDatastore, val interface{}, valPrimary string) error {
	txn, err := ds.NewTransaction(false)
	if err != nil {
		return err
	}
	defer txn.Discard()

	err = RemoveIndexWithTxn(index, txn, val, valPrimary)
	if err != nil {
		return err
	}

	return txn.Commit()
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

		key := indexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(keyStr)

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

	key := datastore.RawKey(res.Key)
	keyParts := key.List()

	return keyParts[len(keyParts)-1], nil
}

func getDsKeyByIndexParts(prefix string, keyIndexName string, keyIndexValue []string, separator string, hash bool) ds.Key {
	key := indexBase.ChildString(prefix).ChildString(keyIndexName)
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
	var keyParts = datastore.RawKey(key).List()

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

	key := indexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(keyStr)
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
