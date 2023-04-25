package filestore

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/anytypeio/any-sync/app"
	"github.com/gogo/protobuf/proto"
	dsCtx "github.com/ipfs/go-datastore"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	ds "github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/noctxds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
)

var (
	// FileInfo is stored in db key pattern:
	// /files/info/<hash>
	filesPrefix     = "files"
	filesInfoBase   = dsCtx.NewKey("/" + filesPrefix + "/info")
	filesKeysBase   = dsCtx.NewKey("/" + filesPrefix + "/keys")
	chunksCountBase = dsCtx.NewKey("/" + filesPrefix + "/chunks_count")

	indexMillSourceOpts = localstore.Index{
		Prefix: filesPrefix,
		Name:   "mill_source_opts",
		Keys: func(val interface{}) []localstore.IndexKeyParts {
			if v, ok := val.(*storage.FileInfo); ok {
				return []localstore.IndexKeyParts{[]string{v.Mill, v.Source, v.Opts}}
			}
			return nil
		},
		Unique: true,
	}

	indexTargets = localstore.Index{
		Prefix: filesPrefix,
		Name:   "targets",
		Keys: func(val interface{}) []localstore.IndexKeyParts {
			if v, ok := val.(*storage.FileInfo); ok {
				var keys []localstore.IndexKeyParts
				for _, target := range v.Targets {
					keys = append(keys, []string{target})
				}

				return keys
			}
			return nil
		},
		Unique: true,
	}

	indexMillChecksum = localstore.Index{
		Prefix: filesPrefix,
		Name:   "mill_checksum",
		Keys: func(val interface{}) []localstore.IndexKeyParts {
			if v, ok := val.(*storage.FileInfo); ok {
				return []localstore.IndexKeyParts{[]string{v.Mill, v.Checksum}}
			}
			return nil
		},
		Unique: false,
	}
)

type dsFileStore struct {
	dsIface datastore.Datastore
	ds      ds.TxnDatastore
	l       sync.Mutex
}

var log = logging.Logger("anytype-localstore")

const CName = "filestore"

type FileStore interface {
	app.ComponentRunnable
	localstore.Indexable
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
	DeleteByTarget(targetHash string) error
	DeleteFileKeys(hash string) error
	ListFileKeys() ([]string, error)
	List() ([]*storage.FileInfo, error)
	RemoveEmpty() error

	GetChunksCount(hash string) (int, error)
	SetChunksCount(hash string, chunksCount int) error
}

func New() FileStore {
	return &dsFileStore{}
}

func (ls *dsFileStore) Init(a *app.App) (err error) {
	ls.dsIface = a.MustComponent(datastore.CName).(datastore.Datastore)
	return nil
}

func (ls *dsFileStore) Run(context.Context) (err error) {
	ds1, err := ls.dsIface.LocalstoreDS()
	if err != nil {
		return err
	}
	ls.ds = ds.New(ds1)
	return
}

func (ls *dsFileStore) Name() (name string) {
	return CName
}

type FileKeys struct {
	Hash string
	Keys map[string]string
}

func (m *dsFileStore) Prefix() string {
	return "files"
}

func (m *dsFileStore) Indexes() []localstore.Index {
	return []localstore.Index{
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

	log.Debugf("file add %s", file.Hash)
	exists, err := txn.Has(fileInfoKey)
	if err != nil {
		return err
	}
	if exists {
		return localstore.ErrDuplicateKey
	}

	b, err := proto.Marshal(file)
	if err != nil {
		return err
	}

	err = txn.Put(fileInfoKey, b)
	if err != nil {
		return err
	}

	err = localstore.AddIndexesWithTxn(m, txn, file, file.Hash)
	if err != nil {
		return err
	}

	return txn.Commit()
}

// AddMulti add multiple files and ignores possible duplicate errors, tx with all inserts discarded in case of other errors
func (m *dsFileStore) AddMulti(upsert bool, files ...*storage.FileInfo) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	for _, file := range files {
		fileInfoKey := filesInfoBase.ChildString(file.Hash)
		exists, err := txn.Has(fileInfoKey)
		if err != nil {
			return err
		}
		if exists && !upsert {
			continue
		}

		b, err := proto.Marshal(file)
		if err != nil {
			return err
		}

		err = txn.Put(fileInfoKey, b)
		if err != nil {
			return err
		}

		err = localstore.AddIndexesWithTxn(m, txn, file, file.Hash)
		if err != nil {
			return err
		}
	}

	return txn.Commit()
}

func (m *dsFileStore) AddFileKeys(fileKeys ...FileKeys) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	for _, fk := range fileKeys {
		if len(fk.Keys) == 0 {
			continue
		}
		err = m.addSingleFileKeys(txn, fk.Hash, fk.Keys)
		if err != nil {
			if err == localstore.ErrDuplicateKey {
				continue
			}
			return err
		}
	}

	return txn.Commit()
}

func (m *dsFileStore) RemoveEmpty() error {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	res, err := localstore.GetKeys(txn, filesKeysBase.String(), 0)
	if err != nil {
		return err
	}

	hashes, err := localstore.GetLeavesFromResults(res)
	if err != nil {
		return err
	}

	var removed int
	for _, hash := range hashes {
		v, err := m.GetFileKeys(hash)
		if err != nil {
			if err != nil {
				log.Errorf("RemoveEmpty failed to get keys: %s", err)
			}
			continue
		}
		if len(v) == 0 {
			removed++
			err = m.DeleteFileKeys(hash)
			if err != nil {
				log.Errorf("RemoveEmpty failed to delete empty file keys: %s", err)
			}
		}
	}
	if removed > 0 {
		log.Errorf("RemoveEmpty removed %d empty file keys", removed)
	}
	return nil
}

