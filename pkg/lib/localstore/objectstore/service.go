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
	"strings"
	"sync"
	"sync/atomic"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/text/collate"

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
)

var log = logging.Logger("anytype-localstore")

const CName = "objectstore"

var (
	_ ObjectStore = (*dsObjectStore)(nil)
)

// ErrSpaceDeleted is returned (wrapped in an invalid store) by SpaceIndex when
// the space is being or has been removed via DeleteSpaceIndex, so callers do
// not resurrect the just-deleted store/DB/directory.
var ErrSpaceDeleted = errors.New("space index is deleted")

type CrossSpace interface {
	QueryCrossSpace(ctx context.Context, q database.Query) (records []database.Record, err error)
	// QueryCrossSpaceNoWait queries only the spaces whose object stores are
	// open right now, without waiting for the background warm-up (stores load
	// sequentially on app start). Per-space queries run concurrently with a
	// small cap. allStoresLoaded reports whether the result
	// covers the full authoritative space set: false when the warm-up had not
	// finished OR when a space's store failed and was skipped (failures are
	// logged, never fail the whole read). The tech space and the marketplace
	// are excluded, matching the fulltext iteration and the cross-space
	// subscription. q.SpaceId is overwritten per space (the fulltext scope
	// must follow the store being queried); q.Offset/q.Limit are applied
	// after the cross-space merge (per space they only bound candidates).
	// The merged order reproduces the order each store cut its candidates
	// with — the requested sorts plus the implicit score-first order of
	// fulltext queries — with a final id tiebreak for stable paging.
	QueryCrossSpaceNoWait(ctx context.Context, q database.Query) (records []database.Record, allStoresLoaded bool, err error)
	QueryByIdCrossSpace(ctx context.Context, ids []string) (records []database.Record, err error)

	ListIdsCrossSpace(ctx context.Context) ([]string, error)
	EnqueueAllForFulltextIndexing(ctx context.Context) error
	BatchProcessFullTextQueue(spaceIds func() []string, limit uint, processIds domain.FullTextProcessFunc) error

	AccountStore
	VirtualSpacesStore
	IndexerStore
}

