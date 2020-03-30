package localstore

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/ipfs/go-datastore"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log"
	"github.com/multiformats/go-base32"

	"github.com/anytypeio/go-anytype-library/pb/storage"
)

var ErrDuplicateKey = fmt.Errorf("duplicate key")
var ErrNotFound = fmt.Errorf("not found")

var log = logging.Logger("anytype-localstore")

var (
	indexBase = ds.NewKey("/idx")
)

type LocalStore struct {
	Files FileStore
}

type FileStore interface {
	Indexable
	Add(file *storage.FileInfo) error
	GetByHash(hash string) (*storage.FileInfo, error)
	GetBySource(mill string, source string, opts string) (*storage.FileInfo, error)
	GetByChecksum(mill string, checksum string) (*storage.FileInfo, error)
	AddTarget(hash string, target string) error
	RemoveTarget(hash string, target string) error
	ListByTarget(target string) ([]*storage.FileInfo, error)
	Count() (int, error)
	DeleteByHash(hash string) error
}

func NewLocalStore(store ds.Batching) LocalStore {
	fileStore := NewFileStore(store.(ds.TxnDatastore))

	return LocalStore{Files: fileStore}
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

func AddIndexes(store Indexable, ds ds.TxnDatastore, newVal interface{}, newValPrimary string) error {
	for _, index := range store.Indexes() {
		err := AddIndex(index, ds, newVal, newValPrimary)
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

	log.Debugf("GetKeysByIndexParts %s", key.String()+"/")
	return ds.Query(query.Query{
		Prefix:   key.String() + "/",
		Limit:    limit,
		KeysOnly: true,
	})
}

func GetAllKeysFromResults(results query.Results) ([]string, error) {
	var keys []string
	for {
		res, ok := <-results.Next()
		if !ok {
			break
		}
		if res.Error != nil {
			return nil, res.Error
		}

		key := datastore.RawKey(res.Key)
		keyParts := key.List()
		keys = append(keys, keyParts[len(keyParts)-1])
	}

	return keys, nil
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

	return ds.Query(query.Query{
		Prefix:   key.String() + "/",
		Limit:    limit,
		KeysOnly: true,
	})
}
