package fileobject

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// TODO UNsugar
var log = logging.Logger("fileobject")

var ErrObjectNotFound = fmt.Errorf("file object not found")

const CName = "fileobject"

type Service interface {
	app.ComponentRunnable

	DeleteFileData(objectId string) error
	Create(ctx context.Context, spaceId string, req CreateRequest) (id string, object *types.Struct, err error)
	CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin) (string, error)
	GetFileIdFromObject(objectId string) (domain.FullFileId, error)
	GetObjectDetailsByFileId(fileId domain.FullFileId) (string, *types.Struct, error)
	MigrateDetails(st *state.State, spc source.Space, keys []*pb.ChangeFileKeys)
	MigrateBlocks(st *state.State, spc source.Space, keys []*pb.ChangeFileKeys)

	FileOffload(ctx context.Context, objectId string, includeNotPinned bool) (totalSize uint64, err error)
	FilesOffload(ctx context.Context, objectIds []string, includeNotPinned bool) (filesOffloaded int, totalSize uint64, err error)
	FileSpaceOffload(ctx context.Context, spaceId string, includeNotPinned bool) (filesOffloaded int, totalSize uint64, err error)
}

type objectCreatorService interface {
	CreateSmartBlockFromStateInSpace(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
}

type service struct {
	spaceService  space.Service
	objectCreator objectCreatorService
	fileService   files.Service
	fileSync      filesync.FileSync
	fileStore     filestore.FileStore
	objectStore   objectstore.ObjectStore

	indexer *indexer
}

func New() Service {
	return &service{}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.spaceService = app.MustComponent[space.Service](a)
	s.objectCreator = app.MustComponent[objectCreatorService](a)
	s.fileService = app.MustComponent[files.Service](a)
	s.fileSync = app.MustComponent[filesync.FileSync](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.indexer = s.newIndexer()
	return nil
}

func (s *service) Run(_ context.Context) error {
	s.indexer.run()
	return nil
}

func (s *service) Close(ctx context.Context) error {
	return s.indexer.close()
}

type CreateRequest struct {
	FileId            domain.FileId
	EncryptionKeys    map[string]string
	ObjectOrigin      objectorigin.ObjectOrigin
	AdditionalDetails *types.Struct
}

func (s *service) Create(ctx context.Context, spaceId string, req CreateRequest) (id string, object *types.Struct, err error) {
	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}

	id, object, err = s.createInSpace(ctx, space, req)
	if err != nil {
		return "", nil, fmt.Errorf("create in space: %w", err)
	}
	err = s.addToSyncQueue(domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId}, true, req.ObjectOrigin.IsImported())
	if err != nil {
		return "", nil, fmt.Errorf("add to sync queue: %w", err)
	}

	return id, object, nil
}

func (s *service) createInSpace(ctx context.Context, space clientspace.Space, req CreateRequest) (id string, object *types.Struct, err error) {
	if req.FileId == "" {
		return "", nil, fmt.Errorf("file hash is empty")
	}

	details := s.makeInitialDetails(req.FileId, req.ObjectOrigin)
	if req.AdditionalDetails != nil {
		for k, v := range req.AdditionalDetails.GetFields() {
			if _, ok := details.Fields[k]; !ok {
				details.Fields[k] = pbtypes.CopyVal(v)
			}
		}
	}

	createState := state.NewDoc("", nil).(*state.State)
	createState.SetDetails(details)
	createState.SetFileInfo(state.FileInfo{
		FileId:         req.FileId,
		EncryptionKeys: req.EncryptionKeys,
	})

	// Type will be changed after indexing, just use general type File for now
	id, object, err = s.objectCreator.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyFile}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create object: %w", err)
	}

	err = s.indexer.addToQueue(ctx, domain.FullID{SpaceID: space.Id(), ObjectID: id}, domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId})
	if err != nil {
		// Will be retried in background, so don't return error
		log.Errorf("add to index queue: %v", err)
		err = nil
	}

	return id, object, nil
}

