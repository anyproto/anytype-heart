package objectstore

import (
	"context"
	"errors"
	"fmt"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceresolverstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
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
	BatchProcessFullTextQueue(ctx context.Context, spaceIds func() []string, limit uint, processIds func(objectIds []domain.FullID) (succeedIds []domain.FullID, ftIndexSeq uint64, err error)) error

	AccountStore
	VirtualSpacesStore
	IndexerStore
}

type ObjectStore interface {
	app.ComponentRunnable

	IterateSpaceIndex(func(store spaceindex.Store) error) error
	SpaceIndex(spaceId string) spaceindex.Store
	InitSpaceIndex(spaceId string) error
	CloseSpaceIndex(spaceId string) error
	DeleteSpaceIndex(spaceId string) error // Deactivate + delete from disk

	SpaceNameGetter
	spaceresolverstore.Store
	CrossSpace

	FtQueueReconcileWithSeq(ctx context.Context, ftIndexSeq uint64) error
	FtQueueMarkAsIndexed(ids []domain.FullID, ftIndexSeq uint64) error

	AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error
	GetFileKeys(fileId domain.FileId) (map[string]string, error)
}

type IndexerStore interface {
	AddToIndexQueue(ctx context.Context, id ...domain.FullID) error
	ListIdsFromFullTextQueue(spaceIds []string, limit uint) ([]domain.FullID, error)
	FtQueueMarkAsIndexed(ids []domain.FullID, ftIndexSeq uint64) error

	// ClearFullTextQueue cleans the pending . Pass nil to clear all spaces.
	ClearFullTextQueue(spaceIds []string) error

	// GetChecksums Used to get information about localstore state and decide do we need to reindex some objects
	GetChecksums(spaceID string) (checksums *model.ObjectStoreChecksums, err error)
	// SaveChecksums Used to save checksums and force reindex counter
	SaveChecksums(spaceID string, checksums *model.ObjectStoreChecksums) (err error)
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

	// Preload existing spaces from disk
	if err := s.preloadExistingSpaces(); err != nil {
		log.Error("preload existing spaces", zap.Error(err))
		// Non-fatal: continue even if preload fails
	}

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
	return err
}

func (s *dsObjectStore) SpaceIndex(spaceId string) spaceindex.Store {
	s.lock.Lock()
	spaceIndex := s.getOrInitSpaceIndex(spaceId)
	s.lock.Unlock()
	return spaceIndex
}

func (s *dsObjectStore) InitSpaceIndex(spaceId string) error {
	if spaceId == "" {
		return errors.New("empty spaceId")
	}
	s.lock.Lock()
	spaceIndex := s.getOrInitSpaceIndex(spaceId)
	s.lock.Unlock()
	return spaceIndex.Init()
}

func (s *dsObjectStore) CloseSpaceIndex(spaceId string) error {
	s.lock.Lock()
	store, ok := s.spaceIndexes[spaceId]
	s.lock.Unlock()

	if !ok {
		return nil
	}
	return store.Close()
}

// DeleteSpaceIndex deactivates the space index (marks it as permanently removed)
// and deletes the database from filesystem.
// The proxy remains in the map (in deactivated state) to prevent re-initialization.
func (s *dsObjectStore) DeleteSpaceIndex(spaceId string) error {
	var errs error

	s.lock.Lock()
	spaceIndex, ok := s.spaceIndexes[spaceId]
	if ok {
		if err := spaceIndex.Deactivate(); err != nil {
			errs = errors.Join(errs, err)
		}
		// Don't delete from map - keep deactivated proxy to prevent re-initialization
	}
	s.lock.Unlock()

	// Delete the space data (DBs + directory) from filesystem
	if err := s.anystoreProvider.DeleteSpaceData(spaceId); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

// OnSpaceModeChange implements SpaceModeObserver interface.
// It initializes or closes the space index based on the mode transition.
func (s *dsObjectStore) OnSpaceModeChange(spaceId string, prevMode, newMode spaceinfo.SpaceMode) {
	switch newMode {
	case spaceinfo.SpaceModeLoading:
		if err := s.InitSpaceIndex(spaceId); err != nil {
			log.Error("init space index on mode change", zap.String("spaceId", spaceId), zap.Error(err))
		}
	case spaceinfo.SpaceModeOffloading:
		if err := s.CloseSpaceIndex(spaceId); err != nil {
			log.Error("close space index on mode change", zap.String("spaceId", spaceId), zap.Error(err))
		}
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

func (s *dsObjectStore) preloadExistingSpaces() error {
	spaceIds, err := s.anystoreProvider.ListSpaceIdsFromFilesystem()
	if err != nil {
		return fmt.Errorf("list space ids: %w", err)
	}

	var wg sync.WaitGroup
	for _, spaceId := range spaceIds {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if initErr := s.InitSpaceIndex(id); initErr != nil {
				log.With("spaceId", id, "error", initErr).Error("preload space index")
			}
		}(spaceId)
	}
	wg.Wait()
	return nil
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
	stores := s.listStores()

	var result []T
	for _, store := range stores {
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
		err := proc(store)
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
