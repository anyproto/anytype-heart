package localstore

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-library/pb/lsmodel"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/multiformats/go-base32"
)

var ErrDuplicateKey = fmt.Errorf("duplicate key")
var ErrNotFound = fmt.Errorf("not found")

var (
	indexBase = ds.NewKey("/idx")
)

type LocalStore struct {
	Files FileStore
}

type FileStore interface {
	Indexable
	Add(file *lsmodel.FileIndex) error
	GetByHash(hash string) (*lsmodel.FileIndex, error)
	GetBySource(mill string, source string, opts string) (*lsmodel.FileIndex, error)
	GetByChecksum(mill string, checksum string) (*lsmodel.FileIndex, error)

	//AddTarget(hash string, target string) error
	//RemoveTarget(hash string, target string) error
	Count() (int, error)
	DeleteByHash(hash string) error
}

func NewLocalStore(store ds.Batching) LocalStore {
	fileStore := NewFileStore(store.(ds.TxnDatastore))

	return LocalStore{Files: fileStore}
}

type Indexable interface {
	Indexes() []Index
	Prefix() string
}

type Index struct {
	Name   string
	Values func(val interface{}) []string
	Unique bool
	Hash   bool
	Primary bool
}

func AddIndexes(store Indexable, ds ds.TxnDatastore, newVal interface{}, newValPrimary string) error {
	for _, index := range store.Indexes() {
		keyStr := strings.Join(index.Values(newVal), "")
		if index.Hash {
			keyBytesF := sha256.Sum256([]byte(keyStr))
			keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
		}

		key := indexBase.ChildString(store.Prefix()).ChildString(keyStr)
		if index.Unique {
			exists, err := ds.Has(key)
			if err != nil {
				return err
			}
			if exists {
				return ErrDuplicateKey
			}
		}

		err := ds.Put(key.ChildString(newValPrimary), []byte{})
		if err != nil {
			return err
		}
	}

	return nil
}

func GetKeyByIndex(prefix string, index Index, ds ds.TxnDatastore, val interface{}) (string, error) {
	results, err := GetKeysByIndex(prefix, index, ds, val,1)
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

	return res.Key, nil
}

func GetKeysByIndex(prefix string, index Index, ds ds.TxnDatastore, val interface{}, limit int) (query.Results, error) {
	indexKeyValues := index.Values(val)
	if indexKeyValues == nil {
		return nil, fmt.Errorf("failed to get index key values – may be incorrect val interface")
	}

	keyStr := strings.Join(index.Values(val), "")
	if index.Hash {
		keyBytesF := sha256.Sum256([]byte(keyStr))
		keyStr = base32.RawStdEncoding.EncodeToString(keyBytesF[:])
	}

	key := indexBase.ChildString(prefix).ChildString(keyStr)
	if index.Unique {
		limit = 1
	}

	return ds.Query(query.Query{
		Prefix:   key.String() + "/",
		Limit:    limit,
		KeysOnly: true,
	})
}
