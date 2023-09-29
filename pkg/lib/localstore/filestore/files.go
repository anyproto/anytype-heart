package filestore

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"
	dsCtx "github.com/ipfs/go-datastore"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
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
	isImportedBase  = dsCtx.NewKey("/" + filesPrefix + "/is_imported")

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
	db      *badger.DB
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
	RemoveEmptyFileKeys() error

	GetChunksCount(hash string) (int, error)
	SetChunksCount(hash string, chunksCount int) error
	GetSyncStatus(hash string) (int, error)
	SetSyncStatus(hash string, syncStatus int) error
	IsFileImported(hash string) (bool, error)
	SetIsFileImported(hash string, isImported bool) error
}

func New() FileStore {
	return &dsFileStore{}
}

func (ls *dsFileStore) Init(a *app.App) (err error) {
	ls.dsIface = a.MustComponent(datastore.CName).(datastore.Datastore)
	return nil
}

func (ls *dsFileStore) Run(context.Context) (err error) {
	ls.db, err = ls.dsIface.LocalStorage()
	if err != nil {
		return err
	}
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
	return m.updateTxn(func(txn *badger.Txn) error {
		fileInfoKey := filesInfoBase.ChildString(file.Hash)

		log.Debugf("file add %s", file.Hash)
		exists, err := badgerhelper.Has(txn, fileInfoKey.Bytes())
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

		err = txn.Set(fileInfoKey.Bytes(), b)
		if err != nil {
			return err
		}

		err = localstore.AddIndexesWithTxn(m, txn, file, file.Hash)
		if err != nil {
			return err
		}
		return nil
	})
}

// AddMulti add multiple files and ignores possible duplicate errors, tx with all inserts discarded in case of other errors
func (m *dsFileStore) AddMulti(upsert bool, files ...*storage.FileInfo) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		for _, file := range files {
			fileInfoKey := filesInfoBase.ChildString(file.Hash)
			exists, err := badgerhelper.Has(txn, fileInfoKey.Bytes())
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

			err = txn.Set(fileInfoKey.Bytes(), b)
			if err != nil {
				return err
			}

			err = localstore.AddIndexesWithTxn(m, txn, file, file.Hash)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *dsFileStore) AddFileKeys(fileKeys ...FileKeys) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		for _, fk := range fileKeys {
			if len(fk.Keys) == 0 {
				continue
			}
			err := m.addSingleFileKeys(txn, fk.Hash, fk.Keys)
			if err != nil {
				if err == localstore.ErrDuplicateKey {
					continue
				}
				return err
			}
		}
		return nil
	})
}

func (m *dsFileStore) RemoveEmptyFileKeys() error {
	return m.updateTxn(func(txn *badger.Txn) error {
		res := localstore.GetKeys(txn, filesKeysBase.String(), 0)

		hashes, err := localstore.GetLeavesFromResults(res)
		if err != nil {
			return err
		}

		var removed int
		for _, hash := range hashes {
			// TODO USE TXN
			v, err := m.GetFileKeys(hash)
			if err != nil {
				if err != nil {
					log.Errorf("RemoveEmptyFileKeys failed to get keys: %s", err)
				}
				continue
			}
			if len(v) == 0 {
				removed++
				// TODO USE TXN
				err = m.deleteFileKeys(hash)
				if err != nil {
					log.Errorf("RemoveEmptyFileKeys failed to delete empty file keys: %s", err)
				}
			}
		}
		if removed > 0 {
			log.Errorf("RemoveEmptyFileKeys removed %d empty file keys", removed)
		}
		return nil
	})
}

func (m *dsFileStore) deleteFileKeys(hash string) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		fileKeysKey := filesKeysBase.ChildString(hash)
		return txn.Delete(fileKeysKey.Bytes())
	})
}

func (m *dsFileStore) addSingleFileKeys(txn *badger.Txn, hash string, keys map[string]string) error {
	fileKeysKey := filesKeysBase.ChildString(hash)

	exists, err := badgerhelper.Has(txn, fileKeysKey.Bytes())
	if err != nil {
		return err
	}
	if exists {
		return localstore.ErrDuplicateKey
	}

	return badgerhelper.SetValueTxn(txn, fileKeysKey.Bytes(), &storage.FileKeys{
		KeysByPath: keys,
	})
}

func (m *dsFileStore) GetFileKeys(hash string) (map[string]string, error) {
	fileKeysKey := filesKeysBase.ChildString(hash)
	fileKeys, err := badgerhelper.GetValue(m.db, fileKeysKey.Bytes(), func(raw []byte) (v storage.FileKeys, err error) {
		return v, proto.Unmarshal(raw, &v)
	})
	if badgerhelper.IsNotFound(err) {
		return nil, localstore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fileKeys.KeysByPath, nil
}

func (m *dsFileStore) AddTarget(hash string, target string) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		file, err := m.getByHash(txn, hash)
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

		fileInfoKey := filesInfoBase.ChildString(file.Hash)
		err = badgerhelper.SetValueTxn(txn, fileInfoKey.Bytes(), file)
		if err != nil {
			return fmt.Errorf("put updated file info: %w", err)
		}

		err = localstore.AddIndexWithTxn(indexTargets, txn, file, file.Hash)
		if err != nil {
			return err
		}
		return nil
	})
}

