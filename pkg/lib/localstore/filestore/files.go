package filestore

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
	dsCtx "github.com/ipfs/go-datastore"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
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
	fileSizeBase    = dsCtx.NewKey("/" + filesPrefix + "/file_size")
	isImportedBase  = dsCtx.NewKey("/" + filesPrefix + "/is_imported")
	fileOrigin      = dsCtx.NewKey("/" + filesPrefix + "/origin")
	fileImportType  = dsCtx.NewKey("/" + filesPrefix + "/importType")

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

	DeleteFileVariants(variantIds []domain.FileContentId) error

	ListFileIds() ([]domain.FileId, error)
	ListFileVariants(fileId domain.FileId) ([]*storage.FileInfo, error)
	ListAllFileVariants() ([]*storage.FileInfo, error)

	DeleteFile(fileId domain.FileId) error

	AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error
	GetFileKeys(fileId domain.FileId) (map[string]string, error)

	GetChunksCount(fileId domain.FileId) (int, error)
	SetChunksCount(fileId domain.FileId, chunksCount int) error
	IsFileImported(fileId domain.FileId) (bool, error)
	SetIsFileImported(fileId domain.FileId, isImported bool) error
	SetFileSize(fileId domain.FileId, size int) error
	GetFileSize(fileId domain.FileId) (int, error)
	GetFileOrigin(fileId domain.FileId) (objectorigin.ObjectOrigin, error)
	SetFileOrigin(fileId domain.FileId, origin objectorigin.ObjectOrigin) error
}

func New() FileStore {
	return &dsFileStore{}
}

func (s *dsFileStore) Init(a *app.App) (err error) {
	s.dsIface = app.MustComponent[datastore.Datastore](a)
	return nil
}

func (s *dsFileStore) Run(context.Context) (err error) {
	s.db, err = s.dsIface.LocalStorage()
	if err != nil {
		return err
	}
	return
}

func (s *dsFileStore) Name() (name string) {
	return CName
}

func (s *dsFileStore) Prefix() string {
	return "files"
}

func (s *dsFileStore) Indexes() []localstore.Index {
	return []localstore.Index{
		indexMillChecksum,
		indexMillSourceOpts,
		indexTargets,
	}
}

func (s *dsFileStore) AddFileVariant(file *storage.FileInfo) error {
	return s.updateTxn(func(txn *badger.Txn) error {
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

		err = localstore.AddIndexesWithTxn(s, txn, file, file.Hash)
		if err != nil {
			return err
		}
		return nil
	})
}

