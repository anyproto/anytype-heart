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

	// DeleteSpaceIndex closes and forgets the in-memory space index and removes
	// the space's objectstore data (index + CRDT databases and directory) from disk.
	DeleteSpaceIndex(spaceId string) error

	// OnSpaceIndexOpened registers cb to be invoked once when each space's
	// objectstore DB transitions from "not opened" to "opened" via SpaceIndex.
	// Already-opened spaces are replayed synchronously during registration.
	// Callbacks are invoked outside the object store's internal locks; they
	// may safely re-enter SpaceIndex.
	OnSpaceIndexOpened(cb func(spaceId string))
	// OpenedSpaceIds returns a snapshot of spaces whose objectstore DB is
	// currently open.
	OpenedSpaceIds() []string

	// WaitStoresLoaded blocks until the background warm-up has opened every
	// space in the authoritative set, or ctx is done. The one-shot
	// cross-space reads (QueryCrossSpace, QueryByIdCrossSpace,
	// ListIdsCrossSpace) and IterateSpaceIndex already wait internally, so
	// callers normally do not need this. NOTE: the full-text path
	// (iterateSpacesForFulltext) and per-space SpaceIndex deliberately do
	// not wait. Exposed for code that wants to await warm-up without
	// issuing a query.
	WaitStoresLoaded(ctx context.Context) error

	SpaceNameGetter
	GetSpaceViewDetails(spaceId string) (*domain.Details, error)
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

