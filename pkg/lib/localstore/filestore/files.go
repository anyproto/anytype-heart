package filestore

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"
	dsCtx "github.com/ipfs/go-datastore"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	fileSizeBase    = dsCtx.NewKey("/" + filesPrefix + "/file_size")
	isImportedBase  = dsCtx.NewKey("/" + filesPrefix + "/is_imported")
	fileOrigin      = dsCtx.NewKey("/" + filesPrefix + "/origin")

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

type ChildFileId string

func (id ChildFileId) String() string {
	return string(id)
}

type FileStore interface {
	app.ComponentRunnable
	localstore.Indexable

	Add(file *storage.FileInfo) error
	AddMulti(upsert bool, files ...*storage.FileInfo) error
	GetChild(fileId ChildFileId) (*storage.FileInfo, error)
	GetChildBySource(mill string, source string, opts string) (*storage.FileInfo, error)
	GetChildByChecksum(mill string, checksum string) (*storage.FileInfo, error)
	AddChildId(target domain.FileId, childId ChildFileId) error
	ListFileIds() ([]domain.FileId, error)
	ListChildrenByFileId(fileId domain.FileId) ([]*storage.FileInfo, error)
	ListChildren() ([]*storage.FileInfo, error)

	DeleteFile(fileId domain.FileId) error

	AddFileKeys(fileKeys ...domain.FileKeys) error
	GetFileKeys(fileId domain.FileId) (map[string]string, error)
	RemoveEmptyFileKeys() error

	GetChunksCount(fileId domain.FileId) (int, error)
	SetChunksCount(fileId domain.FileId, chunksCount int) error
	IsFileImported(fileId domain.FileId) (bool, error)
	SetIsFileImported(fileId domain.FileId, isImported bool) error
	SetFileSize(fileId domain.FileId, size int) error
	GetFileSize(fileId domain.FileId) (int, error)
	SetFileOrigin(fileId domain.FileId, origin model.ObjectOrigin) error
	GetFileOrigin(fileId domain.FileId) (int, error)
}

func New() FileStore {
	return &dsFileStore{}
}

func (ls *dsFileStore) Init(a *app.App) (err error) {
	ls.dsIface = app.MustComponent[datastore.Datastore](a)
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

func (m *dsFileStore) AddFileKeys(fileKeys ...domain.FileKeys) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		for _, fk := range fileKeys {
			if len(fk.EncryptionKeys) == 0 {
				continue
			}
			err := m.addSingleFileKeys(txn, fk)
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

		fileIds, err := localstore.GetLeavesFromResults(res)
		if err != nil {
			return err
		}

		var removed int
		for _, fileId := range fileIds {
			// TODO USE TXN
			v, err := m.GetFileKeys(domain.FileId(fileId))
			if err != nil {
				if err != nil {
					log.Errorf("RemoveEmptyFileKeys failed to get keys: %s", err)
				}
				continue
			}
			if len(v) == 0 {
				removed++
				// TODO USE TXN
				err = m.deleteFileKeys(domain.FileId(fileId))
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

func (m *dsFileStore) deleteFileKeys(fileId domain.FileId) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		fileKeysKey := filesKeysBase.ChildString(fileId.String())
		return txn.Delete(fileKeysKey.Bytes())
	})
}

func (m *dsFileStore) addSingleFileKeys(txn *badger.Txn, fileKeys domain.FileKeys) error {
	fileKeysKey := filesKeysBase.ChildString(fileKeys.FileId.String())

	exists, err := badgerhelper.Has(txn, fileKeysKey.Bytes())
	if err != nil {
		return err
	}
	if exists {
		return localstore.ErrDuplicateKey
	}

	return badgerhelper.SetValueTxn(txn, fileKeysKey.Bytes(), &storage.FileKeys{
		KeysByPath: fileKeys.EncryptionKeys,
	})
}

func (m *dsFileStore) GetFileKeys(fileId domain.FileId) (map[string]string, error) {
	fileKeysKey := filesKeysBase.ChildString(fileId.String())
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

func (m *dsFileStore) AddChildId(fileId domain.FileId, childId ChildFileId) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		file, err := m.getChild(txn, childId)
		if err != nil {
			return err
		}

		for _, et := range file.Targets {
			if et == fileId.String() {
				// already exists
				return nil
			}
		}

		file.Targets = append(file.Targets, fileId.String())

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

func (m *dsFileStore) GetChild(childId ChildFileId) (*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) (*storage.FileInfo, error) {
		return m.getChild(txn, childId)
	})
}

func (m *dsFileStore) getChild(txn *badger.Txn, childId ChildFileId) (*storage.FileInfo, error) {
	fileInfoKey := filesInfoBase.ChildString(childId.String())
	file, err := badgerhelper.GetValueTxn(txn, fileInfoKey.Bytes(), unmarshalFileInfo)
	if badgerhelper.IsNotFound(err) {
		return nil, localstore.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (m *dsFileStore) GetChildByChecksum(mill string, checksum string) (*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) (*storage.FileInfo, error) {
		key, err := localstore.GetKeyByIndex(indexMillChecksum, txn, &storage.FileInfo{Mill: mill, Checksum: checksum})
		if err != nil {
			return nil, err
		}
		return badgerhelper.GetValueTxn(txn, filesInfoBase.ChildString(key).Bytes(), unmarshalFileInfo)
	})
}

