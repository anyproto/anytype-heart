package filestore

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/proto"
	dsCtx "github.com/ipfs/go-datastore"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	ds "github.com/anyproto/anytype-heart/pkg/lib/datastore/noctxds"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/slice"
)

var (
	// FileInfo is stored in db key pattern:
	// /files/info/<hash>
	filesPrefix     = "files"
	filesInfoBase   = dsCtx.NewKey("/" + filesPrefix + "/info")
	filesKeysBase   = dsCtx.NewKey("/" + filesPrefix + "/keys")
	chunksCountBase = dsCtx.NewKey("/" + filesPrefix + "/chunks_count")
	syncStatusBase  = dsCtx.NewKey("/" + filesPrefix + "/sync_status")

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
	ListTargets() ([]string, error)
	ListByTarget(target string) ([]*storage.FileInfo, error)
	DeleteFile(hash string) error
	List() ([]*storage.FileInfo, error)
	RemoveEmpty() error

	GetChunksCount(hash string) (int, error)
	SetChunksCount(hash string, chunksCount int) error
	GetSyncStatus(hash string) (int, error)
	SetSyncStatus(hash string, syncStatus int) error
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
			err = m.deleteFileKeys(hash)
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

func (m *dsFileStore) deleteFileKeys(hash string) error {
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

	return lo.Uniq(targets), nil
}

func (m *dsFileStore) ListByTarget(target string) ([]*storage.FileInfo, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	files, err := m.listByTarget(target, txn)
	if err != nil {
		return nil, err
	}

	return files, nil
}

func (m *dsFileStore) listByTarget(target string, txn ds.Txn) ([]*storage.FileInfo, error) {
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

func (m *dsFileStore) DeleteFile(hash string) error {
	m.l.Lock()
	defer m.l.Unlock()

	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("create txn: %w", err)
	}
	defer txn.Discard()

	files, err := m.listByTarget(hash, txn)
	if err != nil {
		return fmt.Errorf("list files by target: %w", err)
	}

	for _, f := range files {
		// Remove indexed targets
		if err = localstore.RemoveIndexWithTxn(indexTargets, txn, f, f.Hash); err != nil {
			return fmt.Errorf("remove index: %w", err)
		}
		f.Targets = slice.Remove(f.Targets, hash)

		if len(f.Targets) == 0 {
			if derr := m.deleteFile(txn, f); derr != nil {
				return fmt.Errorf("failed to delete file %s: %w", f.Hash, derr)
			}
		} else {
			// Update targets
			raw, err := proto.Marshal(f)
			if err != nil {
				return err
			}
			err = txn.Put(filesInfoBase.ChildString(f.Hash), raw)
			if err != nil {
				return err
			}
			// Add updated targets to index
			if err = localstore.AddIndexWithTxn(indexTargets, txn, f, f.Hash); err != nil {
				return fmt.Errorf("update index: %w", err)
			}
		}
	}

	err = txn.Delete(filesKeysBase.ChildString(hash))
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsFileStore) deleteFile(txn ds.Txn, file *storage.FileInfo) error {
	err := localstore.RemoveIndexes(m, m.ds, file, file.Hash)
	if err != nil {
		return err
	}

	fileInfoKey := filesInfoBase.ChildString(file.Hash)
	return txn.Delete(fileInfoKey)
}

func (m *dsFileStore) getInt(key dsCtx.Key) (int, error) {
	val, err := m.ds.Get(key)
	if err != nil {
		if err == dsCtx.ErrNotFound {
			return 0, localstore.ErrNotFound
		}
		return 0, err
	}
	return int(binary.LittleEndian.Uint64(val)), nil
}

func (m *dsFileStore) setInt(key dsCtx.Key, val int) error {
	raw := binary.LittleEndian.AppendUint64(nil, uint64(val))
	return m.ds.Put(key, raw)
}

func (m *dsFileStore) GetChunksCount(hash string) (int, error) {
	key := chunksCountBase.ChildString(hash)
	return m.getInt(key)
}

func (m *dsFileStore) SetChunksCount(hash string, chunksCount int) error {
	key := chunksCountBase.ChildString(hash)
	return m.setInt(key, chunksCount)
}

func (m *dsFileStore) GetSyncStatus(hash string) (int, error) {
	key := syncStatusBase.ChildString(hash)
	return m.getInt(key)
}

func (m *dsFileStore) SetSyncStatus(hash string, status int) error {
	key := syncStatusBase.ChildString(hash)
	return m.setInt(key, status)
}

func (ls *dsFileStore) Close(ctx context.Context) (err error) {
	return nil
}
