package objectstore

/*
AI generated

Name: Object Store and Fulltext Queue Manager
Scope: global

## Responsibility
- Manages per-space indexes (spaceindex.Store) for object details and links
- Provides cross-space queries aggregating data from all space indexes
- Manages fulltext indexing queue (add, list, mark indexed, reconcile consistency)
- Stores file encryption keys
- Stores account status and indexer checksums
- Manages virtual spaces registry
- Maps object IDs to space IDs (spaceresolverstore)

## External State
- any-store collections in common db: fulltext_queue, indexerChecksums, virtualSpaces, file_keys, bindId
- Per-space databases via spaceindex (objects, links, headsState, activeViews, pendingDetails collections)

## Documentation
Fulltext queue flow:
1. Objects added to queue via AddToIndexQueue with seq=0
2. BatchProcessFullTextQueue processes items: lists pending, calls processor, marks as indexed with ftIndexSeq
3. On startup, FtQueueReconcileWithSeq checks consistency: resets seq to 0 for items with seq > ftIndexSeq, deletes already-indexed items
*/

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceresolverstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-localstore")

const CName = "objectstore"

var (
	_ ObjectStore = (*dsObjectStore)(nil)
)

type CrossSpace interface {
	QueryCrossSpace(q database.Query) (records []database.Record, err error)
	QueryByIdCrossSpace(ids []string) (records []database.Record, err error)

	ListIdsCrossSpace() ([]string, error)
	EnqueueAllForFulltextIndexing(ctx context.Context) error
	BatchProcessFullTextQueue(spaceIds func() []string, limit uint, processIds domain.FullTextProcessFunc) error

	AccountStore
	VirtualSpacesStore
	IndexerStore
}

type ObjectStore interface {
	app.ComponentRunnable

	IterateSpaceIndex(func(store spaceindex.Store) error) error
	SpaceIndex(spaceId string) spaceindex.Store

	SpaceNameGetter
	spaceresolverstore.Store
	CrossSpace

	FtQueueReconcileWithSeq(ctx context.Context, ftIndexSeq uint64) error
	FtQueueMarkAsIndexed(ids []domain.FullID, ftIndexSeq uint64) error

	AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error
	GetFileKeys(fileId domain.FileId) (map[string]string, error)
}

type IndexerStore interface {
	AddToIndexQueue(ctx context.Context, id ...domain.FullID) (ftQueueCounter uint64, enqueued int, err error)
	// AddToIndexQueueWithCounter adds objects to FT queue and returns a counter for consistency tracking.
	// The counter is persisted atomically with the queue entries for crash recovery.
	AddToIndexQueueWithCounter(ctx context.Context, ftQueueCounter uint64, ids ...domain.FullID) (enqueued int, err error)
	AddChatMessageToIndexQueue(ctx context.Context, chatId domain.FullID, orderId string) error
	AddChatMessageDeleteToIndexQueue(ctx context.Context, chatId domain.FullID, messageId string) error
	ListIdsFromFullTextQueue(spaceIds []string, limit uint) ([]domain.FullTextQueuedObject, error)
	FtQueueMarkAsIndexed(ids []domain.FullID, ftIndexSeq uint64) error

	// ClearFullTextQueue cleans the pending . Pass nil to clear all spaces.
	ClearFullTextQueue(spaceIds []string) error

	// GetChecksums Used to get information about localstore state and decide do we need to reindex some objects
	GetChecksums(spaceID string) (checksums *model.ObjectStoreChecksums, err error)
	// SaveChecksums Used to save checksums and force reindex counter
	SaveChecksums(spaceID string, checksums *model.ObjectStoreChecksums) (err error)

	// GetFTRecheckCounter returns the global FT recheck check counter
	GetFTRecheckCounter(ctx context.Context) (int32, error)
	// SetFTRecheckCounter sets the global FT recheck counter
	SetFTRecheckCounter(ctx context.Context, counter int32) error
	// RunFTConsistencyCheck checks all objects in the object store against the FT index
	// and enqueues missing ones for FT indexing
	RunFTConsistencyCheck(ctx context.Context, fts ftsearch.FTSearch) (checked, enqueued int, err error)
	// GetFTQueueCounter returns the last persisted FT queue counter for a specific space (crash recovery)
	GetFTQueueCounter(ctx context.Context, spaceId string) (uint64, error)
	// WriteTx starts a write transaction to commonDB
	WriteTx(ctx context.Context) (anystore.WriteTx, error)
}

type AccountStore interface {
	GetAccountStatus() (status *coordinatorproto.SpaceStatusPayload, err error)
	SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) (err error)
}

