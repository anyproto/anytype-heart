package fileobject

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/fileblocks"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/peermanager"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

// TODO UNsugar
var log = logging.Logger("fileobject")

const CName = "fileobject"

type Service interface {
	app.ComponentRunnable

	InitEmptyFileState(st *state.State)
	CanDeleteFile(ctx context.Context, objectId string) error
	DeleteFileData(spaceId string, objectId string) error
	Create(ctx context.Context, spaceId string, req filemodels.CreateRequest) (id string, object *domain.Details, err error)
	CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin) (string, error)
	GetFileIdFromObject(objectId string) (domain.FullFileId, error)

	DoFileWaitLoad(ctx context.Context, objectId string, proc func(object fileobject.FileObject) error) error
	GetFileData(ctx context.Context, objectId string) (files.File, error)
	GetImageData(ctx context.Context, objectId string) (files.Image, error)

	GetImageDataFromRawId(ctx context.Context, fileId domain.FileId) (files.Image, error)

	GetObjectDetailsByFileId(fileId domain.FullFileId) (string, *domain.Details, error)

	MigrateFileIdsInDetails(st *state.State, spc source.Space)
	MigrateFileIdsInBlocks(st *state.State, spc source.Space)
	MigrateFiles(st *state.State, spc source.Space, keysChanges []*pb.ChangeFileKeys)
	EnsureFileAddedToSyncQueue(id domain.FullID, details *domain.Details) error
}

type objectCreatorService interface {
	CreateSmartBlockFromStateInSpaceWithOptions(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State, opts ...objectcreator.CreateOption) (id string, newDetails *domain.Details, err error)
}

type service struct {
	spaceService    space.Service
	objectCreator   objectCreatorService
	fileService     files.Service
	fileSync        filesync.FileSync
	fileOffloader   fileoffloader.Service
	objectStore     objectstore.ObjectStore
	spaceIdResolver idresolver.Resolver
	migrationQueue  *persistentqueue.Queue[*migrationItem]
	accountService  accountService
	objectArchiver  objectArchiver
	formatFetcher   relationutils.RelationFormatFetcher

	indexMigrationChan chan *indexMigrationItem

	indexer *indexer

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
	closeWg            *sync.WaitGroup
}

func New() Service {
	return &service{
		indexMigrationChan: make(chan *indexMigrationItem),
		closeWg:            &sync.WaitGroup{},
	}
}

func (s *service) Name() string {
	return CName
}

type configProvider interface {
	IsLocalOnlyMode() bool
}

var _ configProvider = (*config.Config)(nil)

func (s *service) Init(a *app.App) error {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	s.spaceService = app.MustComponent[space.Service](a)
	s.objectCreator = app.MustComponent[objectCreatorService](a)
	s.fileService = app.MustComponent[files.Service](a)
	s.fileSync = app.MustComponent[filesync.FileSync](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.spaceIdResolver = app.MustComponent[idresolver.Resolver](a)
	s.fileOffloader = app.MustComponent[fileoffloader.Service](a)
	s.objectArchiver = app.MustComponent[objectArchiver](a)
	s.accountService = app.MustComponent[accountService](a)
	s.formatFetcher = app.MustComponent[relationutils.RelationFormatFetcher](a)

	provider := app.MustComponent[anystoreprovider.Provider](a)

	cfg := app.MustComponent[configProvider](a)

	s.indexer = s.newIndexer()

	migrationQueueCtx := context.Background()
	if cfg.IsLocalOnlyMode() {
		migrationQueueCtx = context.WithValue(migrationQueueCtx, peermanager.ContextPeerFindDeadlineKey, time.Now().Add(1*time.Minute))
	}

	migrationQueueStore, err := persistentqueue.NewAnystoreStorage(provider.GetCommonDb(), "queue/file_migration", makeMigrationItem)
	if err != nil {
		return fmt.Errorf("init migration queue store: %w", err)
	}

	s.migrationQueue = persistentqueue.New(
		migrationQueueStore,
		log.Desugar(),
		s.migrationQueueHandler,
		nil,
		persistentqueue.WithContext(migrationQueueCtx),
	)
	return nil
}

func (s *service) Run(_ context.Context) error {
	s.closeWg.Add(1)
	go func() {
		defer s.closeWg.Done()

		err := s.deleteMigratedFilesInNonPersonalSpaces(s.componentCtx)
		if err != nil {
			log.Errorf("delete migrated files in non personal spaces: %v", err)
		}
		err = s.ensureNotSyncedFilesAddedToQueue()
		if err != nil {
			log.Errorf("ensure not synced files added to queue: %v", err)
		}
	}()
	s.indexer.run()
	s.migrationQueue.Run()

	err := s.startIndexMigration()
	if err != nil {
		return fmt.Errorf("start index migration: %w", err)
	}
	return nil
}

type objectArchiver interface {
	SetListIsArchived(ctx context.Context, objectIds []string, isArchived bool) error
}

func (s *service) deleteMigratedFilesInNonPersonalSpaces(ctx context.Context) error {
	personalSpaceId := s.spaceService.PersonalSpaceId()

	records, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileId,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyUniqueKey,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeySpaceId,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.String(personalSpaceId),
			},
		},
	})
	if err != nil {
		return err
	}
	if len(records) > 0 {
		ids := make([]string, 0, len(records))
		for _, record := range records {
			ids = append(ids, record.Details.GetString(bundle.RelationKeyId))
		}
		if err = s.objectArchiver.SetListIsArchived(ctx, ids, true); err != nil {
			return err
		}
	}

	return nil
}

