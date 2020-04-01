package localstore

import (
	"fmt"
	"sync"

	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	"github.com/anytypeio/go-anytype-library/pb/storage"
)

var (
	// FileInfo is stored in db key pattern:
	// /files/info/<hash>
	filesPrefix             = "files"
	filesInfoBase           = ds.NewKey("/" + filesPrefix + "/info")
	_             FileStore = (*dsFileStore)(nil)

	indexMillSourceOpts = Index{
		Prefix: filesPrefix,
		Name:   "mill_source_opts",
		Keys: func(val interface{}) []IndexKeyParts {
			if v, ok := val.(*storage.FileInfo); ok {
				return []IndexKeyParts{[]string{v.Mill, v.Source, v.Opts}}
			}
			return nil
		},
		Unique: true,
	}

	indexTargets = Index{
		Prefix: filesPrefix,
		Name:   "targets",
		Keys: func(val interface{}) []IndexKeyParts {
			if v, ok := val.(*storage.FileInfo); ok {
				var keys []IndexKeyParts
				for _, target := range v.Targets {
					keys = append(keys, []string{target})
				}

				return keys
			}
			return nil
		},
		Unique: true,
	}

	indexMillChecksum = Index{
		Prefix: filesPrefix,
		Name:   "mill_checksum",
		Keys: func(val interface{}) []IndexKeyParts {
			if v, ok := val.(*storage.FileInfo); ok {
				return []IndexKeyParts{[]string{v.Mill, v.Checksum}}
			}
			return nil
		},
		Unique: false,
	}
)

type dsFileStore struct {
	ds ds.TxnDatastore
	l  sync.Mutex
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
		indexTargets,
	}
}

func (m *dsFileStore) Add(file *storage.FileInfo) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	fileInfoKey := filesInfoBase.ChildString(file.Hash)
	err = AddIndexes(m, m.ds, file, file.Hash)
	if err != nil {
		return err
	}

	exists, err := txn.Has(fileInfoKey)
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

	err = txn.Put(fileInfoKey, b)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsFileStore) AddTarget(hash string, target string) error {
	// lock to protect from race AddTarget conds
	m.l.Lock()
	defer m.l.Unlock()

	file, err := m.GetByHash(hash)
	if err != nil {
		return err
	}

	for _, et := range file.Targets {
		if et == target {
			// already exists
			return nil
		}
	}

	file.Targets = append(file.Targets, target)

	b, err := proto.Marshal(file)
	if err != nil {
		return err
	}

	fileInfoKey := filesInfoBase.ChildString(file.Hash)
	err = AddIndex(indexTargets, m.ds, file, file.Hash)
	if err != nil {
		return err
	}

	return m.ds.Put(fileInfoKey, b)
}

func (m *dsFileStore) RemoveTarget(hash string, target string) error {
	// lock to protect from race conds
	m.l.Lock()
	defer m.l.Unlock()

	file, err := m.GetByHash(hash)
	if err != nil {
		return err
	}

	var filtered []string
	for _, et := range file.Targets {
		if et != target {
			filtered = append(filtered, et)
		}
	}

	if len(filtered) == len(file.Targets) {
		return nil
	}
	file.Targets = filtered

	b, err := proto.Marshal(file)
	if err != nil {
		return err
	}

	fileInfoKey := filesInfoBase.ChildString(file.Hash)

	return m.ds.Put(fileInfoKey, b)
}

func (m *dsFileStore) GetByHash(hash string) (*storage.FileInfo, error) {
	fileInfoKey := filesInfoBase.ChildString(hash)
	b, err := m.ds.Get(fileInfoKey)
	if err != nil {
		if err == ds.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	file := storage.FileInfo{}
	err = proto.Unmarshal(b, &file)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

func (m *dsFileStore) GetByChecksum(mill string, checksum string) (*storage.FileInfo, error) {
	key, err := GetKeyByIndex(indexMillChecksum, m.ds, &storage.FileInfo{Mill: mill, Checksum: checksum})
	if err != nil {
		return nil, err
	}

	val, err := m.ds.Get(filesInfoBase.ChildString(key))
	if err != nil {
		return nil, err
	}

	file := storage.FileInfo{}
	err = proto.Unmarshal(val, &file)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

func (m *dsFileStore) GetBySource(mill string, source string, opts string) (*storage.FileInfo, error) {
	key, err := GetKeyByIndex(indexMillSourceOpts, m.ds, &storage.FileInfo{Mill: mill, Source: source, Opts: opts})
	if err != nil {
		return nil, err
	}

	val, err := m.ds.Get(filesInfoBase.ChildString(key))
	if err != nil {
		return nil, err
	}

	file := storage.FileInfo{}
	err = proto.Unmarshal(val, &file)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

func (m *dsFileStore) ListByTarget(target string) ([]*storage.FileInfo, error) {
	results, err := GetKeysByIndexParts(m.ds, indexTargets.Prefix, indexTargets.Name, []string{target}, indexTargets.Hash, 0)
	if err != nil {
		return nil, err
	}

	keys, err := GetAllKeysFromResults(results)
	if err != nil {
		return nil, err
	}

	var files []*storage.FileInfo
	for _, key := range keys {
		val, err := m.ds.Get(filesInfoBase.ChildString(key))
		if err != nil {
			return nil, err
		}

		file := storage.FileInfo{}
		err = proto.Unmarshal(val, &file)
		if err != nil {
			return nil, err
		}

		files = append(files, &file)
	}

	return files, nil
}

func (m *dsFileStore) Count() (int, error) {
	count, err := m.ds.GetSize(filesInfoBase)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (m *dsFileStore) DeleteByHash(hash string) error {
	fileInfoKey := filesInfoBase.ChildString(hash)
	return m.ds.Delete(fileInfoKey)
}