func (m *dsFileStore) GetChildBySource(mill string, source string, opts string) (*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) (*storage.FileInfo, error) {
		key, err := localstore.GetKeyByIndex(indexMillSourceOpts, txn, &storage.FileInfo{Mill: mill, Source: source, Opts: opts})
		if err != nil {
			return nil, err
		}
		return badgerhelper.GetValueTxn(txn, filesInfoBase.ChildString(key).Bytes(), unmarshalFileInfo)
	})
}

func (m *dsFileStore) ListFileIds() ([]domain.FileId, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) ([]domain.FileId, error) {
		targetPrefix := localstore.IndexBase.ChildString(indexTargets.Prefix).ChildString(indexTargets.Name).String()

		keys := localstore.GetKeys(txn, targetPrefix, 0)
		fileIds := make([]domain.FileId, len(keys))
		for i, key := range keys {
			fileId, err := localstore.CarveKeyParts(key, -2, -1)
			if err != nil {
				return nil, err
			}
			fileIds[i] = domain.FileId(fileId)
		}

		return lo.Uniq(fileIds), nil
	})
}

func (m *dsFileStore) ListChildrenByFileId(fileId domain.FileId) ([]*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) ([]*storage.FileInfo, error) {
		files, err := m.listByTarget(txn, fileId)
		if err != nil {
			return nil, err
		}

		return files, nil
	})
}

func (m *dsFileStore) listByTarget(txn *badger.Txn, fileId domain.FileId) ([]*storage.FileInfo, error) {
	results, err := localstore.GetKeysByIndexParts(txn, indexTargets.Prefix, indexTargets.Name, []string{fileId.String()}, "", indexTargets.Hash, 0)
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

func (m *dsFileStore) ListChildren() ([]*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(m.db, func(txn *badger.Txn) ([]*storage.FileInfo, error) {
		keys := localstore.GetKeys(txn, filesInfoBase.String(), 0)

		childrenIds, err := localstore.GetLeavesFromResults(keys)
		if err != nil {
			return nil, err
		}

		infos := make([]*storage.FileInfo, 0, len(childrenIds))
		for _, childId := range childrenIds {
			info, err := m.getChild(txn, ChildFileId(childId))
			if err != nil {
				return nil, err
			}

			infos = append(infos, info)
		}

		return infos, nil
	})
}

func (m *dsFileStore) DeleteFile(fileId domain.FileId) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		files, err := m.listByTarget(txn, fileId)
		if err != nil {
			return fmt.Errorf("list files by target: %w", err)
		}

		for _, file := range files {
			// Remove indexed targets
			if err = localstore.RemoveIndexWithTxn(indexTargets, txn, file, file.Hash); err != nil {
				return fmt.Errorf("remove index: %w", err)
			}
			file.Targets = slice.RemoveMut(file.Targets, fileId.String())

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

		err = txn.Delete(filesKeysBase.ChildString(fileId.String()).Bytes())
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

func (m *dsFileStore) GetChunksCount(fileId domain.FileId) (int, error) {
	key := chunksCountBase.ChildString(fileId.String())
	return m.getInt(key)
}

func (m *dsFileStore) SetChunksCount(fileId domain.FileId, chunksCount int) error {
	key := chunksCountBase.ChildString(fileId.String())
	return m.setInt(key, chunksCount)
}

func (m *dsFileStore) IsFileImported(fileId domain.FileId) (bool, error) {
	key := isImportedBase.ChildString(fileId.String())
	raw, err := m.getInt(key)
	if err == localstore.ErrNotFound {
		return false, nil
	}
	return raw == 1, err
}

func (m *dsFileStore) SetIsFileImported(fileId domain.FileId, isImported bool) error {
	var raw int
	if isImported {
		raw = 1
	}
	key := isImportedBase.ChildString(fileId.String())
	return m.setInt(key, raw)
}

func (m *dsFileStore) GetFileSize(fileId domain.FileId) (int, error) {
	key := fileSizeBase.ChildString(fileId.String())
	return m.getInt(key)
}

func (m *dsFileStore) SetFileSize(fileId domain.FileId, status int) error {
	key := fileSizeBase.ChildString(fileId.String())
	return m.setInt(key, status)
}

func (ls *dsFileStore) SetFileOrigin(fileId domain.FileId, origin model.ObjectOrigin) error {
	key := fileOrigin.ChildString(fileId.String())
	return ls.setInt(key, int(origin))
}

func (ls *dsFileStore) GetFileOrigin(fileId domain.FileId) (int, error) {
	key := fileOrigin.ChildString(fileId.String())
	return ls.getInt(key)
}

func (ls *dsFileStore) Close(ctx context.Context) (err error) {
	return nil
}