// After migrating to new sync queue we need to ensure that all not synced files are added to the queue
func (s *service) ensureNotSyncedFilesAddedToQueue() error {
	records, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileId,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyFileBackupStatus,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Int64(int64(filesyncstatus.Synced)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query file objects: %w", err)
	}

	for _, record := range records {
		fullId := extractFullFileIdFromDetails(record.Details)
		if record.Details.GetString(bundle.RelationKeyCreator) == s.accountService.MyParticipantId(fullId.SpaceId) {
			id := record.Details.GetString(bundle.RelationKeyId)
			req := filesync.AddFileRequest{
				FileObjectId:   id,
				FileId:         fullId,
				UploadedByUser: false,
				Imported:       false,
			}
			err := s.addToSyncQueue(req)
			if err != nil {
				log.Errorf("add to sync queue: %v", err)
			}
		}
	}

	return nil
}

func extractFullFileIdFromDetails(details *domain.Details) domain.FullFileId {
	return domain.FullFileId{
		SpaceId: details.GetString(bundle.RelationKeySpaceId),
		FileId:  domain.FileId(details.GetString(bundle.RelationKeyFileId)),
	}
}

// EnsureFileAddedToSyncQueue adds file to sync queue if it is not synced yet, we need to do this
// after migrating to new sync queue
func (s *service) EnsureFileAddedToSyncQueue(id domain.FullID, details *domain.Details) error {
	if details.GetInt64(bundle.RelationKeyFileBackupStatus) == int64(filesyncstatus.Synced) {
		return nil
	}
	fullId := domain.FullFileId{
		SpaceId: id.SpaceID,
		FileId:  domain.FileId(details.GetString(bundle.RelationKeyFileId)),
	}
	req := filesync.AddFileRequest{
		FileObjectId:   id.ObjectID,
		FileId:         fullId,
		UploadedByUser: false,
		Imported:       false,
	}
	err := s.addToSyncQueue(req)
	return err
}

func (s *service) Close(ctx context.Context) error {
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}
	s.closeWg.Wait()
	err := s.migrationQueue.Close()
	return errors.Join(err, s.indexer.close())
}

type CreateRequest struct {
	FileId                domain.FileId
	EncryptionKeys        map[string]string
	ObjectOrigin          objectorigin.ObjectOrigin
	AdditionalDetails     *domain.Details
	AsyncMetadataIndexing bool
}

func (s *service) InitEmptyFileState(st *state.State) {
	fileblocks.InitEmptyFileState(st)
}

type imageVariant struct {
	variantId domain.FileId
	size      int64
}

