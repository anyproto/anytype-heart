package fileobject

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/cheggaaa/mb/v3"
	format "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	fileblock "github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/space"
)

type indexer struct {
	fileService  files.Service
	spaceService space.Service
	objectStore  objectstore.ObjectStore

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
		fileService:  s.fileService,
		spaceService: s.spaceService,
		objectStore:  s.objectStore,

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
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value: domain.Int64List([]model.ObjectTypeLayout{
					model.ObjectType_file,
					model.ObjectType_image,
					model.ObjectType_video,
					model.ObjectType_audio,
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
		err := ind.injectMetadataToState(ctx, st, fileId, id)
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

func (ind *indexer) injectMetadataToState(ctx context.Context, st *state.State, fileId domain.FullFileId, id domain.FullID) error {
	infos, err := ind.fileService.GetFileVariants(ctx, fileId, st.GetFileInfo().EncryptionKeys)
	if err != nil {
		return fmt.Errorf("get infos for indexing: %w", err)
	}
	err = filemodels.InjectVariantsToDetails(infos, st)
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

	details = prevDetails.Merge(details)
	st.SetDetails(details)

	err = ind.addBlocks(st, details, id.ObjectID)
	if err != nil {
		return fmt.Errorf("add blocks: %w", err)
	}
	return nil
}

func (ind *indexer) buildDetails(ctx context.Context, id domain.FullFileId, infos []*storage.FileInfo) (details *domain.Details, typeKey domain.TypeKey, err error) {
	file := fileobject.NewFile(ind.fileService, id, infos)

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
	if mill.IsImage(file.Media()) {
		typeKey = bundle.TypeKeyImage
	}

	details.SetInt64(bundle.RelationKeyFileIndexingStatus, int64(model.FileIndexingStatus_Indexed))
	return details, typeKey, nil
}

func (ind *indexer) addBlocks(st *state.State, details *domain.Details, objectId string) error {
	fname := details.GetString(bundle.RelationKeyName)
	fileType := fileblock.DetectTypeByMIME(fname, details.GetString(bundle.RelationKeyFileMimeType))

	ext := details.GetString(bundle.RelationKeyFileExt)

	if ext != "" && !strings.HasSuffix(fname, "."+ext) {
		fname = fname + "." + ext
	}

	var blocks []*model.Block
	blocks = append(blocks, &model.Block{
		Id: "file",
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:           fname,
				Mime:           details.GetString(bundle.RelationKeyFileMimeType),
				TargetObjectId: objectId,
				Type:           fileType,
				Size_:          int64(details.GetFloat64(bundle.RelationKeySizeInBytes)),
				State:          model.BlockContentFile_Done,
				AddedAt:        int64(details.GetFloat64(bundle.RelationKeyFileMimeType)),
			},
		}})

	switch fileType {
	case model.BlockContentFile_Image:
		st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, domain.String(objectId))

		if details.GetInt64(bundle.RelationKeyWidthInPixels) != 0 {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyWidthInPixels))
		}

		if details.GetInt64(bundle.RelationKeyHeightInPixels) != 0 {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyHeightInPixels))
		}

		if details.GetString(bundle.RelationKeyCamera) != "" {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyCamera))
		}

		if details.GetInt64(bundle.RelationKeySizeInBytes) != 0 {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeySizeInBytes))
		}
		if details.GetString(bundle.RelationKeyMediaArtistName) != "" {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyMediaArtistName))
		}
		if details.GetString(bundle.RelationKeyMediaArtistURL) != "" {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyMediaArtistURL))
		}
	default:
		blocks = append(blocks, makeRelationBlock(bundle.RelationKeySizeInBytes))
	}

	for _, b := range blocks {
		if st.Exists(b.Id) {
			st.Set(simple.New(b))
		} else {
			st.Add(simple.New(b))
			err := st.InsertTo(st.RootId(), model.Block_Inner, b.Id)
			if err != nil {
				return fmt.Errorf("failed to insert file block: %w", err)
			}
		}
	}
	template.WithAllBlocksEditsRestricted(st)
	return nil
}

func makeRelationBlock(relationKey domain.RelationKey) *model.Block {
	return &model.Block{
		Id: relationKey.String(),
		Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{
				Key: relationKey.String(),
			},
		},
	}
}
