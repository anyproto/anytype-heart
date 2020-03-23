package localstore

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	"github.com/anytypeio/go-anytype-library/pb/lsmodel"
)

// FileIndex is stored in db key pattern:
// /files/index/<hash>
var (
	filesMetaBase = ds.NewKey("/files/index")

	_ FileStore = (*dsFileStore)(nil)

	indexMillSourceOpts = Index{
		Name: "mill_source_opts",
		Values: func(val interface{}) []string {
			if v, ok := val.(*lsmodel.FileIndex); ok {
				return []string{v.Mill, v.Source, v.Opts}
			}
			return nil
		},
		Unique: true,
	}

	indexMillChecksum = Index{
		Name: "mill_checksum",
		Values: func(val interface{}) []string {
			if v, ok := val.(*lsmodel.FileIndex); ok {
				return []string{v.Mill, v.Checksum}
			}
			return nil
		},
		Unique: true,
	}
)

type dsFileStore struct {
	ds ds.TxnDatastore
}

func NewFileStore(ds ds.TxnDatastore) FileStore {
	return &dsFileStore{
		ds: ds,
	}
}

func (m *dsFileStore) Prefix() string {
	return "files"
}

func (m *dsFileStore) Indexes() []Index {
	return []Index{
		indexMillChecksum,
		indexMillSourceOpts,
	}
}

func (m *dsFileStore) Add(file *lsmodel.FileIndex) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	metaKey := filesMetaBase.ChildString(file.Hash)
	err = AddIndexes(m, m.ds, file, file.Hash)
	if err != nil {
		return err
	}

	exists, err := txn.Has(metaKey)
	if err != nil {
		return err
	}
	if exists {
		return ErrDuplicateKey
	}

	b, err := proto.Marshal(file)
	if err != nil {
		return err
	}

	err = txn.Put(metaKey, b)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsFileStore) GetByHash(hash string) (*lsmodel.FileIndex, error) {
	metaKey := filesMetaBase.ChildString(hash)
	b, err := m.ds.Get(metaKey)
	if err != nil {
		if err == ds.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	file := lsmodel.FileIndex{}
	err = proto.Unmarshal(b, &file)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

func (m *dsFileStore) GetByChecksum(mill string, checksum string) (*lsmodel.FileIndex, error) {
	key, err := GetKeyByIndex(m.Prefix(), indexMillChecksum, m.ds, &lsmodel.FileIndex{Mill: mill, Checksum: checksum})
	if err != nil {
		return nil, err
	}

	val, err := m.ds.Get(filesMetaBase.ChildString(key))
	if err != nil {
		return nil, err
	}

	file := lsmodel.FileIndex{}
	err = proto.Unmarshal(val, &file)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

func (m *dsFileStore) GetBySource(mill string, source string, opts string) (*lsmodel.FileIndex, error) {
	key, err := GetKeyByIndex(m.Prefix(), indexMillSourceOpts, m.ds, &lsmodel.FileIndex{Mill: mill, Source: source, Opts: opts})
	if err != nil {
		return nil, err
	}

	val, err := m.ds.Get(filesMetaBase.ChildString(key))
	if err != nil {
		return nil, err
	}

	file := lsmodel.FileIndex{}
	err = proto.Unmarshal(val, &file)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

func (m *dsFileStore) Count() (int, error) {
	count, err := m.ds.GetSize(filesMetaBase)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (m *dsFileStore) DeleteByHash(hash string) error {
	metaKey := filesMetaBase.ChildString(hash)
	return m.ds.Delete(metaKey)
}