func (s *service) Create(ctx context.Context, spaceId string, req filemodels.CreateRequest) (id string, object *domain.Details, err error) {
	if !req.AsyncMetadataIndexing && len(req.FileVariants) == 0 {
		return "", nil, fmt.Errorf("file variants are not provided")
	}

	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}

	id, object, err = s.createInSpace(ctx, space, req)
	if err != nil {
		return "", nil, fmt.Errorf("create in space: %w", err)
	}

	added, err := s.addImageToSyncQueue(req, id, space.Id())
	if err != nil {
		return "", nil, fmt.Errorf("add image variants to sync queue: %w", err)
	}
	if added {
		return id, object, nil
	}

	syncReq := filesync.AddFileRequest{
		FileObjectId:   id,
		FileId:         domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId},
		UploadedByUser: true,
		Imported:       req.ObjectOrigin.IsImported(),
	}
	err = s.addToSyncQueue(syncReq)
	if err != nil {
		return "", nil, fmt.Errorf("add to sync queue: %w", err)
	}

	return id, object, nil
}

func (s *service) addImageToSyncQueue(req filemodels.CreateRequest, id string, spaceId string) (bool, error) {
	var imageVariants []imageVariant
	for _, variant := range req.FileVariants {
		if variant.Mill == mill.ImageResizeId {
			imageVariants = append(imageVariants, imageVariant{
				variantId: domain.FileId(variant.Hash),
				size:      variant.Size_,
			})
		}
	}
	if len(imageVariants) == 0 {
		return false, nil
	}
	sort.Slice(imageVariants, func(i, j int) bool {
		return imageVariants[i].size < imageVariants[j].size
	})
	variants := make([]domain.FileId, 0, len(imageVariants))
	for _, variant := range imageVariants {
		variants = append(variants, variant.variantId)
	}
	err := s.addToSyncQueue(filesync.AddFileRequest{
		FileObjectId:   id,
		FileId:         domain.FullFileId{SpaceId: spaceId, FileId: req.FileId},
		UploadedByUser: true,
		Imported:       req.ObjectOrigin.IsImported(),
		Variants:       variants,
	})
	if err != nil {
		return false, fmt.Errorf("add image variant to sync queue: %w", err)
	}
	return true, nil
}

func (s *service) createInSpace(ctx context.Context, space clientspace.Space, req filemodels.CreateRequest) (id string, object *domain.Details, err error) {
	if req.FileId == "" {
		return "", nil, fmt.Errorf("file hash is empty")
	}

	details := s.makeInitialDetails(req.FileId, req.ObjectOrigin, req.ImageKind)

	payload, err := space.CreateTreePayload(ctx, payloadcreator.PayloadCreationParams{
		Time:           time.Now(),
		SmartblockType: coresb.SmartBlockTypeFileObject,
	})
	if err != nil {
		return "", nil, fmt.Errorf("create tree payload: %w", err)
	}

	createState := state.NewDoc(payload.RootRawChange.Id, nil).(*state.State)
	createState.SetDetails(details)
	createState.SetFileInfo(state.FileInfo{
		FileId:         req.FileId,
		EncryptionKeys: req.EncryptionKeys,
	})
	if !req.AsyncMetadataIndexing {
		fileblocks.InitEmptyFileState(createState)
		fullFileId := domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId}
		fullObjectId := domain.FullID{SpaceID: space.Id(), ObjectID: payload.RootRawChange.Id}

		err = s.indexer.injectMetadataToState(ctx, createState, req.FileVariants, fullFileId, fullObjectId)
		if err != nil {
			return "", nil, fmt.Errorf("inject metadata to state: %w", err)
		}
	}

	if req.AdditionalDetails != nil {
		for k, v := range req.AdditionalDetails.Iterate() {
			createState.SetDetailAndBundledRelation(k, v)
		}
	}

	// Type will be changed after indexing, just use general type File for now
	id, object, err = s.objectCreator.CreateSmartBlockFromStateInSpaceWithOptions(ctx, space, []domain.TypeKey{bundle.TypeKeyFile}, createState, objectcreator.WithPayload(&payload))
	if err != nil {
		return "", nil, fmt.Errorf("create object: %w", err)
	}

	if req.AsyncMetadataIndexing {
		err = s.indexer.addToQueue(ctx, domain.FullID{SpaceID: space.Id(), ObjectID: id}, domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId})
		if err != nil {
			// Will be retried in background, so don't return error
			log.Errorf("add to index queue: %v", err)
		}
	}

	return id, object, nil
}