type ObjectStore interface {
	app.ComponentRunnable

	IterateSpaceIndex(ctx context.Context, f func(store spaceindex.Store) error) error
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
	FtQueueMarkAsIndexed(objects []domain.FullTextQueuedObject, ftIndexSeq uint64) error

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
	FtQueueMarkAsIndexed(objects []domain.FullTextQueuedObject, ftIndexSeq uint64) error

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
	// RunFTConsistencyCheck checks all objects in the object store against the FT index,
	// enqueues missing ones for FT indexing and garbage-collects orphaned FT docs.
	// complete is false when coverage was partial (truncated listing, skipped space,
	// capped orphan GC) — the caller must not persist the recheck counter then.
	RunFTConsistencyCheck(ctx context.Context, fts ftsearch.FTSearch) (checked, enqueued int, complete bool, err error)
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
	// deletedSpaceIds is a tombstone set guarded by lock. A space is added
	// here at the very start of DeleteSpaceIndex (before the store is closed
	// and its files removed) so that any concurrent getOrInitSpaceIndex /
	// SpaceIndex refuses to recreate the store/DB/dir for a space that is
	// being or has been deleted (TOCTOU resurrection guard).
	deletedSpaceIds map[string]struct{}
	// crossSpaceInflight counts cross-space / full-text iterations that have
	// snapshotted the spaceIndexes registry and are still operating on the
	// snapshotted stores OUTSIDE lock. DeleteSpaceIndex waits (via
	// crossSpaceDrained) for this to reach zero before it closes a store and
	// removes its files, so an in-flight iterator can never use-after-close a
	// store whose anystore DB files were just os.RemoveAll'd.
	crossSpaceInflight int
	crossSpaceDrained  *sync.Cond

	spaceOpenedLock           sync.Mutex
	openedSpaceIds            map[string]struct{}
	spaceIndexOpenedCallbacks []func(spaceId string)

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func (s *dsObjectStore) ProvideStat() any {
	count, err := s.ListIdsCrossSpace(s.componentCtx)
	if err != nil {
		return 0
	}
	return len(count)
}

func (s *dsObjectStore) StatId() string {
	return "ds_count"
}

func (s *dsObjectStore) StatType() string {
	return CName
}

func (s *dsObjectStore) IterateSpaceIndex(ctx context.Context, f func(store spaceindex.Store) error) error {
	// Wait-by-default (see collectCrossSpace): never iterate a partial set
	// of space indexes.
	if err := s.WaitStoresLoaded(ctx); err != nil {
		return fmt.Errorf("wait stores loaded: %w", err)
	}
	spaceIndexes := s.beginCrossSpaceIteration()
	defer s.endCrossSpaceIteration()
	for _, store := range spaceIndexes {
		if err := f(store); err != nil {
			return err
		}
	}
	return nil
}

func New() ObjectStore {
	ctx, cancel := context.WithCancel(context.Background())
	s := &dsObjectStore{
		componentCtx:       ctx,
		componentCtxCancel: cancel,
		subManager:         &spaceindex.SubscriptionManager{},
		spaceIndexes:       map[string]spaceindex.Store{},
		deletedSpaceIds:    map[string]struct{}{},
		openedSpaceIds:     map[string]struct{}{},
		loadedCh:           make(chan struct{}),
	}
	s.crossSpaceDrained = sync.NewCond(&s.lock)
	return s
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
	spaceIndex, ok := s.getOrInitSpaceIndex(spaceId)
	s.lock.Unlock()
	if !ok {
		// Space is tombstoned (being / already deleted): refuse to reopen it
		// so a racing caller cannot resurrect the store/DB/directory that
		// DeleteSpaceIndex is removing.
		return spaceindex.NewInvalidStore(ErrSpaceDeleted)
	}
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
//
// Concurrency: the space is tombstoned first (under s.lock) so that any racing
// SpaceIndex / cross-space iteration refuses to reopen it (preventing TOCTOU
// resurrection of the DB/dir, and preventing markSpaceIndexOpened from re-adding
// it to openedSpaceIds). We then wait for all in-flight cross-space iterations
// that may already hold a reference to this store to drain before closing it and
// removing its files, preventing a use-after-close / removed-file access from
// those background loops.
//
// Remaining barrier work (documented, not yet covered): direct callers of
// SpaceIndex(spaceId) that capture the returned store and keep using it across
// this call (e.g. core/indexer/fulltext.go prepareSearchDocs) are not part of
// the cross-space in-flight set, so a reference obtained in the narrow window
// just before the tombstone could still be used while the DB is closed. Fully
// closing that window requires a per-store "closing" RWMutex read-held by every
// spaceindex query/write entrypoint and write-held by Close; that larger
// refactor is deferred. The tombstone already prevents the more damaging
// resurrection/leak, and the drain barrier covers the always-running
// cross-space/FT iterators that are the most frequent racers.
func (s *dsObjectStore) DeleteSpaceIndex(spaceId string) error {
	if spaceId == "" {
		return errors.New("empty spaceId")
	}

	s.lock.Lock()
	// Tombstone first so concurrent getOrInitSpaceIndex/markSpaceIndexOpened
	// short-circuit and cannot resurrect the space.
	s.deletedSpaceIds[spaceId] = struct{}{}
	store, ok := s.spaceIndexes[spaceId]
	if ok {
		delete(s.spaceIndexes, spaceId)
	}
	// Wait for in-flight cross-space iterations to finish: any snapshot that
	// captured this store before the tombstone must release it before we close
	// the store and remove its files. New iterations started after the
	// tombstone exclude this space (it is no longer in s.spaceIndexes).
	for s.crossSpaceInflight > 0 {
		s.crossSpaceDrained.Wait()
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

	// Lift the tombstone once the on-disk data is fully removed. At this point
	// the store is closed and DeleteSpaceData's os.RemoveAll has completed, so a
	// subsequent SpaceIndex(spaceId) can only create a brand-new empty store/DB
	// (e.g. when the same space is re-joined within the session) — there is no
	// removed-files window left to race. We keep the tombstone on error so a
	// failed delete (retried by the offloader every 20s) still refuses re-opens
	// until the removal actually succeeds.
	if errs == nil {
		s.lock.Lock()
		delete(s.deletedSpaceIds, spaceId)
		s.lock.Unlock()
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
	// Refuse to (re-)mark a tombstoned space as opened. collectCrossSpace /
	// iterateSpacesForFulltext call this on a store captured from a registry
	// snapshot; without this guard a snapshot taken just before
	// DeleteSpaceIndex could re-add the deleted space to openedSpaceIds and
	// re-fire OnSpaceIndexOpened for a space whose data was just removed.
	s.lock.Lock()
	_, deleted := s.deletedSpaceIds[spaceId]
	s.lock.Unlock()
	if deleted {
		return
	}

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

// getOrInitSpaceIndex returns the in-memory store for spaceId, creating it on
// demand. Must be called with s.lock held. The returned bool is false when the
// space is tombstoned (being / already deleted via DeleteSpaceIndex): in that
// case no store is created and the caller must NOT open the space, otherwise it
// would resurrect the just-removed DB/directory.
func (s *dsObjectStore) getOrInitSpaceIndex(spaceId string) (spaceindex.Store, bool) {
	if _, deleted := s.deletedSpaceIds[spaceId]; deleted {
		return nil, false
	}
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
	return store, true
}

// preloadConcurrencyDefault bounds parallel per-space store opens during
// warm-up. >1 so a single slow/stuck space Init does not serialize the rest
// of the warm-up (cross-space reads block until the whole authoritative set
// is open; serializing on a bad space would stall all of them). Direct
// SpaceIndex calls are unaffected (own goroutine, not semaphore-gated) and
// share the idempotent per-space Init lock only for the same space.
const preloadConcurrencyDefault = 4

// crossSpaceQueryConcurrency caps parallel per-space queries in
// QueryCrossSpaceNoWait — same rationale as the preload cap: overlap the
// per-space I/O without letting a wide vault fan out unboundedly.
const crossSpaceQueryConcurrency = 4

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
		if s.componentCtx.Err() != nil {
			return // shutting down: don't bother listing/opening anything
		}
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
				// is left out of OpenedSpaceIds (intended). Init on the
				// returned store is idempotent and recovers that error so
				// the warm-up failure is diagnosable; later cross-space
				// reads hitting the bad store surface the same error.
				if initErr := s.SpaceIndex(spaceId).Init(); initErr != nil {
					log.Error("warm-up: init space index",
						zap.String("spaceId", spaceId), zap.Error(initErr))
				}
			}(spaceId)
		}
		wg.Wait()
	})
}

