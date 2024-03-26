package fileobject

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	fileblock "github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
}

func (s *service) newIndexer() *indexer {
	ind := &indexer{
		fileService:  s.fileService,
		spaceService: s.spaceService,
		objectStore:  s.objectStore,

		indexQueue: mb.New[indexRequest](0),
		isQueued:   make(map[domain.FullID]struct{}),
	}
	ind.initQuery()
	return ind
}

func (ind *indexer) run() {
	ind.indexCtx, ind.indexCancel = context.WithCancel(context.Background())
	go ind.runIndexingProvider()
	go ind.runIndexingWorker()
}

func (ind *indexer) close() error {
	ind.indexCancel()
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
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value: pbtypes.IntList(
					int(model.ObjectType_file),
					int(model.ObjectType_image),
					int(model.ObjectType_video),
					int(model.ObjectType_audio),
				),
			},
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyFileIndexingStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
		},
	}
}

func (ind *indexer) addToQueueFromObjectStore(ctx context.Context) error {
	recs, _, err := ind.objectStore.Query(ind.query)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	for _, rec := range recs {
		spaceId := pbtypes.GetString(rec.Details, bundle.RelationKeySpaceId.String())
		id := domain.FullID{
			SpaceID:  spaceId,
			ObjectID: pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()),
		}
		fileId := domain.FullFileId{
			SpaceId: spaceId,
			FileId:  domain.FileId(pbtypes.GetString(rec.Details, bundle.RelationKeyFileId.String())),
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
	for {
		select {
		case <-ind.indexCtx.Done():
			return
		default:
		}
		if err := ind.indexNext(ind.indexCtx); err != nil {
			log.Errorf("index loop: %v", err)
		}
	}
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
	details, typeKey, err := ind.buildDetails(ctx, fileId)
	if errors.Is(err, domain.ErrFileNotFound) {
		log.Errorf("build details: %v", err)
		ind.markFileAsNotFound(st)
		return nil
	}
	if err != nil {
		return fmt.Errorf("build details: %w", err)
	}

	st.SetObjectTypeKey(typeKey)
	prevDetails := st.CombinedDetails()

	keys := make([]domain.RelationKey, 0, len(details.Fields))
	for k := range details.Fields {
		keys = append(keys, domain.RelationKey(k))
	}
	st.AddBundledRelations(keys...)

	details = pbtypes.StructMerge(prevDetails, details, true)
	st.SetDetails(details)

	err = ind.addBlocks(st, details, id.ObjectID)
	if err != nil {
		return fmt.Errorf("add blocks: %w", err)
	}
	return nil
}

func (ind *indexer) markFileAsNotFound(st *state.State) {
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileIndexingStatus, pbtypes.Int64(int64(model.FileIndexingStatus_NotFound)))
}

func (ind *indexer) buildDetails(ctx context.Context, id domain.FullFileId) (details *types.Struct, typeKey domain.TypeKey, err error) {
	file, err := ind.fileService.FileByHash(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if mill.IsImage(file.Info().Media) {
		image, err := ind.fileService.ImageByHash(ctx, id)
		if err != nil {
			return nil, "", err
		}
		details, err = image.Details(ctx)
		if err != nil {
			return nil, "", err
		}
		typeKey = bundle.TypeKeyImage
	} else {
		details, typeKey, err = file.Details(ctx)
		if err != nil {
			return nil, "", err
		}
	}
	details.Fields[bundle.RelationKeyFileIndexingStatus.String()] = pbtypes.Int64(int64(model.FileIndexingStatus_Indexed))
	return details, typeKey, nil
}

func (ind *indexer) addBlocks(st *state.State, details *types.Struct, objectId string) error {
	fileType := fileblock.DetectTypeByMIME(pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()))

	fname := pbtypes.GetString(details, bundle.RelationKeyName.String())
	ext := pbtypes.GetString(details, bundle.RelationKeyFileExt.String())

	if ext != "" && !strings.HasSuffix(fname, "."+ext) {
		fname = fname + "." + ext
	}

	var blocks []*model.Block
	blocks = append(blocks, &model.Block{
		Id: "file",
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:           fname,
				Mime:           pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()),
				TargetObjectId: objectId,
				Type:           fileType,
				Size_:          int64(pbtypes.GetFloat64(details, bundle.RelationKeySizeInBytes.String())),
				State:          model.BlockContentFile_Done,
				AddedAt:        int64(pbtypes.GetFloat64(details, bundle.RelationKeyFileMimeType.String())),
			},
		}})

	switch fileType {
	case model.BlockContentFile_Image:
		st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, pbtypes.String(objectId))

		if pbtypes.GetInt64(details, bundle.RelationKeyWidthInPixels.String()) != 0 {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyWidthInPixels))
		}

		if pbtypes.GetInt64(details, bundle.RelationKeyHeightInPixels.String()) != 0 {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyHeightInPixels))
		}

		if pbtypes.GetString(details, bundle.RelationKeyCamera.String()) != "" {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyCamera))
		}

		if pbtypes.GetInt64(details, bundle.RelationKeySizeInBytes.String()) != 0 {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeySizeInBytes))
		}
		if pbtypes.GetString(details, bundle.RelationKeyMediaArtistName.String()) != "" {
			blocks = append(blocks, makeRelationBlock(bundle.RelationKeyMediaArtistName))
		}
		if pbtypes.GetString(details, bundle.RelationKeyMediaArtistURL.String()) != "" {
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
