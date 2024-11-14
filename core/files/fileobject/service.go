package fileobject

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/avast/retry-go/v4"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/peermanager"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

// TODO UNsugar
var log = logging.Logger("fileobject")

const CName = "fileobject"

type Service interface {
	app.ComponentRunnable

	InitEmptyFileState(st *state.State)
	DeleteFileData(spaceId string, objectId string) error
	Create(ctx context.Context, spaceId string, req filemodels.CreateRequest) (id string, object *types.Struct, err error)
	CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin) (string, error)
	GetFileIdFromObject(objectId string) (domain.FullFileId, error)
	GetFileIdFromObjectWaitLoad(ctx context.Context, objectId string) (domain.FullFileId, error)
	GetObjectDetailsByFileId(fileId domain.FullFileId) (string, *types.Struct, error)
	MigrateFileIdsInDetails(st *state.State, spc source.Space)
	MigrateFileIdsInBlocks(st *state.State, spc source.Space)
	MigrateFiles(st *state.State, spc source.Space, keysChanges []*pb.ChangeFileKeys)
	EnsureFileAddedToSyncQueue(id domain.FullID, details *types.Struct) error
}

type objectCreatorService interface {
	CreateSmartBlockFromStateInSpaceWithOptions(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State, opts ...objectcreator.CreateOption) (id string, newDetails *types.Struct, err error)
}

type service struct {
	spaceService    space.Service
	objectCreator   objectCreatorService
	fileService     files.Service
	fileSync        filesync.FileSync
	fileStore       filestore.FileStore
	fileOffloader   fileoffloader.Service
	objectStore     objectstore.ObjectStore
	spaceIdResolver idresolver.Resolver
	migrationQueue  *persistentqueue.Queue[*migrationItem]
	objectArchiver  objectArchiver

	indexer *indexer

	resolverRetryStartDelay time.Duration
	resolverRetryMaxDelay   time.Duration

	closeWg *sync.WaitGroup
}

func New(
	resolverRetryStartDelay time.Duration,
	resolverRetryMaxDelay time.Duration,
) Service {
	return &service{
		resolverRetryStartDelay: resolverRetryStartDelay,
		resolverRetryMaxDelay:   resolverRetryMaxDelay,
		closeWg:                 &sync.WaitGroup{},
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
	s.spaceService = app.MustComponent[space.Service](a)
	s.objectCreator = app.MustComponent[objectCreatorService](a)
	s.fileService = app.MustComponent[files.Service](a)
	s.fileSync = app.MustComponent[filesync.FileSync](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.spaceIdResolver = app.MustComponent[idresolver.Resolver](a)
	s.fileOffloader = app.MustComponent[fileoffloader.Service](a)
	s.objectArchiver = app.MustComponent[objectArchiver](a)
	cfg := app.MustComponent[configProvider](a)

	s.indexer = s.newIndexer()

	dbProvider := app.MustComponent[datastore.Datastore](a)
	db, err := dbProvider.LocalStorage()
	if err != nil {
		return fmt.Errorf("get badger: %w", err)
	}

	migrationQueueCtx := context.Background()
	if cfg.IsLocalOnlyMode() {
		migrationQueueCtx = context.WithValue(migrationQueueCtx, peermanager.ContextPeerFindDeadlineKey, time.Now().Add(1*time.Minute))
	}
	s.migrationQueue = persistentqueue.New(
		persistentqueue.NewBadgerStorage(db, []byte("queue/file_migration/"), makeMigrationItem),
		log.Desugar(),
		s.migrationQueueHandler,
		persistentqueue.WithContext(migrationQueueCtx),
	)
	return nil
}

func (s *service) Run(_ context.Context) error {
	s.closeWg.Add(1)
	go func() {
		defer s.closeWg.Done()

		err := s.deleteMigratedFilesInNonPersonalSpaces(context.Background())
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
	return nil
}

type objectArchiver interface {
	SetListIsArchived(objectIds []string, isArchived bool) error
}

func (s *service) deleteMigratedFilesInNonPersonalSpaces(ctx context.Context) error {
	personalSpaceId := s.spaceService.PersonalSpaceId()

	records, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String(personalSpaceId),
			},
		},
	})
	if err != nil {
		return err
	}
	if len(records) > 0 {
		ids := make([]string, 0, len(records))
		for _, record := range records {
			ids = append(ids, pbtypes.GetString(record.Details, bundle.RelationKeyId.String()))
		}
		if err = s.objectArchiver.SetListIsArchived(ids, true); err != nil {
			return err
		}
	}

	return nil
}