type VirtualSpacesStore interface {
	SaveVirtualSpace(id string) error
	ListVirtualSpaces() ([]string, error)
	DeleteVirtualSpace(spaceID string) error
}

type TechSpaceIdProvider interface {
	TechSpaceId() string
}

type dsObjectStore struct {
	anystoreProvider anystoreprovider.Provider

	spaceresolverstore.Store

	techSpaceId string

	db anystore.DB

	indexerChecksums anystore.Collection
	virtualSpaces    anystore.Collection

	fileKeys      keyvaluestore.Store[map[string]string]
	accountStatus keyvaluestore.Store[*coordinatorproto.SpaceStatusPayload]
	fulltextQueue anystore.Collection

	arenaPool *anyenc.ArenaPool

	fts                 ftsearch.FTSearch
	subManager          *spaceindex.SubscriptionManager
	sourceService       spaceindex.SourceDetailsFromID
	techSpaceIdProvider TechSpaceIdProvider

	spaceStoreDirsCheck sync.Once

	lock         sync.Mutex
	spaceIndexes map[string]spaceindex.Store

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func (s *dsObjectStore) ProvideStat() any {
	count, _ := s.ListIdsCrossSpace()
	return len(count)
}

func (s *dsObjectStore) StatId() string {
	return "ds_count"
}

func (s *dsObjectStore) StatType() string {
	return CName
}

func (s *dsObjectStore) IterateSpaceIndex(f func(store spaceindex.Store) error) error {
	s.lock.Lock()
	spaceIndexes := make([]spaceindex.Store, 0, len(s.spaceIndexes))
	for _, store := range s.spaceIndexes {
		spaceIndexes = append(spaceIndexes, store)
	}
	s.lock.Unlock()
	for _, store := range s.spaceIndexes {
		if err := f(store); err != nil {
			return err
		}
	}
	return nil
}

func New() ObjectStore {
	ctx, cancel := context.WithCancel(context.Background())
	return &dsObjectStore{
		componentCtx:       ctx,
		componentCtxCancel: cancel,
		subManager:         &spaceindex.SubscriptionManager{},
		spaceIndexes:       map[string]spaceindex.Store{},
	}
}

func (s *dsObjectStore) Init(a *app.App) (err error) {
	s.sourceService = app.MustComponent[spaceindex.SourceDetailsFromID](a)
	s.fts = app.MustComponent[ftsearch.FTSearch](a)
	s.anystoreProvider = app.MustComponent[anystoreprovider.Provider](a)
	s.db = s.anystoreProvider.GetCommonDb()
	s.arenaPool = &anyenc.ArenaPool{}

	s.techSpaceIdProvider = app.MustComponent[TechSpaceIdProvider](a)
	statService, _ := app.GetComponent[debugstat.StatService](a)
	if statService != nil {
		statService.AddProvider(s)
	}

	return s.initCollections(s.componentCtx)
}

func (s *dsObjectStore) Name() (name string) {
	return CName
}

func (s *dsObjectStore) Run(ctx context.Context) error {
	s.techSpaceId = s.techSpaceIdProvider.TechSpaceId()

	store, err := spaceresolverstore.New(s.componentCtx, s.db)
	if err != nil {
		return fmt.Errorf("new space resolver store: %w", err)
	}

	s.Store = store

	return err
}

func (s *dsObjectStore) GetCommonDb() anystore.DB {
	return s.db
}

func (s *dsObjectStore) initCollections(ctx context.Context) error {
	store := s.anystoreProvider.GetCommonDb()

	fulltextQueue, err := store.Collection(ctx, "fulltext_queue")
	if err != nil {
		return fmt.Errorf("open fulltextQueue collection: %w", err)
	}

	indexes := []anystore.IndexInfo{
		{
			Fields: []string{spaceIdKey, ftSequenceKey},
		},
	}
	err = anystorehelper.AddIndexes(ctx, fulltextQueue, indexes)
	if err != nil {
		return fmt.Errorf("add indexes to fulltextQueue collection: %w", err)
	}

	fileKeys, err := keyvaluestore.NewJson[map[string]string](store, "file_keys")
	if err != nil {
		return fmt.Errorf("open file_keys collection: %w", err)
	}

	system := s.anystoreProvider.GetSystemCollection()
	s.accountStatus = keyvaluestore.NewJsonFromCollection[*coordinatorproto.SpaceStatusPayload](system)

	indexerChecksums, err := store.Collection(ctx, "indexerChecksums")
	if err != nil {
		return fmt.Errorf("open indexerChecksums collection: %w", err)
	}
	virtualSpaces, err := store.Collection(ctx, "virtualSpaces")
	if err != nil {
		return fmt.Errorf("open virtualSpaces collection: %w", err)
	}

	s.db = store
	s.fulltextQueue = fulltextQueue
	s.indexerChecksums = indexerChecksums
	s.virtualSpaces = virtualSpaces
	s.fileKeys = fileKeys

	return nil
}

func (s *dsObjectStore) Close(_ context.Context) (err error) {
	return err
}

func (s *dsObjectStore) SpaceIndex(spaceId string) spaceindex.Store {
	if spaceId == "" {
		return spaceindex.NewInvalidStore(errors.New("empty spaceId"))
	}
	s.lock.Lock()
	spaceIndex := s.getOrInitSpaceIndex(spaceId)
	s.lock.Unlock()
	err := spaceIndex.Init()
	if err != nil {
		return spaceindex.NewInvalidStore(err)
	}
	return spaceIndex
}

func (s *dsObjectStore) getOrInitSpaceIndex(spaceId string) spaceindex.Store {
	store, ok := s.spaceIndexes[spaceId]
	if !ok {
		store = spaceindex.New(s.componentCtx, spaceId, spaceindex.Deps{
			DbProvider:    s.anystoreProvider,
			SourceService: s.sourceService,
			Fts:           s.fts,
			SubManager:    s.subManager,
			FulltextQueue: s,
		})
		s.spaceIndexes[spaceId] = store
	}
	return store
}

func (s *dsObjectStore) preloadExistingObjectStores() error {
	var err error
	s.spaceStoreDirsCheck.Do(func() {
		spaceIds, err := s.anystoreProvider.ListSpaceIdsFromFilesystem()
		if err != nil {
			log.Error("list space ids from filesystem", zap.Error(err))
		}

		var indexes []spaceindex.Store
		s.lock.Lock()
		for _, spaceId := range spaceIds {
			spaceIndex := s.getOrInitSpaceIndex(spaceId)
			indexes = append(indexes, spaceIndex)
		}
		s.lock.Unlock()

		var wg sync.WaitGroup
		for _, index := range indexes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				initErr := index.Init()
				if initErr != nil {
					log.With("error", initErr).Error("pre-init space index")
				}
			}()
		}
		wg.Wait()
	})
	return err
}

