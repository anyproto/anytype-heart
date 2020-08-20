package localstore

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/util"
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
	Files FileStore
	Pages PageStore
}

type FileStore interface {
	Indexable
	Add(file *storage.FileInfo) error
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
}

type PageStore interface {
	Indexable
	database.Reader

	AddPage(page *model.PageInfoWithOutboundLinksIDs) error
	UpdatePage(id string, details *types.Struct, links []string, snippet string) error
	UpdateLastModified(id string, time time.Time) error
	UpdateLastOpened(id string, time time.Time) error
	DeletePage(id string) error

	GetWithLinksInfoByID(id string) (*model.PageInfoWithLinks, error)
	GetWithOutboundLinksInfoById(id string) (*model.PageInfoWithOutboundLinks, error)
	GetDetails(id string) (*model.PageDetails, error)
	GetByIDs(ids ...string) ([]*model.PageInfo, error)
	List() ([]*model.PageInfo, error)
}

func NewLocalStore(store ds.Batching) LocalStore {
	return LocalStore{
		Files: NewFileStore(store.(ds.TxnDatastore)),
		Pages: NewPageStore(store.(ds.TxnDatastore)),
	}
}

type Indexable interface {
	Indexes() []Index
}

type Index struct {
	Prefix string
	Name   string
	Keys   func(val interface{}) []IndexKeyParts
	Unique bool
	Hash   bool
}

type IndexKeyParts []string

func AddIndex(index Index, ds ds.TxnDatastore, newVal interface{}, newValPrimary string) error {
	for _, keyParts := range index.Keys(newVal) {
		keyStr := strings.Join(keyParts, "")
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
	for _, keyParts := range index.Keys(val) {
		keyStr := strings.Join(keyParts, "")
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

		err := ds.Delete(key.ChildString(valPrimary))
		if err != nil {
			return err
		}
	}
	return nil
}

func AddIndexes(store Indexable, ds ds.TxnDatastore, newVal interface{}, newValPrimary string) error {
	for _, index := range store.Indexes() {
		err := AddIndex(index, ds, newVal, newValPrimary)
		if err != nil {
			return err
		}
	}

	return nil
}

func RemoveIndexes(store Indexable, ds ds.TxnDatastore, val interface{}, valPrimary string) error {
	for _, index := range store.Indexes() {
		err := RemoveIndex(index, ds, val, valPrimary)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetKeyByIndex(index Index, ds ds.TxnDatastore, val interface{}) (string, error) {
	results, err := GetKeysByIndex(index, ds, val, 1)
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

func GetKeysByIndexParts(ds ds.TxnDatastore, prefix string, keyIndexName string, keyIndexValue []string, hash bool, limit int) (query.Results, error) {
	keyStr := strings.Join(keyIndexValue, "")
	if hash {
		keyBytesF := sha256.Sum256([]byte(keyStr))
		keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
	}

	key := indexBase.ChildString(prefix).ChildString(keyIndexName).ChildString(keyStr)
	return GetKeys(ds, key.String(), limit)
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
	keys, err := ExtractKeysFromResults(results, false)
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

func ExtractKeysFromResults(results query.Results, unique bool) ([]string, error) {
	var keys []string
	for res := range results.Next() {
		if res.Error != nil {
			return nil, res.Error
		}
		keys = append(keys, res.Key)
	}

	if unique {
		keys = util.UniqueStrings(keys)
	}

	return keys, nil
}

func CarveKeyParts(key string, from, to int) (string, error) {
	var keyParts = datastore.RawKey(key).List()

	carved, err := slices.Carve(polymorph.FromStrings(keyParts), from, to)
	if err != nil {
		return "", err
	}

	switch len(carved) {
	case 0:
		return "", nil
	case 1:
		return carved[0].(string), nil
	default:
		return strings.Join(polymorph.ToStrings(carved), "/"), nil
	}
}

func GetKeysByIndex(index Index, ds ds.TxnDatastore, val interface{}, limit int) (query.Results, error) {
	indexKeyValues := index.Keys(val)
	if indexKeyValues == nil {
		return nil, fmt.Errorf("failed to get index key values – may be incorrect val interface")
	}

	keys := index.Keys(val)
	if len(keys) > 1 {
		return nil, fmt.Errorf("multiple keys index not supported – use GetKeysByIndexParts instead")
	}

	keyStr := strings.Join(keys[0], "")
	if index.Hash {
		keyBytesF := sha256.Sum256([]byte(keyStr))
		keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
	}

	key := indexBase.ChildString(index.Prefix).ChildString(index.Name).ChildString(keyStr)
	if index.Unique {
		limit = 1
	}

	return GetKeys(ds, key.String(), limit)
}

func GetKeys(ds ds.TxnDatastore, prefix string, limit int) (query.Results, error) {
	return ds.Query(query.Query{
		Prefix:   prefix + "/",
		Limit:    limit,
		KeysOnly: true,
	})
}