// AddMulti add multiple files and ignores possible duplicate errors, tx with all inserts discarded in case of other errors
func (s *dsFileStore) AddFileVariants(upsert bool, files ...*storage.FileInfo) error {
	return s.updateTxn(func(txn *badger.Txn) error {
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

			err = localstore.AddIndexesWithTxn(s, txn, file, file.Hash)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *dsFileStore) AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		for _, fk := range fileKeys {
			if len(fk.EncryptionKeys) == 0 {
				continue
			}
			err := s.addSingleFileKeys(txn, fk)
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

func (s *dsFileStore) addSingleFileKeys(txn *badger.Txn, fileKeys domain.FileEncryptionKeys) error {
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

func (s *dsFileStore) GetFileKeys(fileId domain.FileId) (map[string]string, error) {
	fileKeysKey := filesKeysBase.ChildString(fileId.String())
	fileKeys, err := badgerhelper.GetValue(s.db, fileKeysKey.Bytes(), func(raw []byte) (v storage.FileKeys, err error) {
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

func (s *dsFileStore) LinkFileVariantToFile(fileId domain.FileId, childId domain.FileContentId) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		file, err := s.getVariant(txn, childId)
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

func (s *dsFileStore) getVariant(txn *badger.Txn, childId domain.FileContentId) (*storage.FileInfo, error) {
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

func (s *dsFileStore) GetFileVariantByChecksum(mill string, checksum string) (*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(s.db, func(txn *badger.Txn) (*storage.FileInfo, error) {
		key, err := localstore.GetKeyByIndex(indexMillChecksum, txn, &storage.FileInfo{Mill: mill, Checksum: checksum})
		if err != nil {
			return nil, err
		}
		return badgerhelper.GetValueTxn(txn, filesInfoBase.ChildString(key).Bytes(), unmarshalFileInfo)
	})
}

func (s *dsFileStore) GetFileVariantBySource(mill string, source string, opts string) (*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(s.db, func(txn *badger.Txn) (*storage.FileInfo, error) {
		key, err := localstore.GetKeyByIndex(indexMillSourceOpts, txn, &storage.FileInfo{Mill: mill, Source: source, Opts: opts})
		if err != nil {
			return nil, err
		}
		return badgerhelper.GetValueTxn(txn, filesInfoBase.ChildString(key).Bytes(), unmarshalFileInfo)
	})
}

func (s *dsFileStore) ListFileIds() ([]domain.FileId, error) {
	return badgerhelper.ViewTxnWithResult(s.db, func(txn *badger.Txn) ([]domain.FileId, error) {
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

func (s *dsFileStore) ListFileVariants(fileId domain.FileId) ([]*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(s.db, func(txn *badger.Txn) ([]*storage.FileInfo, error) {
		files, err := s.listByTarget(txn, fileId)
		if err != nil {
			return nil, err
		}

		return files, nil
	})
}

func (s *dsFileStore) listByTarget(txn *badger.Txn, fileId domain.FileId) ([]*storage.FileInfo, error) {
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

func (s *dsFileStore) ListAllFileVariants() ([]*storage.FileInfo, error) {
	return badgerhelper.ViewTxnWithResult(s.db, func(txn *badger.Txn) ([]*storage.FileInfo, error) {
		keys := localstore.GetKeys(txn, filesInfoBase.String(), 0)

		childrenIds, err := localstore.GetLeavesFromResults(keys)
		if err != nil {
			return nil, err
		}

		infos := make([]*storage.FileInfo, 0, len(childrenIds))
		for _, childId := range childrenIds {
			info, err := s.getVariant(txn, domain.FileContentId(childId))
			if err != nil {
				return nil, err
			}

			infos = append(infos, info)
		}

		return infos, nil
	})
}

func (s *dsFileStore) DeleteFile(fileId domain.FileId) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		files, err := s.listByTarget(txn, fileId)
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
				if derr := s.deleteFileVariant(txn, file); derr != nil {
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

func (s *dsFileStore) DeleteFileVariants(variantIds []domain.FileContentId) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		for _, variantId := range variantIds {
			variant, err := s.getVariant(txn, variantId)
			if err != nil {
				log.Errorf("delete file variant: failed to get file variant %s: %s", variantId, err)
				continue
			}
			err = s.deleteFileVariant(txn, variant)
			if err != nil {
				log.Errorf("delete file variant: %s: %s", variantId, err)
				continue
			}
		}
		return nil
	})
}

func (s *dsFileStore) deleteFileVariant(txn *badger.Txn, file *storage.FileInfo) error {
	err := localstore.RemoveIndexesWithTxn(s, txn, file, file.Hash)
	if err != nil {
		return err
	}

	fileInfoKey := filesInfoBase.ChildString(file.Hash)
	return txn.Delete(fileInfoKey.Bytes())
}

func (s *dsFileStore) GetChunksCount(fileId domain.FileId) (int, error) {
	key := chunksCountBase.ChildString(fileId.String())
	return s.getInt(key)
}

func (s *dsFileStore) SetChunksCount(fileId domain.FileId, chunksCount int) error {
	key := chunksCountBase.ChildString(fileId.String())
	return s.setInt(key, chunksCount)
}

func (s *dsFileStore) IsFileImported(fileId domain.FileId) (bool, error) {
	key := isImportedBase.ChildString(fileId.String())
	raw, err := s.getInt(key)
	if err == localstore.ErrNotFound {
		return false, nil
	}
	return raw == 1, err
}

func (s *dsFileStore) SetIsFileImported(fileId domain.FileId, isImported bool) error {
	var raw int
	if isImported {
		raw = 1
	}
	key := isImportedBase.ChildString(fileId.String())
	return s.setInt(key, raw)
}

func (s *dsFileStore) GetFileSize(fileId domain.FileId) (int, error) {
	key := fileSizeBase.ChildString(fileId.String())
	return s.getInt(key)
}

func (s *dsFileStore) SetFileSize(fileId domain.FileId, status int) error {
	key := fileSizeBase.ChildString(fileId.String())
	return s.setInt(key, status)
}

// GetFileOrigin returns object origin stored only for files created before Files-as-Objects version
func (s *dsFileStore) GetFileOrigin(fileId domain.FileId) (objectorigin.ObjectOrigin, error) {
	origin, err := s.getInt(fileOrigin.ChildString(fileId.String()))
	if err != nil {
		return objectorigin.None(), fmt.Errorf("failed to get file origin: %w", err)
	}

	// ImportType could be missing for non-import origins
	importType, err := s.getInt(fileImportType.ChildString(fileId.String()))
	if err != nil && !errors.Is(err, localstore.ErrNotFound) {
		return objectorigin.None(), fmt.Errorf("failed to get file import type: %w", err)
	}

	return objectorigin.ObjectOrigin{
		Origin:     model.ObjectOrigin(origin),
		ImportType: model.ImportType(importType),
	}, nil
}

func (s *dsFileStore) SetFileOrigin(fileId domain.FileId, origin objectorigin.ObjectOrigin) error {
	err := s.setInt(fileOrigin.ChildString(fileId.String()), int(origin.Origin))
	if err != nil {
		return fmt.Errorf("failed to set file origin: %w", err)
	}
	err = s.setInt(fileImportType.ChildString(fileId.String()), int(origin.ImportType))
	if err != nil {
		return fmt.Errorf("failed to set file import type: %w", err)
	}
	return nil
}

func (s *dsFileStore) Close(ctx context.Context) (err error) {
	return nil
}
