package fileobject

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/cheggaaa/mb/v3"
	format "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/fileblocks"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/space"
)

type accountService interface {
	MyParticipantId(string) string
}

type indexer struct {
	fileService    files.Service
	spaceService   space.Service
	objectStore    objectstore.ObjectStore
	accountService accountService

	query        database.Query
	indexCtx     context.Context
	indexCancel  func()
	indexQueue   *mb.MB[indexRequest]
	isQueuedLock sync.RWMutex
	isQueued     map[domain.FullID]struct{}

	closeWg *sync.WaitGroup
}

func (s *service) newIndexer() *indexer {
	ind := &indexer{
		fileService:    s.fileService,
		spaceService:   s.spaceService,
		objectStore:    s.objectStore,
		accountService: s.accountService,

		indexQueue: mb.New[indexRequest](0),
		isQueued:   make(map[domain.FullID]struct{}),

		closeWg: &sync.WaitGroup{},
	}
	ind.initQuery()
	return ind
}

func (ind *indexer) run() {
	ind.indexCtx, ind.indexCancel = context.WithCancel(context.Background())

	ind.closeWg.Add(1)
	go ind.runIndexingProvider()

	ind.closeWg.Add(1)
	go ind.runIndexingWorker()
}

func (ind *indexer) close() error {
	ind.indexCancel()
	ind.closeWg.Wait()
	return ind.indexQueue.Close()
}

type indexRequest struct {
	id     domain.FullID
	fileId domain.FullFileId
}

func (ind *indexer) addToQueue(ctx context.Context, id domain.FullID, fileId domain.FullFileId) error {
	ind.isQueuedLock.Lock()
	defer ind.isQueuedLock.Unlock()
	_, ok := ind.isQueued[id]
	if ok {
		return nil
	}
	ind.isQueued[id] = struct{}{}

	return ind.indexQueue.Add(ctx, indexRequest{id: id, fileId: fileId})
}

func (ind *indexer) markIndexingDone(id domain.FullID) {
	ind.isQueuedLock.Lock()
	defer ind.isQueuedLock.Unlock()
	delete(ind.isQueued, id)
}

func (ind *indexer) initQuery() {
	ind.query = database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value: domain.Int64List([]model.ObjectTypeLayout{
					model.ObjectType_file,
					model.ObjectType_image,
					model.ObjectType_video,
					model.ObjectType_audio,
					model.ObjectType_pdf,
				}),
			},
			{
				RelationKey: bundle.RelationKeyFileId,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyFileIndexingStatus,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Int64(int64(model.FileIndexingStatus_Indexed)),
			},
		},
	}
}

func (ind *indexer) addToQueueFromObjectStore(ctx context.Context) error {
	recs, err := ind.objectStore.QueryCrossSpace(ind.query)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	for _, rec := range recs {
		spaceId := rec.Details.GetString(bundle.RelationKeySpaceId)

		// There is no point to index file if the current user is not an owner of the file
		myParticipantId := ind.accountService.MyParticipantId(spaceId)
		if rec.Details.GetString(bundle.RelationKeyCreator) != myParticipantId {
			continue
		}

		id := domain.FullID{
			SpaceID:  spaceId,
			ObjectID: rec.Details.GetString(bundle.RelationKeyId),
		}
		fileId := domain.FullFileId{
			SpaceId: spaceId,
			FileId:  domain.FileId(rec.Details.GetString(bundle.RelationKeyFileId)),
		}
		// Additional check if we are accidentally migrated file object
		if !fileId.Valid() {
			continue
		}
		err = ind.addToQueue(ctx, id, fileId)
		if err != nil {
			return fmt.Errorf("add to index queue: %w", err)
		}
	}
	return nil
}

const indexingProviderPeriod = 60 * time.Second