func (s *service) migrateDeriveObject(ctx context.Context, space clientspace.Space, req CreateRequest, uniqueKey domain.UniqueKey) (err error) {
	if req.FileId == "" {
		return fmt.Errorf("file hash is empty")
	}
	details := s.makeInitialDetails(req.FileId, req.ObjectOrigin)
	details.Fields[bundle.RelationKeyFileBackupStatus.String()] = pbtypes.Int64(int64(syncstatus.StatusSynced))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(details)
	createState.SetFileInfo(state.FileInfo{
		FileId:         req.FileId,
		EncryptionKeys: req.EncryptionKeys,
	})

	// Type will be changed after indexing, just use general type File for now
	id, _, err := s.objectCreator.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyFile}, createState)
	if errors.Is(err, treestorage.ErrTreeExists) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}
	err = s.indexer.addToQueue(ctx, domain.FullID{SpaceID: space.Id(), ObjectID: id}, domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId})
	if err != nil {
		// Will be retried in background, so don't return error
		log.Errorf("add to index queue: %v", err)
		err = nil
	}
	return err
}

func (s *service) makeInitialDetails(fileId domain.FileId, origin objectorigin.ObjectOrigin) *types.Struct {
	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyFileId.String(): pbtypes.String(fileId.String()),
			// Use general file layout. It will be changed for proper layout after indexing
			bundle.RelationKeyLayout.String():             pbtypes.Int64(int64(model.ObjectType_file)),
			bundle.RelationKeyFileIndexingStatus.String(): pbtypes.Int64(int64(model.FileIndexingStatus_NotIndexed)),
		},
	}
	origin.AddToDetails(details)
	return details
}

// CreateFromImport creates file object from imported raw IPFS file. Encryption keys for this file should exist in file store.
func (s *service) CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin) (string, error) {
	// Check that fileId is not a file object id
	recs, _, err := s.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId.FileId.String()),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId.SpaceId),
			},
		},
	})
	if err == nil && len(recs) > 0 {
		return recs[0], nil
	}

	fileObjectId, _, err := s.GetObjectDetailsByFileId(fileId)
	if err == nil {
		return fileObjectId, nil
	}
	keys, err := s.fileStore.GetFileKeys(fileId.FileId)
	if err != nil {
		return "", fmt.Errorf("get file keys: %w", err)
	}
	fileObjectId, _, err = s.Create(context.Background(), fileId.SpaceId, CreateRequest{
		FileId:         fileId.FileId,
		EncryptionKeys: keys,
		ObjectOrigin:   origin,
	})
	if err != nil {
		return "", fmt.Errorf("create object: %w", err)
	}
	return fileObjectId, nil
}

func (s *service) addToSyncQueue(id domain.FullFileId, uploadedByUser bool, imported bool) error {
	if err := s.fileSync.AddFile(id.SpaceId, id.FileId, uploadedByUser, imported); err != nil {
		return fmt.Errorf("add file to sync queue: %w", err)
	}
	// TODO Maybe we need a watcher here?
	return nil
}

func (s *service) GetObjectIdByFileId(fileId domain.FullFileId) (string, error) {
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId.FileId.String()),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId.SpaceId),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("query objects by file hash: %w", err)
	}
	if len(records) == 0 {
		return "", ErrObjectNotFound
	}
	return pbtypes.GetString(records[0].Details, bundle.RelationKeyId.String()), nil
}

func (s *service) GetObjectDetailsByFileId(fileId domain.FullFileId) (string, *types.Struct, error) {
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId.FileId.String()),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId.SpaceId),
			},
		},
	})
	if err != nil {
		return "", nil, fmt.Errorf("query objects by file hash: %w", err)
	}
	if len(records) == 0 {
		return "", nil, ErrObjectNotFound
	}
	details := records[0].Details
	return pbtypes.GetString(details, bundle.RelationKeyId.String()), details, nil
}