func (s *dsObjectStore) listStores() []spaceindex.Store {
	err := s.preloadExistingObjectStores()
	if err != nil {
		log.Errorf("preloadExistingObjectStores: %v", err)
	}

	s.lock.Lock()
	stores := make([]spaceindex.Store, 0, len(s.spaceIndexes))
	for _, store := range s.spaceIndexes {
		stores = append(stores, store)
	}
	s.lock.Unlock()
	return stores
}

func collectCrossSpace[T any](s *dsObjectStore, proc func(store spaceindex.Store) ([]T, error)) ([]T, error) {
	stores := s.listStores()

	var result []T
	for _, store := range stores {
		err := store.Init()
		if err != nil {
			return nil, fmt.Errorf("init store: %w", err)
		}
		items, err := proc(store)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}
	return result, nil
}

func iterateSpacesForFulltext(s *dsObjectStore, proc func(store spaceindex.Store) error) error {
	stores := s.listStores()
	for _, store := range stores {
		if store.SpaceId() == s.techSpaceId || store.SpaceId() == addr.AnytypeMarketplaceWorkspace {
			continue
		}
		err := store.Init()
		if err != nil {
			return fmt.Errorf("init store: %w", err)
		}
		err = proc(store)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *dsObjectStore) ListIdsCrossSpace() ([]string, error) {
	return collectCrossSpace(s, func(store spaceindex.Store) ([]string, error) {
		return store.ListIds()
	})
}

func (s *dsObjectStore) EnqueueAllForFulltextIndexing(ctx context.Context) error {
	txn, err := s.fulltextQueue.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	arena := s.arenaPool.Get()
	defer func() {
		_ = txn.Rollback()
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	const maxErrorsToLog = 5
	var loggedErrors int

	err = iterateSpacesForFulltext(s, func(store spaceindex.Store) error {
		err := store.IterateAll(func(doc *anyenc.Value) error {
			id := doc.GetString(idKey)
			spaceId := doc.GetString(spaceIdKey)

			arena.Reset()
			obj := arena.NewObject()
			obj.Set(idKey, arena.NewString(id))
			obj.Set(spaceIdKey, arena.NewString(spaceId))
			obj.Set(ftSequenceKey, arena.NewBinary(emptyBuffer))
			err := s.fulltextQueue.UpsertOne(txn.Context(), obj)
			if err != nil {
				if loggedErrors < maxErrorsToLog {
					log.With("error", err).Warnf("EnqueueAllForFulltextIndexing: upsert")
					loggedErrors++
				}
				return nil
			}
			return nil
		})
		return err
	})
	if err != nil {
		return err
	}
	return txn.Commit()
}

func isFtIndexable(id string, value *anyenc.Value) bool {
	if sbt, err := typeprovider.SmartblockTypeFromID(id); err == nil {
		ft, _, _ := sbt.Indexable()
		if !ft {
			return false
		}
	}

	if value.GetBool(bundle.RelationKeyIsDeleted.String()) || value.GetBool(bundle.RelationKeyIsArchived.String()) || value.GetBool(bundle.RelationKeyIsHidden.String()) {
		return false
	}

	if slices.Contains([]model.ObjectTypeLayout{model.ObjectType_chatDerived, model.ObjectType_chatDeprecated, model.ObjectType_dashboard},
		model.ObjectTypeLayout(value.GetInt(bundle.RelationKeyResolvedLayout.String()))) {
		return false
	}
	var checkTextProperties = []domain.RelationKey{
		bundle.RelationKeyName,
		bundle.RelationKeyDescription,
		bundle.RelationKeySnippet,
	}
	for _, property := range checkTextProperties {
		if value.GetString(property.String()) != "" {
			return true
		}
	}
	return false
}

// RunFTConsistencyCheck checks all objects in the object store against the full-text index
// and enqueues any missing objects for FT indexing. This is a lightweight consistency check
// that doesn't load objects into cache.
func (s *dsObjectStore) RunFTConsistencyCheck(ctx context.Context, fts ftsearch.FTSearch) (checked, enqueued int, err error) {
	// First, get all indexed object IDs from FT in one pass
	indexedIds, err := fts.ListAllObjectIds()
	if err != nil {
		return 0, 0, fmt.Errorf("list indexed object ids: %w", err)
	}

	var missingIds []domain.FullID

	// Iterate all objects in object store and check against indexed IDs
	var spaces, objs int
	err = iterateSpacesForFulltext(s, func(store spaceindex.Store) error {
		spaces++
		return store.IterateAll(func(doc *anyenc.Value) error {
			objs++
			id := doc.GetString(idKey)
			spaceId := store.SpaceId()
			if !isFtIndexable(id, doc) {
				return nil
			}

			checked++
			if _, exists := indexedIds[id]; !exists {
				missingIds = append(missingIds, domain.FullID{ObjectID: id, SpaceID: spaceId})
			}
			return nil
		})
	})
	fmt.Printf("checked %d objects in %d spaces, found %d missing\n", objs, spaces, len(missingIds))
	if err != nil {
		return checked, 0, fmt.Errorf("iterate objects: %w", err)
	}

	// Batch enqueue all missing IDs at once
	if len(missingIds) > 0 {
		for _, id := range missingIds {
			index := s.getOrInitSpaceIndex(id.SpaceID)
			d, err := index.GetDetails(id.ObjectID)
			if err != nil {
				fmt.Printf("object %s/%s get details error: %v\n", id.SpaceID, id.ObjectID, err)
			} else {
				fmt.Printf("object %s/%s missisng details: %+v\n", id.SpaceID, id.ObjectID, pbtypes.Sprint(d.ToProto()))
			}
		}
		_, enqueued, err = s.AddToIndexQueue(ctx, missingIds...)
		if err != nil {
			return checked, 0, fmt.Errorf("batch enqueue: %w", err)
		}
	}

	return checked, enqueued, nil
}

func (s *dsObjectStore) QueryByIdCrossSpace(ids []string) ([]database.Record, error) {
	return collectCrossSpace(s, func(store spaceindex.Store) ([]database.Record, error) {
		return store.QueryByIds(ids)
	})
}

func (s *dsObjectStore) QueryCrossSpace(q database.Query) ([]database.Record, error) {
	return collectCrossSpace(s, func(store spaceindex.Store) ([]database.Record, error) {
		return store.Query(q)
	})
}

func (s *dsObjectStore) SubscribeLinksUpdate(callback func(info spaceindex.LinksUpdateInfo)) {
	s.subManager.SubscribeLinksUpdate(callback)
}

// WriteTx returns a new write transaction for commonDb
func (s *dsObjectStore) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return s.db.WriteTx(ctx)
}