func (s *service) makeInitialDetails(fileId domain.FileId, origin objectorigin.ObjectOrigin, kind model.ImageKind) *domain.Details {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyFileId, fileId.String())
	// Use general file layout. It will be changed for proper layout after indexing
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_file))
	details.SetInt64(bundle.RelationKeyFileIndexingStatus, int64(model.FileIndexingStatus_NotIndexed))
	details.SetInt64(bundle.RelationKeySyncStatus, int64(domain.ObjectSyncStatusQueued))
	details.SetInt64(bundle.RelationKeySyncError, int64(domain.SyncErrorNull))
	details.SetInt64(bundle.RelationKeyFileBackupStatus, int64(filesyncstatus.Queued))
	origin.AddToDetails(details)
	if kind == model.ImageKind_Basic {
		return details
	}
	details.SetInt64(bundle.RelationKeyImageKind, int64(kind))
	if kind == model.ImageKind_AutomaticallyAdded {
		details.SetBool(bundle.RelationKeyIsHiddenDiscovery, true)
	}
	return details
}

// CreateFromImport creates file object from imported raw IPFS file. Encryption keys for this file should exist in file store.
func (s *service) CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin) (string, error) {
	// Check that fileId is not a file object id
	recs, _, err := s.objectStore.SpaceIndex(fileId.SpaceId).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(fileId.FileId.String()),
			},
			{
				RelationKey: bundle.RelationKeySpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(fileId.SpaceId),
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
	keys, err := s.objectStore.GetFileKeys(fileId.FileId)
	if err != nil {
		return "", fmt.Errorf("get file keys: %w", err)
	}
	fileObjectId, _, err = s.Create(context.Background(), fileId.SpaceId, filemodels.CreateRequest{
		FileId:                fileId.FileId,
		EncryptionKeys:        keys,
		ObjectOrigin:          origin,
		AsyncMetadataIndexing: true,
	})
	if err != nil {
		return "", fmt.Errorf("create object: %w", err)
	}
	return fileObjectId, nil
}

func (s *service) addToSyncQueue(req filesync.AddFileRequest) error {
	if err := s.fileSync.AddFile(req); err != nil {
		return fmt.Errorf("add file to sync queue: %w", err)
	}
	return nil
}

func (s *service) GetObjectDetailsByFileId(fileId domain.FullFileId) (string, *domain.Details, error) {
	records, err := s.objectStore.SpaceIndex(fileId.SpaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(fileId.FileId.String()),
			},
			{
				RelationKey: bundle.RelationKeySpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(fileId.SpaceId),
			},
		},
	})
	if err != nil {
		return "", nil, fmt.Errorf("query objects by file hash: %w", err)
	}
	if len(records) == 0 {
		return "", nil, filemodels.ErrObjectNotFound
	}
	details := records[0].Details
	return details.GetString(bundle.RelationKeyId), details, nil
}

func (s *service) getFileDetails(objectId string) (*domain.Details, error) {
	spaceId, err := s.spaceIdResolver.ResolveSpaceID(objectId)
	if err != nil {
		return nil, fmt.Errorf("resolve space id: %w", err)
	}
	details, err := s.objectStore.SpaceIndex(spaceId).GetDetails(objectId)
	if err != nil {
		return nil, fmt.Errorf("get object details: %w", err)
	}
	return details, nil
}

func (s *service) GetFileIdFromObject(objectId string) (domain.FullFileId, error) {
	details, err := s.getFileDetails(objectId)
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("get object details: %w", err)
	}

	fileId := details.GetString(bundle.RelationKeyFileId)
	if fileId == "" {
		return domain.FullFileId{}, filemodels.ErrEmptyFileId
	}
	return domain.FullFileId{
		SpaceId: details.GetString(bundle.RelationKeySpaceId),
		FileId:  domain.FileId(fileId),
	}, nil
}