func (s *service) GetFileIdFromObject(objectId string) (domain.FullFileId, error) {
	details, err := s.objectStore.GetDetails(objectId)
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("get object details: %w", err)
	}
	spaceId := pbtypes.GetString(details.Details, bundle.RelationKeySpaceId.String())
	fileId := pbtypes.GetString(details.Details, bundle.RelationKeyFileId.String())
	if fileId == "" {
		return domain.FullFileId{}, fmt.Errorf("empty file hash")
	}
	return domain.FullFileId{
		SpaceId: spaceId,
		FileId:  domain.FileId(fileId),
	}, nil
}

func (s *service) migrate(space clientspace.Space, objectId string, keys []*pb.ChangeFileKeys, fileId string, origin objectorigin.ObjectOrigin) string {
	// Don't migrate empty or its own id
	if fileId == "" || objectId == fileId {
		return fileId
	}
	if !domain.IsFileId(fileId) {
		return fileId
	}
	var fileKeys map[string]string
	for _, k := range keys {
		if k.Hash == fileId {
			fileKeys = k.Keys
		}
	}
	err := space.Do(fileId, func(sb smartblock.SmartBlock) error {
		return nil
	})
	// Already migrated or it is a link to object
	if err == nil {
		return fileId
	}

	fileObjectId, err := s.GetObjectIdByFileId(domain.FullFileId{
		SpaceId: space.Id(),
		FileId:  domain.FileId(fileId),
	})
	if err == nil {
		return fileObjectId
	}

	// If due to some reason fileId is a file object id from another space, we should not migrate it
	// This is definitely a bug, but we should not break things further.
	exists, err := s.isFileExistInAnotherSpace(space.Id(), fileId)
	if err != nil {
		log.Errorf("checking that file exist in another space: %v", err)
		return fileId
	}
	if exists {
		log.With("fileObjectId", fileId).Error("found file object in another space")
		return fileId
	}

	if len(fileKeys) == 0 {
		log.Warnf("no encryption keys for fileId %s", fileId)
	}
	// Add fileId as uniqueKey to avoid migration of the same file
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeFileObject, fileId)
	if err != nil {
		return fileId
	}
	fileObjectId, err = space.DeriveObjectIdWithAccountSignature(context.Background(), uniqueKey)
	if err != nil {
		log.Errorf("can't derive object id for fileId %s: %v", fileId, err)
		return fileId
	}

	storedOrigin, err := s.fileStore.GetFileOrigin(domain.FileId(fileId))
	if err == nil {
		origin = storedOrigin
	}
	err = s.migrateDeriveObject(context.Background(), space, CreateRequest{
		FileId:         domain.FileId(fileId),
		EncryptionKeys: fileKeys,
		ObjectOrigin:   origin,
	}, uniqueKey)
	if err != nil {
		log.Errorf("create file object for fileId %s: %v", fileId, err)
	}
	return fileObjectId
}

func (s *service) isFileExistInAnotherSpace(spaceId string, fileObjectId string) (bool, error) {
	recs, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileObjectId),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		return false, fmt.Errorf("query objects by file hash: %w", err)
	}
	return len(recs) > 0, nil
}

func (s *service) MigrateBlocks(st *state.State, spc source.Space, keys []*pb.ChangeFileKeys) {
	origin := objectorigin.FromDetails(st.Details())
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if migrator, ok := b.(simple.FileMigrator); ok {
			migrator.MigrateFile(func(oldHash string) (newHash string) {
				return s.migrate(spc.(clientspace.Space), st.RootId(), keys, oldHash, origin)
			})
		}
		return true
	})
}

func (s *service) MigrateDetails(st *state.State, spc source.Space, keys []*pb.ChangeFileKeys) {
	origin := objectorigin.FromDetails(st.Details())
	st.ModifyLinkedFilesInDetails(func(id string) string {
		return s.migrate(spc.(clientspace.Space), st.RootId(), keys, id, origin)
	})
	st.ModifyLinkedObjectsInDetails(func(id string) string {
		return s.migrate(spc.(clientspace.Space), st.RootId(), keys, id, origin)
	})
}

func (s *service) FileOffload(ctx context.Context, objectId string, includeNotPinned bool) (totalSize uint64, err error) {
	details, err := s.objectStore.GetDetails(objectId)
	if err != nil {
		return 0, fmt.Errorf("get object details: %w", err)
	}
	return s.fileOffload(ctx, details.GetDetails(), includeNotPinned)
}