// After migrating to new sync queue we need to ensure that all not synced files are added to the queue
func (s *service) ensureNotSyncedFilesAddedToQueue() error {
	records, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyFileBackupStatus.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Int64(int64(filesyncstatus.Synced)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query file objects: %w", err)
	}

	for _, record := range records {
		fullId := extractFullFileIdFromDetails(record.Details)
		id := pbtypes.GetString(record.Details, bundle.RelationKeyId.String())
		err := s.addToSyncQueue(id, fullId, false, false)
		if err != nil {
			log.Errorf("add to sync queue: %v", err)
		}
	}

	return nil
}

func extractFullFileIdFromDetails(details *types.Struct) domain.FullFileId {
	return domain.FullFileId{
		SpaceId: pbtypes.GetString(details, bundle.RelationKeySpaceId.String()),
		FileId:  domain.FileId(pbtypes.GetString(details, bundle.RelationKeyFileId.String())),
	}
}

// EnsureFileAddedToSyncQueue adds file to sync queue if it is not synced yet, we need to do this
// after migrating to new sync queue
func (s *service) EnsureFileAddedToSyncQueue(id domain.FullID, details *types.Struct) error {
	if pbtypes.GetInt64(details, bundle.RelationKeyFileBackupStatus.String()) == int64(filesyncstatus.Synced) {
		return nil
	}
	fullId := domain.FullFileId{
		SpaceId: id.SpaceID,
		FileId:  domain.FileId(pbtypes.GetString(details, bundle.RelationKeyFileId.String())),
	}
	err := s.addToSyncQueue(id.ObjectID, fullId, false, false)
	return err
}

func (s *service) Close(ctx context.Context) error {
	s.closeWg.Wait()
	err := s.migrationQueue.Close()
	return errors.Join(err, s.indexer.close())
}

func (s *service) InitEmptyFileState(st *state.State) {
	template.InitTemplate(st,
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithFeaturedRelations,
		template.WithAllBlocksEditsRestricted,
	)
}

func (s *service) Create(ctx context.Context, spaceId string, req filemodels.CreateRequest) (id string, object *types.Struct, err error) {
	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}

	id, object, err = s.createInSpace(ctx, space, req)
	if err != nil {
		return "", nil, fmt.Errorf("create in space: %w", err)
	}
	err = s.addToSyncQueue(id, domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId}, true, req.ObjectOrigin.IsImported())
	if err != nil {
		return "", nil, fmt.Errorf("add to sync queue: %w", err)
	}

	return id, object, nil
}

func (s *service) createInSpace(ctx context.Context, space clientspace.Space, req filemodels.CreateRequest) (id string, object *types.Struct, err error) {
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
		s.InitEmptyFileState(createState)
		fullFileId := domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId}
		fullObjectId := domain.FullID{SpaceID: space.Id(), ObjectID: payload.RootRawChange.Id}

		err = s.indexer.injectMetadataToState(ctx, createState, fullFileId, fullObjectId)
		if err != nil {
			return "", nil, fmt.Errorf("inject metadata to state: %w", err)
		}
	}

	if req.AdditionalDetails != nil {
		for k, v := range req.AdditionalDetails.GetFields() {
			createState.SetDetailAndBundledRelation(domain.RelationKey(k), v)
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

func (s *service) makeInitialDetails(fileId domain.FileId, origin objectorigin.ObjectOrigin, kind model.ImageKind) *types.Struct {
	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyFileId.String(): pbtypes.String(fileId.String()),
			// Use general file layout. It will be changed for proper layout after indexing
			bundle.RelationKeyLayout.String():             pbtypes.Int64(int64(model.ObjectType_file)),
			bundle.RelationKeyFileIndexingStatus.String(): pbtypes.Int64(int64(model.FileIndexingStatus_NotIndexed)),
			bundle.RelationKeySyncStatus.String():         pbtypes.Int64(int64(domain.ObjectSyncStatusQueued)),
			bundle.RelationKeySyncError.String():          pbtypes.Int64(int64(domain.SyncErrorNull)),
			bundle.RelationKeyFileBackupStatus.String():   pbtypes.Int64(int64(filesyncstatus.Queued)),
		},
	}
	origin.AddToDetails(details)
	if kind == model.ImageKind_Basic {
		return details
	}
	details.Fields[bundle.RelationKeyImageKind.String()] = pbtypes.Int64(int64(kind))
	if kind == model.ImageKind_AutomaticallyAdded {
		details.Fields[bundle.RelationKeyIsHiddenDiscovery.String()] = pbtypes.Bool(true)
	}
	return details
}