// runIndexingProvider provides worker with job to do
func (ind *indexer) runIndexingProvider() {
	defer ind.closeWg.Done()

	ticker := time.NewTicker(indexingProviderPeriod)
	run := func() {
		if err := ind.addToQueueFromObjectStore(ind.indexCtx); err != nil {
			log.Errorf("add to index queue from object store: %v", err)
		}
	}

	run()
	for {
		select {
		case <-ind.indexCtx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (ind *indexer) runIndexingWorker() {
	defer ind.closeWg.Done()

	for {
		select {
		case <-ind.indexCtx.Done():
			return
		default:
		}
		if err := ind.indexNext(ind.indexCtx); err != nil {
			logIndexLoop(err)
		}
	}
}

func logIndexLoop(err error) {
	if errors.Is(err, treestorage.ErrUnknownTreeId) {
		return
	}
	if errors.Is(err, format.ErrNotFound{}) {
		return
	}
	if errors.Is(err, rpcstore.ErrNoConnectionToAnyFileClient) {
		return
	}
	if errors.Is(err, files.FailedProtoUnmarshallError) {
		return
	}
	log.Errorf("index loop: %v", err)
}

func (ind *indexer) indexNext(ctx context.Context) error {
	req, err := ind.indexQueue.NewCond().WaitOne(ctx)
	if err != nil {
		return fmt.Errorf("wait for index request: %w", err)
	}
	return ind.indexFile(ctx, req.id, req.fileId)
}

// indexFile updates file details from metadata and adds file to local cache
func (ind *indexer) indexFile(ctx context.Context, id domain.FullID, fileId domain.FullFileId) error {
	defer ind.markIndexingDone(id)

	space, err := ind.spaceService.Get(ctx, id.SpaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	err = space.Do(id.ObjectID, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		infos, err := ind.fileService.GetFileVariants(ctx, fileId, st.GetFileInfo().EncryptionKeys)
		if err != nil {
			return fmt.Errorf("get infos for indexing: %w", err)
		}
		err = ind.injectMetadataToState(ctx, st, infos, fileId, id)
		if err != nil {
			return fmt.Errorf("inject metadata to state: %w", err)
		}
		return sb.Apply(st)
	})
	if err != nil {
		return fmt.Errorf("apply to smart block: %w", err)
	}
	return nil
}

func (ind *indexer) injectMetadataToState(ctx context.Context, st *state.State, infos []*storage.FileInfo, fileId domain.FullFileId, id domain.FullID) error {
	err := filemodels.InjectVariantsToDetails(infos, st)
	if err != nil {
		return fmt.Errorf("inject variants: %w", err)
	}

	prevDetails := st.CombinedDetails()
	details, typeKey, err := ind.buildDetails(ctx, fileId, infos)
	if err != nil {
		return fmt.Errorf("build details: %w", err)
	}

	st.SetObjectTypeKey(typeKey)

	keys := make([]domain.RelationKey, 0, details.Len())
	for k, _ := range details.Iterate() {
		keys = append(keys, k)
	}
	st.AddBundledRelationLinks(keys...)
	st.AddRelationKeys(keys...)

	details = prevDetails.Merge(details)
	st.SetDetails(details)

	err = fileblocks.AddFileBlocks(st, details, id.ObjectID)
	if err != nil {
		return fmt.Errorf("add blocks: %w", err)
	}
	return nil
}

func (ind *indexer) buildDetails(ctx context.Context, id domain.FullFileId, infos []*storage.FileInfo) (details *domain.Details, typeKey domain.TypeKey, err error) {
	file, err := files.NewFile(ind.fileService, id, infos)
	if err != nil {
		return nil, "", fmt.Errorf("new file: %w", err)
	}

	if file.Mill() == mill.BlobId {
		details, typeKey, err = file.Details(ctx)
		if err != nil {
			return nil, "", err
		}
	} else {
		image := files.NewImage(ind.fileService, id, infos)
		details, err = image.Details(ctx)
		if err != nil {
			return nil, "", err
		}
	}

	// Overwrite typeKey for images in case that image is uploaded as file.
	// That can be possible because some images can't be handled properly and wee fall back to
	// handling them as files
	if mill.IsImage(file.MimeType()) {
		typeKey = bundle.TypeKeyImage
	}

	details.SetInt64(bundle.RelationKeyFileIndexingStatus, int64(model.FileIndexingStatus_Indexed))
	return details, typeKey, nil
}