func (s *service) DoFileWaitLoad(ctx context.Context, objectId string, proc func(object fileobject.FileObject) error) error {
	spaceId, err := s.resolveSpaceIdWithRetry(ctx, objectId)
	if err != nil {
		return fmt.Errorf("resolve space id: %w", err)
	}
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	return spc.Do(objectId, func(sb smartblock.SmartBlock) error {
		fileObj, ok := sb.(fileobject.FileObject)
		if !ok {
			return fmt.Errorf("object is not a fileobject")
		}
		id := fileObj.GetFullFileId()
		if id.FileId == "" {
			return filemodels.ErrEmptyFileId
		}
		return proc(fileObj)
	})
}

// GetFileData waits for object to load and returns its file data
func (s *service) GetFileData(ctx context.Context, objectId string) (files.File, error) {
	var file files.File
	err := s.DoFileWaitLoad(ctx, objectId, func(object fileobject.FileObject) error {
		var err error
		file, err = object.GetFile()
		return err
	})
	return file, err
}

// GetImageData waits for object to load and returns its image data
func (s *service) GetImageData(ctx context.Context, objectId string) (files.Image, error) {
	var img files.Image
	err := s.DoFileWaitLoad(ctx, objectId, func(object fileobject.FileObject) error {
		var err error
		img, err = object.GetImage()
		return err
	})
	return img, err
}

func (s *service) GetImageDataFromRawId(ctx context.Context, fileId domain.FileId) (files.Image, error) {
	keys, err := s.objectStore.GetFileKeys(fileId)
	if err != nil {
		return nil, fmt.Errorf("get file keys: %w", err)
	}

	fullId := domain.FullFileId{
		FileId: fileId,
	}
	variants, err := s.fileService.GetFileVariants(ctx, fullId, keys)
	if err != nil {
		return nil, fmt.Errorf("get file variants: %w", err)
	}
	return files.NewImage(s.fileService, fullId, variants), nil
}

func (s *service) resolveSpaceIdWithRetry(ctx context.Context, objectId string) (string, error) {
	_, err := cid.Decode(objectId)
	if err != nil {
		return "", fmt.Errorf("decode object id: %w", err)
	}
	if domain.IsFileId(objectId) {
		return "", fmt.Errorf("object id is file cid")
	}

	return s.spaceIdResolver.ResolveSpaceIdWithRetry(ctx, objectId)
}

func (s *service) CanDeleteFile(ctx context.Context, objectId string) error {
	details, err := s.getFileDetails(objectId)
	if err != nil {
		return fmt.Errorf("get file details: %w", err)
	}

	spaceId := details.GetString(bundle.RelationKeySpaceId)

	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}

	workspaceDetails, err := s.objectStore.SpaceIndex(spaceId).GetDetails(spc.DerivedIDs().Workspace)
	if err != nil {
		return fmt.Errorf("get workspace details: %w", err)
	}

	if workspaceDetails.GetInt64(bundle.RelationKeySpaceUxType) == int64(model.SpaceUxType_OneToOne) {
		myParticipantId := s.accountService.MyParticipantId(spaceId)

		if details.GetString(bundle.RelationKeyCreator) != myParticipantId {
			return fmt.Errorf("can't delete other's file")
		}
	}
	return nil
}

func (s *service) DeleteFileData(spaceId string, objectId string) error {
	fullId, err := s.GetFileIdFromObject(objectId)
	if err != nil {
		return fmt.Errorf("get file id from object: %w", err)
	}
	records, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyId,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.String(objectId),
			},
			{
				RelationKey: bundle.RelationKeyFileId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(fullId.FileId.String()),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("list objects that use file id: %w", err)
	}
	if len(records) == 0 {
		err := s.CanDeleteFile(s.componentCtx, objectId)
		if err != nil {
			return fmt.Errorf("can delete a file: %w", err)
		}

		if err := s.fileSync.DeleteFile(objectId, fullId); err != nil {
			return fmt.Errorf("failed to remove file from sync: %w", err)
		}
		_, err = s.fileOffloader.FileOffloadFullId(context.Background(), domain.FullID{SpaceID: spaceId, ObjectID: objectId}, true)
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}