func (m *dsFileStore) GetByHash(hash string) (*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) (*storage.FileInfo, error) {
		return m.getByHash(txn, hash)
	})
}

func (m *dsFileStore) getByHash(txn *badger.Txn, hash string) (*storage.FileInfo, error) {
	fileInfoKey := filesInfoBase.ChildString(hash)
	file, err := badgerhelper.GetValueTxn(txn, fileInfoKey.Bytes(), unmarshalFileInfo)
	if badgerhelper.IsNotFound(err) {
		return nil, localstore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (m *dsFileStore) GetByChecksum(mill string, checksum string) (*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) (*storage.FileInfo, error) {
		key, err := localstore.GetKeyByIndex(indexMillChecksum, txn, &storage.FileInfo{Mill: mill, Checksum: checksum})
		if err != nil {
			return nil, err
		}
		return badgerhelper.GetValueTxn(txn, filesInfoBase.ChildString(key).Bytes(), unmarshalFileInfo)
	})
}

func (m *dsFileStore) GetBySource(mill string, source string, opts string) (*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) (*storage.FileInfo, error) {
		key, err := localstore.GetKeyByIndex(indexMillSourceOpts, txn, &storage.FileInfo{Mill: mill, Source: source, Opts: opts})
		if err != nil {
			return nil, err
		}
		return badgerhelper.GetValueTxn(txn, filesInfoBase.ChildString(key).Bytes(), unmarshalFileInfo)
	})
}

func (m *dsFileStore) ListTargets() ([]string, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) ([]string, error) {
		targetPrefix := localstore.IndexBase.ChildString(indexTargets.Prefix).ChildString(indexTargets.Name).String()

		keys := localstore.GetKeys(txn, targetPrefix, 0)
		targets := make([]string, len(keys))
		for i, key := range keys {
			target, err := localstore.CarveKeyParts(key, -2, -1)
			if err != nil {
				return nil, err
			}
			targets[i] = target
		}

		return lo.Uniq(targets), nil
	})
}

func (m *dsFileStore) ListByTarget(target string) ([]*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) ([]*storage.FileInfo, error) {
		files, err := m.listByTarget(txn, target)
		if err != nil {
			return nil, err
		}

		return files, nil
	})
}

func (m *dsFileStore) listByTarget(txn *badger.Txn, target string) ([]*storage.FileInfo, error) {
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
		file, err := badgerhelper.GetValueTxn(txn, filesInfoBase.ChildString(key).Bytes(), unmarshalFileInfo)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func (m *dsFileStore) List() ([]*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) ([]*storage.FileInfo, error) {
		keys := localstore.GetKeys(txn, filesInfoBase.String(), 0)

		hashes, err := localstore.GetLeavesFromResults(keys)
		if err != nil {
			return nil, err
		}

		infos := make([]*storage.FileInfo, 0, len(hashes))
		for _, hash := range hashes {
			info, err := m.getByHash(txn, hash)
			if err != nil {
				return nil, err
			}

			infos = append(infos, info)
		}

		return infos, nil
	})
}

func (m *dsFileStore) DeleteFile(hash string) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		files, err := m.listByTarget(txn, hash)
		if err != nil {
			return fmt.Errorf("list files by target: %w", err)
		}

		for _, file := range files {
			// Remove indexed targets
			if err = localstore.RemoveIndexWithTxn(indexTargets, txn, file, file.Hash); err != nil {
				return fmt.Errorf("remove index: %w", err)
			}
			file.Targets = slice.Remove(file.Targets, hash)

			if len(file.Targets) == 0 {
				if derr := m.deleteFile(txn, file); derr != nil {
					return fmt.Errorf("failed to delete file %s: %w", file.Hash, derr)
				}
			} else {
				// Update targets by saving the whole file structure
				err = badgerhelper.SetValueTxn(txn, filesInfoBase.ChildString(file.Hash).Bytes(), file)
				if err != nil {
					return fmt.Errorf("put updated file info: %w", err)
				}

				// Add updated targets to index
				if err = localstore.AddIndexWithTxn(indexTargets, txn, file, file.Hash); err != nil {
					return fmt.Errorf("update index: %w", err)
				}
			}
		}

		err = txn.Delete(filesKeysBase.ChildString(hash).Bytes())
		if err != nil {
			return err
		}
		return nil
	})
}

func (m *dsFileStore) deleteFile(txn *badger.Txn, file *storage.FileInfo) error {
	err := localstore.RemoveIndexesWithTxn(m, txn, file, file.Hash)
	if err != nil {
		return err
	}

	fileInfoKey := filesInfoBase.ChildString(file.Hash)
	return txn.Delete(fileInfoKey.Bytes())
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

func (m *dsFileStore) IsFileImported(hash string) (bool, error) {
	key := isImportedBase.ChildString(hash)
	raw, err := m.getInt(key)
	if err == localstore.ErrNotFound {
		return false, nil
	}
	return raw == 1, err
}

func (m *dsFileStore) SetIsFileImported(hash string, isImported bool) error {
	var raw int
	if isImported {
		raw = 1
	}
	key := isImportedBase.ChildString(hash)
	return m.setInt(key, raw)
}

func (ls *dsFileStore) Close(ctx context.Context) (err error) {
	return nil
}