func (m *dsFileStore) ListFileKeys() ([]string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	res, err := localstore.GetKeys(txn, filesKeysBase.String(), 0)
	if err != nil {
		return nil, err
	}

	return localstore.ExtractKeysFromResults(res)
}

func (m *dsFileStore) DeleteFileKeys(hash string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	fileKeysKey := filesKeysBase.ChildString(hash)
	err = txn.Delete(fileKeysKey)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsFileStore) addSingleFileKeys(txn ds.Txn, hash string, keys map[string]string) error {
	fileKeysKey := filesKeysBase.ChildString(hash)

	exists, err := txn.Has(fileKeysKey)
	if err != nil {
		return err
	}
	if exists {
		return localstore.ErrDuplicateKey
	}

	b, err := proto.Marshal(&storage.FileKeys{
		KeysByPath: keys,
	})
	if err != nil {
		return err
	}

	return txn.Put(fileKeysKey, b)
}

func (m *dsFileStore) GetFileKeys(hash string) (map[string]string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	fileKeysKey := filesKeysBase.ChildString(hash)

	b, err := txn.Get(fileKeysKey)
	if err != nil {
		if err == dsCtx.ErrNotFound {
			return nil, localstore.ErrNotFound
		}
		return nil, err
	}

	var fileKeys = storage.FileKeys{}
	err = proto.Unmarshal(b, &fileKeys)
	if err != nil {
		return nil, err
	}

	return fileKeys.KeysByPath, nil
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
	err = localstore.AddIndex(indexTargets, m.ds, file, file.Hash)
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
		if err == dsCtx.ErrNotFound {
			return nil, localstore.ErrNotFound
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
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	key, err := localstore.GetKeyByIndex(indexMillChecksum, txn, &storage.FileInfo{Mill: mill, Checksum: checksum})
	if err != nil {
		return nil, err
	}

	val, err := txn.Get(filesInfoBase.ChildString(key))
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
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	key, err := localstore.GetKeyByIndex(indexMillSourceOpts, txn, &storage.FileInfo{Mill: mill, Source: source, Opts: opts})
	if err != nil {
		return nil, err
	}

	val, err := txn.Get(filesInfoBase.ChildString(key))
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

func (m *dsFileStore) ListTargets() ([]string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	targetPrefix := localstore.IndexBase.ChildString(indexTargets.Prefix).ChildString(indexTargets.Name).String()

	res, err := localstore.GetKeys(txn, targetPrefix, 0)
	if err != nil {
		return nil, err
	}

	keys, err := localstore.ExtractKeysFromResults(res)
	if err != nil {
		return nil, err
	}

	var targets = make([]string, len(keys))
	for i, key := range keys {
		target, err := localstore.CarveKeyParts(key, -2, -1)
		if err != nil {
			return nil, err
		}
		targets[i] = target
	}

	return util.UniqueStrings(targets), nil
}

func (m *dsFileStore) ListByTarget(target string) ([]*storage.FileInfo, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	results, err := localstore.GetKeysByIndexParts(txn, indexTargets.Prefix, indexTargets.Name, []string{target}, "", indexTargets.Hash, 0)
	if err != nil {
		return nil, err
	}

	keys, err := localstore.GetLeavesFromResults(results)
	if err != nil {
		return nil, err
	}

	var files []*storage.FileInfo
	for _, key := range keys {
		val, err := txn.Get(filesInfoBase.ChildString(key))
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

func (m *dsFileStore) List() ([]*storage.FileInfo, error) {
	var infos []*storage.FileInfo
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	res, err := localstore.GetKeys(txn, filesInfoBase.String(), 0)
	if err != nil {
		return nil, err
	}

	hashes, err := localstore.GetLeavesFromResults(res)
	if err != nil {
		return nil, err
	}

	for _, hash := range hashes {
		info, err := m.GetByHash(hash)
		if err != nil {
			return nil, err
		}

		infos = append(infos, info)
	}

	return infos, nil
}

func (m *dsFileStore) DeleteByHash(hash string) error {
	file, err := m.GetByHash(hash)
	if err != nil {
		return fmt.Errorf("failed to find file by hash to remove")
	}

	return m.deleteFile(file)
}

func (m *dsFileStore) deleteFile(file *storage.FileInfo) error {
	err := localstore.RemoveIndexes(m, m.ds, file, file.Hash)
	if err != nil {
		return err
	}

	fileInfoKey := filesInfoBase.ChildString(file.Hash)
	return m.ds.Delete(fileInfoKey)
}

func (m *dsFileStore) DeleteByTarget(targetHash string) error {
	files, err := m.ListByTarget(targetHash)
	if err != nil {
		return fmt.Errorf("failed to find files by target to remove: %w", err)
	}
	for _, f := range files {
		if derr := m.deleteFile(f); derr != nil {
			return fmt.Errorf("failed to delete file %s: %w", f.Hash, derr)
		}
	}
	return nil
}

func (m *dsFileStore) GetChunksCount(hash string) (int, error) {
	key := chunksCountBase.ChildString(hash)
	b, err := m.ds.Get(key)
	if err != nil {
		if err == dsCtx.ErrNotFound {
			return 0, localstore.ErrNotFound
		}
		return 0, err
	}
	val := binary.LittleEndian.Uint64(b)
	return int(val), nil
}

func (m *dsFileStore) SetChunksCount(hash string, chunksCount int) error {
	key := chunksCountBase.ChildString(hash)
	val := binary.LittleEndian.AppendUint64(nil, uint64(chunksCount))
	return m.ds.Put(key, val)
}

func (ls *dsFileStore) Close(ctx context.Context) (err error) {
	return nil
}
