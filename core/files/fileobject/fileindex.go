package fileobject

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
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

	details, typeKey, err := ind.buildDetails(ctx, fileId)
	if err != nil {
		return fmt.Errorf("get details for file or image: %w", err)
	}
	space, err := ind.spaceService.Get(ctx, id.SpaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	err = space.Do(id.ObjectID, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetObjectTypeKey(typeKey)
		prevDetails := st.CombinedDetails()
		details = pbtypes.StructMerge(prevDetails, details, true)
		st.SetDetails(details)
		return sb.Apply(st)
	})
	if err != nil {
		return fmt.Errorf("apply to smart block: %w", err)
	}
	return nil
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