func (s *service) fileOffload(ctx context.Context, fileDetails *types.Struct, includeNotPinned bool) (uint64, error) {
	fileId := pbtypes.GetString(fileDetails, bundle.RelationKeyFileId.String())
	if fileId == "" {
		return 0, fmt.Errorf("empty file hash")
	}
	backupStatus := syncstatus.SyncStatus(pbtypes.GetInt64(fileDetails, bundle.RelationKeyFileBackupStatus.String()))
	id := domain.FullFileId{
		SpaceId: pbtypes.GetString(fileDetails, bundle.RelationKeySpaceId.String()),
		FileId:  domain.FileId(fileId),
	}

	if !includeNotPinned && backupStatus != syncstatus.StatusSynced {
		return 0, nil
	}

	return s.fileService.FileOffload(ctx, id)
}

func (s *service) FilesOffload(ctx context.Context, objectIds []string, includeNotPinned bool) (filesOffloaded int, totalSize uint64, err error) {
	if len(objectIds) == 0 {
		return s.offloadAllFiles(ctx, includeNotPinned)
	}

	for _, objectId := range objectIds {
		size, err := s.FileOffload(ctx, objectId, includeNotPinned)
		if err != nil {
			log.Errorf("failed to offload file %s: %v", objectId, err)
			continue
		}
		totalSize += size
		if size > 0 {
			filesOffloaded++
		}
	}
	return filesOffloaded, totalSize, nil
}

func (s *service) offloadAllFiles(ctx context.Context, includeNotPinned bool) (filesOffloaded int, totalSize uint64, err error) {
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
		},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("query file objects by spaceId: %w", err)
	}
	for _, record := range records {
		size, err := s.fileOffload(ctx, record.Details, includeNotPinned)
		if err != nil {
			objectId := pbtypes.GetString(record.Details, bundle.RelationKeyId.String())
			log.Errorf("failed to offload file %s: %v", objectId, err)
			continue
		}
		totalSize += size
		if size > 0 {
			filesOffloaded++
		}
	}
	return filesOffloaded, totalSize, nil
}

func (s *service) FileSpaceOffload(ctx context.Context, spaceId string, includeNotPinned bool) (filesOffloaded int, totalSize uint64, err error) {
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
		},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("query file objects by spaceId: %w", err)
	}
	for _, record := range records {
		fileId := pbtypes.GetString(record.Details, bundle.RelationKeyFileId.String())
		size, err := s.offloadFileSafe(ctx, spaceId, fileId, record, includeNotPinned)
		if err != nil {
			log.Errorf("failed to offload file %s: %v", fileId, err)
			return 0, 0, err
		}
		if size > 0 {
			filesOffloaded++
			err = s.fileStore.DeleteFile(domain.FileId(fileId))
			if err != nil {
				return 0, 0, fmt.Errorf("failed to delete file from store: %w", err)
			}
		}
		totalSize += size
	}
	return filesOffloaded, totalSize, nil
}

func (s *service) DeleteFileData(objectId string) error {
	fullId, err := s.GetFileIdFromObject(objectId)
	if err != nil {
		return fmt.Errorf("get file id from object: %w", err)
	}
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String(objectId),
			},
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("list objects that use file id: %w", err)
	}
	if len(records) == 0 {
		if err := s.fileStore.DeleteFile(fullId.FileId); err != nil {
			return err
		}
		if err := s.fileSync.RemoveFile(fullId.SpaceId, fullId.FileId); err != nil {
			return fmt.Errorf("failed to remove file from sync: %w", err)
		}
		_, err = s.FileOffload(context.Background(), objectId, true)
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (s *service) offloadFileSafe(ctx context.Context,
	spaceId string,
	fileId string,
	record database.Record,
	includeNotPinned bool,
) (uint64, error) {
	existingObjects, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		return 0, err
	}
	if len(existingObjects) > 0 {
		return s.fileOffload(ctx, record.Details, false)
	}
	return s.fileOffload(ctx, record.Details, includeNotPinned)
}