// CreateFromImport creates file object from imported raw IPFS file. Encryption keys for this file should exist in file store.
func (s *service) CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin) (string, error) {
	// Check that fileId is not a file object id
	recs, _, err := s.objectStore.SpaceIndex(fileId.SpaceId).QueryObjectIds(database.Query{
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

func (s *service) addToSyncQueue(objectId string, fileId domain.FullFileId, uploadedByUser bool, imported bool) error {
	if err := s.fileSync.AddFile(objectId, fileId, uploadedByUser, imported); err != nil {
		return fmt.Errorf("add file to sync queue: %w", err)
	}
	return nil
}

func (s *service) GetObjectDetailsByFileId(fileId domain.FullFileId) (string, *types.Struct, error) {
	records, err := s.objectStore.SpaceIndex(fileId.SpaceId).Query(database.Query{
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
		return "", nil, filemodels.ErrObjectNotFound
	}
	details := records[0].Details
	return pbtypes.GetString(details, bundle.RelationKeyId.String()), details, nil
}

func (s *service) GetFileIdFromObject(objectId string) (domain.FullFileId, error) {
	spaceId, err := s.spaceIdResolver.ResolveSpaceID(objectId)
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("resolve space id: %w", err)
	}
	details, err := s.objectStore.SpaceIndex(spaceId).GetDetails(objectId)
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("get object details: %w", err)
	}
	fileId := pbtypes.GetString(details.Details, bundle.RelationKeyFileId.String())
	if fileId == "" {
		return domain.FullFileId{}, filemodels.ErrEmptyFileId
	}
	return domain.FullFileId{
		SpaceId: spaceId,
		FileId:  domain.FileId(fileId),
	}, nil
}

func (s *service) GetFileIdFromObjectWaitLoad(ctx context.Context, objectId string) (domain.FullFileId, error) {
	spaceId, err := s.resolveSpaceIdWithRetry(ctx, objectId)
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("resolve space id: %w", err)
	}
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("get space: %w", err)
	}
	id := domain.FullFileId{
		SpaceId: spaceId,
	}
	err = spc.Do(objectId, func(sb smartblock.SmartBlock) error {
		details := sb.Details()
		id.FileId = domain.FileId(pbtypes.GetString(details, bundle.RelationKeyFileId.String()))
		if id.FileId == "" {
			return filemodels.ErrEmptyFileId
		}
		return nil
	})
	if err != nil {
		return domain.FullFileId{}, fmt.Errorf("get object details: %w", err)
	}
	return id, nil
}

func (s *service) resolveSpaceIdWithRetry(ctx context.Context, objectId string) (string, error) {
	_, err := cid.Decode(objectId)
	if err != nil {
		return "", fmt.Errorf("decode object id: %w", err)
	}
	if domain.IsFileId(objectId) {
		return "", fmt.Errorf("object id is file cid")
	}

	spaceId, err := retry.DoWithData(func() (string, error) {
		return s.spaceIdResolver.ResolveSpaceID(objectId)
	},
		retry.Context(ctx),
		retry.Attempts(0),
		retry.Delay(s.resolverRetryStartDelay),
		retry.MaxDelay(s.resolverRetryMaxDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
	return spaceId, err
}

func (s *service) DeleteFileData(spaceId string, objectId string) error {
	fullId, err := s.GetFileIdFromObject(objectId)
	if err != nil {
		return fmt.Errorf("get file id from object: %w", err)
	}
	records, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String(objectId),
			},
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fullId.FileId.String()),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("list objects that use file id: %w", err)
	}
	if len(records) == 0 {
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