// backgroundWarmUp runs the bounded preload and signals completion. Launched
// as a goroutine from Run so component startup is never blocked.
//
// loadedCh is only closed when the preload actually covered the full
// authoritative set. A warm-up aborted by shutdown must NOT signal "loaded":
// WaitStoresLoaded returning nil over a partial store set would let
// destructive cross-space callers act on partial data. Aborted waiters
// unblock through the componentCtx case of WaitStoresLoaded, with an error.
func (s *dsObjectStore) backgroundWarmUp() {
	s.preloadExistingObjectStores()
	if s.componentCtx.Err() != nil {
		return
	}
	close(s.loadedCh)
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

// beginCrossSpaceIteration snapshots the currently-registered space indexes
// and registers an in-flight cross-space iteration. The snapshot and the
// counter increment happen atomically under s.lock so that DeleteSpaceIndex,
// which sets a tombstone and then waits for crossSpaceInflight to reach zero,
// can never close a store (and os.RemoveAll its DB files) while a snapshot that
// already captured that store is still being iterated. Every call MUST be
// paired with exactly one deferred endCrossSpaceIteration.
func (s *dsObjectStore) beginCrossSpaceIteration() []spaceindex.Store {
	s.lock.Lock()
	stores := make([]spaceindex.Store, 0, len(s.spaceIndexes))
	for _, store := range s.spaceIndexes {
		stores = append(stores, store)
	}
	s.crossSpaceInflight++
	s.lock.Unlock()
	return stores
}

// endCrossSpaceIteration releases an in-flight cross-space iteration and wakes
// any DeleteSpaceIndex waiting for the iterations to drain.
func (s *dsObjectStore) endCrossSpaceIteration() {
	s.lock.Lock()
	s.crossSpaceInflight--
	if s.crossSpaceInflight == 0 {
		s.crossSpaceDrained.Broadcast()
	}
	s.lock.Unlock()
}

func collectCrossSpace[T any](ctx context.Context, s *dsObjectStore, proc func(store spaceindex.Store) ([]T, error)) ([]T, error) {
	// Wait-by-default: every cross-space query (QueryCrossSpace,
	// QueryByIdCrossSpace, ListIdsCrossSpace) blocks until the background
	// warm-up has opened the full authoritative space set, so callers can
	// never silently act on a partial local view. The wait is one-time
	// (loadedCh stays closed afterwards, so this is instant) and honors the
	// caller's ctx (cancellation/timeout of the request aborts the wait).
	// The warm-up goroutine itself never reaches here (it uses SpaceIndex
	// directly and its OnSpaceIndexOpened callback only does per-space
	// subscription work), so this cannot self-deadlock.
	if err := s.WaitStoresLoaded(ctx); err != nil {
		return nil, fmt.Errorf("wait stores loaded: %w", err)
	}
	stores := s.beginCrossSpaceIteration()
	defer s.endCrossSpaceIteration()

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
	stores := s.beginCrossSpaceIteration()
	defer s.endCrossSpaceIteration()
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

func (s *dsObjectStore) ListIdsCrossSpace(ctx context.Context) ([]string, error) {
	return collectCrossSpace(ctx, s, func(store spaceindex.Store) ([]string, error) {
		return store.ListIds()
	})
}

func (s *dsObjectStore) EnqueueAllForFulltextIndexing(ctx context.Context) error {
	// Full FT rebuild must cover every space, not just those already open,
	// so wait for the warm-up here (unlike the rest of the FT path, which
	// stays lazy — see iterateSpacesForFulltext).
	if err := s.WaitStoresLoaded(ctx); err != nil {
		return fmt.Errorf("wait stores loaded: %w", err)
	}
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

	// one generation for the whole rebuild pass is enough: it only has to differ
	// from generations captured by listings that happened before this enqueue
	gen := GenerateFTQueueCounter()
	err = iterateSpacesForFulltext(s, func(store spaceindex.Store) error {
		err := store.IterateAll(func(doc *anyenc.Value) error {
			id := doc.GetString(idKey)
			spaceId := doc.GetString(spaceIdKey)

			arena.Reset()
			obj := arena.NewObject()
			obj.Set(idKey, arena.NewString(id))
			obj.Set(spaceIdKey, arena.NewString(spaceId))
			obj.Set(ftSequenceKey, arena.NewBinary(emptyBuffer))
			obj.Set(ftGenKey, arena.NewNumberFloat64(float64(gen)))
			// Chat objects keep their searchable text in messages, not in object
			// relations/blocks. Tag them with FtAllOrderId so a full FT rebuild
			// reindexes the whole message history via indexChatMessages;
			// otherwise the consume path treats them as a regular object and chat
			// search stays empty after the index is rebuilt (GO-7316). Both
			// chatDerived (space chats) and discussion (object chats) use the chat
			// editor and store messages the same way, so both must be tagged.
			switch model.ObjectTypeLayout(doc.GetInt(bundle.RelationKeyResolvedLayout.String())) {
			case model.ObjectType_chatDerived, model.ObjectType_discussion:
				obj.Set(ftOrderIdKey, arena.NewString(FtAllOrderId))
			}
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

const (
	// ftConsistencyListLimit bounds the per-space FT doc id listing. A space
	// with more docs gets its orphan GC skipped (we can't tell orphans from
	// docs beyond the page) but the missing-check still runs.
	ftConsistencyListLimit = 1_000_000
	// ftOrphanGCLimit caps how many orphaned objects a single check deletes,
	// as a safety valve against a runaway deletion; the rest is collected on
	// the next session's check (the run reports itself incomplete).
	ftOrphanGCLimit = 50_000
	// ftOrphanStoreRatio guards against GCing a wiped/not-yet-rebuilt store:
	// when the FT index holds more than this many times the store's objects,
	// the mismatch is far more likely a store in a transient state than
	// genuine orphans, so the space's GC is skipped and retried next session.
	ftOrphanStoreRatio = 10
	// ftConsistencyCtxCheckEvery bounds how often the store iteration checks
	// for cancellation, so app shutdown isn't blocked by a full sweep.
	ftConsistencyCtxCheckEvery = 1000
)

// RunFTConsistencyCheck checks all objects in the object store against the full-text index,
// enqueues missing objects for FT indexing and garbage-collects FT documents whose object
// (or space) no longer exists in the store. This is a lightweight consistency check
// that doesn't load objects into cache. complete reports whether the whole index was
// covered: a truncated listing, a skipped space or a capped orphan GC returns false so
// the caller does not persist the recheck counter and the next session finishes the job.
func (s *dsObjectStore) RunFTConsistencyCheck(ctx context.Context, fts ftsearch.FTSearch) (checked, enqueued int, complete bool, err error) {
	var (
		missingIds      []domain.FullID
		orphanObjectIds []string
	)
	complete = true

	var spaces, objs int
	err = iterateSpacesForFulltext(s, func(store spaceindex.Store) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("consistency check canceled: %w", err)
		}
		spaces++
		spaceId := store.SpaceId()

		// collect object ids present in the FT index for this space
		ftDocIds, err := fts.ListIdsBySpace(spaceId, ftConsistencyListLimit)
		if err != nil {
			return fmt.Errorf("list ft ids for space: %w", err)
		}
		listingComplete := len(ftDocIds) < ftConsistencyListLimit
		ftObjectIds := make(map[string]struct{})
		for _, docId := range ftDocIds {
			if idx := strings.Index(docId, "/"); idx > 0 {
				ftObjectIds[docId[:idx]] = struct{}{}
			} else {
				ftObjectIds[docId] = struct{}{}
			}
		}

		storeIds := make(map[string]struct{})
		err = store.IterateAll(func(doc *anyenc.Value) error {
			objs++
			if objs%ftConsistencyCtxCheckEvery == 0 {
				if err := ctx.Err(); err != nil {
					return fmt.Errorf("consistency check canceled: %w", err)
				}
			}
			id := doc.GetString(idKey)
			if doc.GetBool(bundle.RelationKeyIsDeleted.String()) {
				// soft-deleted objects keep a store stub; their leftover FT
				// docs are garbage and must stay collectable as orphans
				return nil
			}
			storeIds[id] = struct{}{}
			if !isFtIndexable(id, doc) {
				return nil
			}

			checked++
			if _, exists := ftObjectIds[id]; !exists {
				missingIds = append(missingIds, domain.FullID{ObjectID: id, SpaceID: spaceId})
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("iterate space objects: %w", err)
		}

		// FT docs whose object is gone from the store are orphans. Only spaces
		// iterated here are considered — docs of not-yet-loaded spaces are never
		// touched — and only when the FT listing was complete.
		switch {
		case !listingComplete:
			complete = false
			log.With("spaceId", spaceId).Warn("ft consistency check: doc listing truncated, skipping orphan gc for space")
		case len(ftObjectIds) > ftOrphanStoreRatio*len(storeIds):
			// the store holds a small fraction of the FT objects: far more
			// likely a wiped/not-yet-rebuilt store than genuine orphans
			complete = false
			log.With("spaceId", spaceId).With("ftObjects", len(ftObjectIds)).With("storeObjects", len(storeIds)).
				Warn("ft consistency check: store is implausibly sparse, skipping orphan gc for space")
		default:
			for objectId := range ftObjectIds {
				if _, exists := storeIds[objectId]; !exists {
					orphanObjectIds = append(orphanObjectIds, objectId)
				}
			}
		}
		return nil
	})
	log.With("objects", objs).With("spaces", spaces).With("missing", len(missingIds)).With("orphans", len(orphanObjectIds)).
		Debug("ft consistency check: iteration finished")
	if err != nil {
		return checked, 0, false, fmt.Errorf("iterate objects: %w", err)
	}

	if len(orphanObjectIds) > 0 {
		if len(orphanObjectIds) > ftOrphanGCLimit {
			complete = false
			log.With("orphans", len(orphanObjectIds)).Warn("ft consistency check: orphan count exceeds gc limit, deleting partially")
			orphanObjectIds = orphanObjectIds[:ftOrphanGCLimit]
		}
		// chunked: each BatchDeleteObjects call holds the ftsearch write lock,
		// so one huge call would block searches for its whole duration
		const orphanDeleteChunk = 1000
		for start := 0; start < len(orphanObjectIds); start += orphanDeleteChunk {
			end := start + orphanDeleteChunk
			if end > len(orphanObjectIds) {
				end = len(orphanObjectIds)
			}
			if err = fts.BatchDeleteObjects(orphanObjectIds[start:end]); err != nil {
				return checked, 0, false, fmt.Errorf("delete orphaned ft docs: %w", err)
			}
		}
	}

	// Batch enqueue all missing IDs at once
	if len(missingIds) > 0 {
		_, enqueued, err = s.AddToIndexQueue(ctx, missingIds...)
		if err != nil {
			return checked, 0, false, fmt.Errorf("batch enqueue: %w", err)
		}
	}

	return checked, enqueued, complete, nil
}

func (s *dsObjectStore) QueryByIdCrossSpace(ctx context.Context, ids []string) ([]database.Record, error) {
	return collectCrossSpace(ctx, s, func(store spaceindex.Store) ([]database.Record, error) {
		return store.QueryByIds(ids)
	})
}

func (s *dsObjectStore) QueryCrossSpace(ctx context.Context, q database.Query) ([]database.Record, error) {
	return collectCrossSpace(ctx, s, func(store spaceindex.Store) ([]database.Record, error) {
		return store.Query(q)
	})
}

func (s *dsObjectStore) QueryCrossSpaceNoWait(ctx context.Context, q database.Query) ([]database.Record, bool, error) {
	// negative paging from the wire must not disable the per-space candidate
	// bound (a negative limit reads as unlimited downstream)
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.Limit < 0 {
		q.Limit = 0
	}

	// read the flag before snapshotting the store set: if the warm-up had
	// finished by then, the snapshot is guaranteed to cover the full set
	allStoresLoaded := false
	select {
	case <-s.loadedCh:
		allStoresLoaded = true
	default:
	}

	// per space the caller's paging only bounds candidates: each space
	// returns its top offset+limit records under the same order the merge
	// applies below, so the global top offset+limit is a subset of their
	// union (top-K merge property) and the real slicing happens after the
	// merge
	perSpaceLimit := 0
	if q.Limit > 0 {
		perSpaceLimit = q.Offset + q.Limit
	}

	stores := s.beginCrossSpaceIteration()
	defer s.endCrossSpaceIteration()
	// deterministic space order: the snapshot iterates a map, ties in the
	// merged sort fall back to concatenation order, and offset paging must be
	// stable across identical calls
	slices.SortFunc(stores, func(a, b spaceindex.Store) int {
		return strings.Compare(a.SpaceId(), b.SpaceId())
	})

	// parity with iterateSpacesForFulltext and the cross-space subscription:
	// system spaces are not user-facing search results
	candidates := stores[:0]
	for _, store := range stores {
		if store.SpaceId() == s.techSpaceId || store.SpaceId() == addr.AnytypeMarketplaceWorkspace {
			continue
		}
		candidates = append(candidates, store)
	}

	// per-space queries run concurrently with a small cap (same rationale as
	// the warm-up's preload concurrency: one slow space must not serialize
	// the rest); results are collected per slot so the concatenation order —
	// which every merged-sort tie falls back to — stays the deterministic
	// sorted-space order regardless of completion order
	perSlot := make([][]database.Record, len(candidates))
	succeeded := make([]bool, len(candidates))
	var skippedSpace atomic.Bool
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(crossSpaceQueryConcurrency)
	for i, store := range candidates {
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			if err := store.Init(); err != nil {
				log.Error("cross-space query: init store", zap.String("spaceId", store.SpaceId()), zap.Error(err))
				// a skipped space makes the view partial
				skippedSpace.Store(true)
				return nil
			}
			s.markSpaceIndexOpened(store.SpaceId())
			perSpaceQuery := q
			perSpaceQuery.Offset = 0
			perSpaceQuery.Limit = perSpaceLimit
			// scope the fulltext to the space being queried: without this
			// every space runs the same global search, resolves the global
			// candidate list against its own store and drops everyone else's
			// hits — spaces whose matches don't rank in the global top
			// candidates silently return nothing, at N× the cost
			perSpaceQuery.SpaceId = store.SpaceId()
			// the per-space filter compilation mutates nested filters in
			// place: give every space its own copy
			perSpaceQuery.Filters = slices.Clone(q.Filters)
			items, err := store.Query(perSpaceQuery)
			if err != nil {
				log.Error("cross-space query: query store", zap.String("spaceId", store.SpaceId()), zap.Error(err))
				skippedSpace.Store(true)
				return nil
			}
			perSlot[i] = items
			succeeded[i] = true
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, false, fmt.Errorf("cross-space query: %w", err)
	}
	if skippedSpace.Load() {
		allStoresLoaded = false
	}

	var records []database.Record
	queried := make([]spaceindex.Store, 0, len(candidates))
	for i, store := range candidates {
		if !succeeded[i] {
			continue
		}
		queried = append(queried, store)
		records = append(records, perSlot[i]...)
	}

	s.sortMergedRecords(records, q, queried)

	if q.Offset > 0 {
		if q.Offset >= len(records) {
			return nil, allStoresLoaded, nil
		}
		records = records[q.Offset:]
	}
	if q.Limit > 0 && len(records) > q.Limit {
		records = records[:q.Limit]
	}
	return records, allStoresLoaded, nil
}

// sortMergedRecords restores a global order over records merged from several
// spaces, reproducing exactly the order each store cut its candidates with:
// database.InjectDefaultOrder supplies the implicit score-first order of
// fulltext queries and database.NewSetOrder the per-sort includeTime and
// custom-order handling — without this the per-space cut and the merged
// order disagree and offset paging duplicates and drops records. Relation
// formats and option/object order maps are resolved across all queried
// stores (an option or linked object lives only in its record's own space).
// Ties end on the object id so the order is stable across calls.
func (s *dsObjectStore) sortMergedRecords(records []database.Record, q database.Query, queried []spaceindex.Store) {
	if len(records) < 2 {
		return
	}
	var order database.Order
	sorts := database.InjectDefaultOrder(q, q.Sorts)
	if len(sorts) > 0 && len(queried) > 0 {
		arena := s.arenaPool.Get()
		defer s.arenaPool.Put(arena)
		order = database.NewSetOrder(unionOrderStore{stores: queried}, arena, &collate.Buffer{}, sorts)
	}
	slices.SortStableFunc(records, func(a, b database.Record) int {
		if order != nil {
			if comp := order.Compare(a.Details, b.Details); comp != 0 {
				return comp
			}
		}
		return strings.Compare(a.Details.GetString(bundle.RelationKeyId), b.Details.GetString(bundle.RelationKeyId))
	})
}

// unionOrderStore lets the merge's order builder resolve relation formats and
// option/object order maps across every queried space instead of a single
// representative one. Only the methods the order path uses do real cross-store
// work (GetRelationFormatByKey, ListRelationOptions, QueryIterate); the rest
// delegate per store and concatenate.
type unionOrderStore struct {
	stores []spaceindex.Store
}

func (u unionOrderStore) SpaceId() string {
	return ""
}

func (u unionOrderStore) Query(q database.Query) ([]database.Record, error) {
	var records []database.Record
	for _, store := range u.stores {
		items, err := store.Query(q)
		if err != nil {
			return nil, fmt.Errorf("query space %s: %w", store.SpaceId(), err)
		}
		records = append(records, items...)
	}
	return records, nil
}

func (u unionOrderStore) QueryRaw(filters *database.Filters, limit int, offset int) ([]database.Record, error) {
	var records []database.Record
	for _, store := range u.stores {
		items, err := store.QueryRaw(filters, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("query raw space %s: %w", store.SpaceId(), err)
		}
		records = append(records, items...)
	}
	return records, nil
}

func (u unionOrderStore) QueryIterate(q database.Query, proc func(details *domain.Details)) error {
	for _, store := range u.stores {
		if err := store.QueryIterate(q, proc); err != nil {
			return fmt.Errorf("iterate space %s: %w", store.SpaceId(), err)
		}
	}
	return nil
}

func (u unionOrderStore) GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error) {
	var lastErr error
	for _, store := range u.stores {
		format, err := store.GetRelationFormatByKey(key)
		if err == nil {
			return format, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("relation format not found in any space: %w", lastErr)
}

func (u unionOrderStore) ListRelationOptions(relationKey domain.RelationKey) ([]*model.RelationOption, error) {
	var options []*model.RelationOption
	for _, store := range u.stores {
		items, err := store.ListRelationOptions(relationKey)
		if err != nil {
			// an option set missing in one space must not void the others
			log.Warn("cross-space order: list relation options", zap.String("spaceId", store.SpaceId()), zap.Error(err))
			continue
		}
		options = append(options, items...)
	}
	return options, nil
}

func (s *dsObjectStore) SubscribeLinksUpdate(callback func(info spaceindex.LinksUpdateInfo)) {
	s.subManager.SubscribeLinksUpdate(callback)
}

// WriteTx returns a new write transaction for commonDb
func (s *dsObjectStore) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return s.db.WriteTx(ctx)
}