// spaceIdsLister enumerates every space the account has on disk, independent
// of the (derived, possibly-incomplete) objectstore index. Satisfied by
// space/spacecore/storage.ClientStorage. Resolved optionally: absent in
// lightweight test/migrator app assemblies.
type spaceIdsLister interface {
	AllSpaceIds() (ids []string, err error)
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
	spaceStorageLister  spaceIdsLister
	loadedCh            chan struct{}

	lock         sync.Mutex
	spaceIndexes map[string]spaceindex.Store

	spaceOpenedLock           sync.Mutex
	openedSpaceIds            map[string]struct{}
	spaceIndexOpenedCallbacks []func(spaceId string)

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
	// Wait-by-default (see collectCrossSpace): never iterate a partial set
	// of space indexes.
	if err := s.WaitStoresLoaded(s.componentCtx); err != nil {
		return fmt.Errorf("wait stores loaded: %w", err)
	}
	s.lock.Lock()
	spaceIndexes := make([]spaceindex.Store, 0, len(s.spaceIndexes))
	for _, store := range s.spaceIndexes {
		spaceIndexes = append(spaceIndexes, store)
	}
	s.lock.Unlock()
	for _, store := range spaceIndexes {
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
		openedSpaceIds:     map[string]struct{}{},
		loadedCh:           make(chan struct{}),
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

	if lister, lerr := app.GetComponent[spaceIdsLister](a); lerr == nil {
		s.spaceStorageLister = lister
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

	go s.backgroundWarmUp()

	return nil
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
	// Cancel componentCtx so any in-flight WaitStoresLoaded (cross-space
	// reads / IterateSpaceIndex block on it) and the background warm-up
	// unblock on shutdown instead of hanging until the process exits.
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}
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
	s.markSpaceIndexOpened(spaceId)
	return spaceIndex
}

// DeleteSpaceIndex closes the in-memory space index, drops it from the
// registry (so it won't be iterated/queried cross-space anymore), forgets it
// from the opened-spaces set, and removes the on-disk objectstore data for the
// space via the anystore provider.
func (s *dsObjectStore) DeleteSpaceIndex(spaceId string) error {
	if spaceId == "" {
		return errors.New("empty spaceId")
	}

	s.lock.Lock()
	store, ok := s.spaceIndexes[spaceId]
	if ok {
		delete(s.spaceIndexes, spaceId)
	}
	s.lock.Unlock()

	var errs error
	if ok {
		if err := store.Close(); err != nil {
			errs = errors.Join(errs, fmt.Errorf("close space index: %w", err))
		}
	}

	s.spaceOpenedLock.Lock()
	delete(s.openedSpaceIds, spaceId)
	s.spaceOpenedLock.Unlock()

	if err := s.anystoreProvider.DeleteSpaceData(spaceId); err != nil {
		errs = errors.Join(errs, fmt.Errorf("delete space data: %w", err))
	}
	return errs
}

func (s *dsObjectStore) OnSpaceIndexOpened(cb func(spaceId string)) {
	s.spaceOpenedLock.Lock()
	s.spaceIndexOpenedCallbacks = append(s.spaceIndexOpenedCallbacks, cb)
	replay := make([]string, 0, len(s.openedSpaceIds))
	for spaceId := range s.openedSpaceIds {
		replay = append(replay, spaceId)
	}
	s.spaceOpenedLock.Unlock()
	for _, spaceId := range replay {
		cb(spaceId)
	}
}

func (s *dsObjectStore) OpenedSpaceIds() []string {
	s.spaceOpenedLock.Lock()
	defer s.spaceOpenedLock.Unlock()
	ids := make([]string, 0, len(s.openedSpaceIds))
	for spaceId := range s.openedSpaceIds {
		ids = append(ids, spaceId)
	}
	return ids
}

// markSpaceIndexOpened records a space as opened and fires registered
// callbacks exactly once, on the first successful open.
//
// No cross-goroutine synchronization is needed here: data consistency is
// guaranteed one layer down. spaceindex.Store.UpdateObjectDetails persists
// to the anystore DB independently of any subscription, and when a per-space
// subscription is later created it wires SubscribeForAll and then re-queries
// the full store (core/subscription/service.go subscribeForQuery →
// queryEntries, plus sortedSub.entriesBeforeStarted). So a write that races
// the open is always recovered: either the post-wiring re-query reads it from
// the persistent store, or SubscribeForAll (wired before that query) delivers
// it. See TestLazySubscribe_NoDataLossUnderConcurrentOpen.
//
// openedSpaceIds is set before the callbacks run so that (a) the callback
// chain's own re-entry into SpaceIndex for the same space — cross-space sub's
// PromotePending → subscriptionservice.Search → getSpaceSubscriptions →
// SpaceIndex — short-circuits here instead of recursing/deadlocking, and
// (b) a listener registering during the firing still observes the space via
// OnSpaceIndexOpened's replay.
func (s *dsObjectStore) markSpaceIndexOpened(spaceId string) {
	s.spaceOpenedLock.Lock()
	if _, ok := s.openedSpaceIds[spaceId]; ok {
		s.spaceOpenedLock.Unlock()
		return
	}
	s.openedSpaceIds[spaceId] = struct{}{}
	callbacks := slices.Clone(s.spaceIndexOpenedCallbacks)
	s.spaceOpenedLock.Unlock()

	for _, cb := range callbacks {
		cb(spaceId)
	}
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

// preloadConcurrencyDefault is 1: warm-up opens spaces strictly one at a
// time. This minimizes the startup disk spike and ensures warm-up holds at
// most one per-space Init lock, so a concurrent direct SpaceIndex call for a
// different space effectively never contends with it.
const preloadConcurrencyDefault = 1

// preloadConcurrency caps parallel per-space store opens during warm-up.
// Variable (not const) so tests can pin it.
var preloadConcurrency = preloadConcurrencyDefault

// authoritativeSpaceIds returns the union of every space dir on disk
// (objectstore index dirs) and every spacecore storage space id. The latter
// is authoritative for "every space that could hold data" and is independent
// of the objectstore index; the former covers index dirs with no matching
// raw storage. Either source failing degrades coverage but never blocks.
func (s *dsObjectStore) authoritativeSpaceIds() []string {
	seen := map[string]struct{}{}
	var ids []string
	add := func(list []string) {
		for _, id := range list {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	fsIds, err := s.anystoreProvider.ListSpaceIdsFromFilesystem()
	if err != nil {
		log.Error("list space ids from filesystem", zap.Error(err))
	}
	add(fsIds)
	if s.spaceStorageLister != nil {
		storageIds, serr := s.spaceStorageLister.AllSpaceIds()
		if serr != nil {
			log.Error("list space ids from spacestorage", zap.Error(serr))
		} else {
			add(storageIds)
		}
	}
	return ids
}

// preloadExistingObjectStores opens every authoritative space's per-space DB
// with bounded concurrency. It is the body of the background warm-up; it
// never runs on a query hot path and never blocks Run().
func (s *dsObjectStore) preloadExistingObjectStores() {
	s.spaceStoreDirsCheck.Do(func() {
		spaceIds := s.authoritativeSpaceIds()
		sem := make(chan struct{}, preloadConcurrency)
		var wg sync.WaitGroup
		for _, spaceId := range spaceIds {
			select {
			case <-s.componentCtx.Done():
				wg.Wait()
				return
			case sem <- struct{}{}:
			}
			wg.Add(1)
			go func(spaceId string) {
				defer wg.Done()
				defer func() { <-sem }()
				// SpaceIndex opens the per-space DB and, on success,
				// calls markSpaceIndexOpened (fires OnSpaceIndexOpened).
				// On Init error it returns an invalid store and the space
				// is left out of OpenedSpaceIds (intended).
				s.SpaceIndex(spaceId)
			}(spaceId)
		}
		wg.Wait()
	})
}

// backgroundWarmUp runs the bounded preload and signals completion. Launched
// as a goroutine from Run so component startup is never blocked.
func (s *dsObjectStore) backgroundWarmUp() {
	defer close(s.loadedCh)
	s.preloadExistingObjectStores()
}

// WaitStoresLoaded blocks until the background warm-up has opened every
// authoritative-set store, or ctx / the component context is done. Safe to
// call from any non-Run goroutine. Designed to be extended later to also
// await per-space indexation.
func (s *dsObjectStore) WaitStoresLoaded(ctx context.Context) error {
	select {
	case <-s.loadedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.componentCtx.Done():
		return s.componentCtx.Err()
	}
}

func (s *dsObjectStore) listStores() []spaceindex.Store {
	s.lock.Lock()
	stores := make([]spaceindex.Store, 0, len(s.spaceIndexes))
	for _, store := range s.spaceIndexes {
		stores = append(stores, store)
	}
	s.lock.Unlock()
	return stores
}

func collectCrossSpace[T any](s *dsObjectStore, proc func(store spaceindex.Store) ([]T, error)) ([]T, error) {
	// Wait-by-default: every cross-space query (QueryCrossSpace,
	// QueryByIdCrossSpace, ListIdsCrossSpace) blocks until the background
	// warm-up has opened the full authoritative space set, so callers can
	// never silently act on a partial local view. The wait is one-time
	// (loadedCh stays closed afterwards, so this is instant) and is bound to
	// componentCtx because these APIs have no ctx parameter. The warm-up
	// goroutine itself never reaches here (it uses SpaceIndex directly and
	// its OnSpaceIndexOpened callback only does per-space subscription work),
	// so this cannot self-deadlock.
	if err := s.WaitStoresLoaded(s.componentCtx); err != nil {
		return nil, fmt.Errorf("wait stores loaded: %w", err)
	}
	stores := s.listStores()

	var result []T
	for _, store := range stores {
		err := store.Init()
		if err != nil {
			return nil, fmt.Errorf("init store: %w", err)
		}
		s.markSpaceIndexOpened(store.SpaceId())
		items, err := proc(store)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}
	return result, nil
}

// iterateSpacesForFulltext deliberately does NOT wait for the warm-up
// (unlike collectCrossSpace / IterateSpaceIndex): full-text enqueue/recheck
// is non-destructive and self-healing — a space missed here is re-enqueued
// when it opens / on its next indexer pass — so blocking the FT path on the
// full authoritative set would only add startup latency for no correctness
// gain.
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
		s.markSpaceIndexOpened(store.SpaceId())
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
			index := s.SpaceIndex(id.SpaceID)
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
