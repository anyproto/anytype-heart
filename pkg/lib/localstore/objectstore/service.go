package objectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceresolverstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	BatchProcessFullTextQueue(ctx context.Context, limit int, processIds func(processIds []string) error) error

	AccountStore
	VirtualSpacesStore
	IndexerStore
}

type ObjectStore interface {
	app.ComponentRunnable

	IterateSpaceIndex(func(store spaceindex.Store) error) error
	SpaceIndex(spaceId string) spaceindex.Store
	GetCrdtDb(spaceId string) anystore.DB

	SpaceNameGetter
	spaceresolverstore.Store
	CrossSpace
}

type IndexerStore interface {
	AddToIndexQueue(ctx context.Context, id ...string) error
	ListIdsFromFullTextQueue(limit int) ([]string, error)
	RemoveIdsFromFullTextQueue(ids []string) error

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

type configProvider interface {
	GetAnyStoreConfig() *anystore.Config
}

type TechSpaceIdProvider interface {
	TechSpaceId() string
}

type dsObjectStore struct {
	spaceresolverstore.Store

	objectStorePath string
	techSpaceId     string
	anyStoreConfig  anystore.Config

	anyStore           anystore.DB
	anyStoreLockRemove func() error

	indexerChecksums anystore.Collection
	virtualSpaces    anystore.Collection
	system           anystore.Collection
	fulltextQueue    anystore.Collection

	arenaPool *anyenc.ArenaPool

	fts                 ftsearch.FTSearch
	subManager          *spaceindex.SubscriptionManager
	sourceService       spaceindex.SourceDetailsFromID
	oldStore            oldstore.Service
	techSpaceIdProvider TechSpaceIdProvider

	sync.Mutex
	spaceIndexes        map[string]spaceindex.Store
	spaceStoreDirsCheck sync.Once

	crtdStoreLock sync.Mutex
	crdtDbs       map[string]anystore.DB

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func (s *dsObjectStore) IterateSpaceIndex(f func(store spaceindex.Store) error) error {
	s.Lock()
	spaceIndexes := make([]spaceindex.Store, 0, len(s.spaceIndexes))
	for _, store := range s.spaceIndexes {
		spaceIndexes = append(spaceIndexes, store)
	}
	s.Unlock()
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
		crdtDbs:            map[string]anystore.DB{},
	}
}

func (s *dsObjectStore) Init(a *app.App) (err error) {
	s.sourceService = app.MustComponent[spaceindex.SourceDetailsFromID](a)

	repoPath := app.MustComponent[wallet.Wallet](a).RepoPath()

	fts := a.Component(ftsearch.CName)
	if fts == nil {
		log.Warnf("init objectstore without fulltext")
	} else {
		s.fts = fts.(ftsearch.FTSearch)
	}
	s.arenaPool = &anyenc.ArenaPool{}

	cfg := app.MustComponent[configProvider](a)
	s.objectStorePath = filepath.Join(repoPath, "objectstore")
	s.anyStoreConfig = *cfg.GetAnyStoreConfig()
	s.setDefaultConfig()
	s.oldStore = app.MustComponent[oldstore.Service](a)
	s.techSpaceIdProvider = app.MustComponent[TechSpaceIdProvider](a)

	return nil
}

func (s *dsObjectStore) Name() (name string) {
	return CName
}

func (s *dsObjectStore) Run(ctx context.Context) error {
	s.techSpaceId = s.techSpaceIdProvider.TechSpaceId()

	err := ensureDirExists(s.objectStorePath)
	if err != nil {
		return err
	}
	err = s.openDatabase(ctx, filepath.Join(s.objectStorePath, "objects.db"))
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	store, err := spaceresolverstore.New(s.componentCtx, s.anyStore)
	if err != nil {
		return fmt.Errorf("new space resolver store: %w", err)
	}

	s.Store = store

	return err
}

func (s *dsObjectStore) setDefaultConfig() {
	if s.anyStoreConfig.SQLiteConnectionOptions == nil {
		s.anyStoreConfig.SQLiteConnectionOptions = map[string]string{}
	}
	s.anyStoreConfig.SQLiteConnectionOptions = maps.Clone(s.anyStoreConfig.SQLiteConnectionOptions)
	s.anyStoreConfig.SQLiteConnectionOptions["synchronous"] = "off"
	s.anyStoreConfig.SQLiteGlobalPageCachePreallocateSizeBytes = 1 << 26
}

func ensureDirExists(dir string) error {
	_, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return fmt.Errorf("create db dir: %w", err)
		}
	}
	return nil
}

func (s *dsObjectStore) openDatabase(ctx context.Context, path string) error {
	store, lockRemove, err := anystorehelper.OpenDatabaseWithLockCheck(ctx, path, s.getAnyStoreConfig())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	fulltextQueue, err := store.Collection(ctx, "fulltext_queue")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open fulltextQueue collection: %w", err))
	}
	system, err := store.Collection(ctx, "system")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open system collection: %w", err))
	}
	indexerChecksums, err := store.Collection(ctx, "indexerChecksums")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open indexerChecksums collection: %w", err))
	}
	virtualSpaces, err := store.Collection(ctx, "virtualSpaces")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open virtualSpaces collection: %w", err))
	}

	s.anyStore = store
	s.anyStoreLockRemove = lockRemove

	s.fulltextQueue = fulltextQueue
	s.system = system
	s.indexerChecksums = indexerChecksums
	s.virtualSpaces = virtualSpaces

	return nil
}

// preloadExistingObjectStores loads all existing object stores from the filesystem
// this makes sense to do because spaces register themselves in the object store asynchronously and we may want to know the list before that
func (s *dsObjectStore) preloadExistingObjectStores() error {
	var err error
	s.spaceStoreDirsCheck.Do(func() {
		var entries []os.DirEntry
		entries, err = os.ReadDir(s.objectStorePath)
		s.Lock()
		defer s.Unlock()
		for _, entry := range entries {
			if entry.IsDir() {
				spaceId := entry.Name()
				_ = s.getOrInitSpaceIndex(spaceId)
			}
		}
	})
	return err
}

func (s *dsObjectStore) Close(_ context.Context) (err error) {
	s.componentCtxCancel()
	if s.anyStore != nil {
		err = errors.Join(err, s.anyStore.Close(), s.anyStoreLockRemove())
	}

	s.Lock()
	// close in parallel
	closeChan := make(chan error, len(s.spaceIndexes))
	for spaceId, store := range s.spaceIndexes {
		go func(spaceId string, store spaceindex.Store) {
			closeChan <- store.Close()
		}(spaceId, store)
	}
	for i := 0; i < len(s.spaceIndexes); i++ {
		err = errors.Join(err, <-closeChan)
	}
	s.spaceIndexes = map[string]spaceindex.Store{}
	s.Unlock()

	s.crtdStoreLock.Lock()
	closeChan = make(chan error, len(s.crdtDbs))
	for spaceId, store := range s.crdtDbs {
		go func(spaceId string, store anystore.DB) {
			closeChan <- store.Close()
		}(spaceId, store)
	}
	for i := 0; i < len(s.crdtDbs); i++ {
		err = errors.Join(err, <-closeChan)
	}
	s.crdtDbs = map[string]anystore.DB{}
	s.crtdStoreLock.Unlock()

	return err
}

func (s *dsObjectStore) SpaceIndex(spaceId string) spaceindex.Store {
	if spaceId == "" {
		return spaceindex.NewInvalidStore(errors.New("empty spaceId"))
	}
	s.Lock()
	defer s.Unlock()

	return s.getOrInitSpaceIndex(spaceId)
}

func (s *dsObjectStore) getOrInitSpaceIndex(spaceId string) spaceindex.Store {
	store, ok := s.spaceIndexes[spaceId]
	if !ok {
		dir := filepath.Join(s.objectStorePath, spaceId)
		err := ensureDirExists(dir)
		if err != nil {
			return spaceindex.NewInvalidStore(err)
		}
		store = spaceindex.New(s.componentCtx, spaceId, spaceindex.Deps{
			AnyStoreConfig: s.getAnyStoreConfig(),
			SourceService:  s.sourceService,
			OldStore:       s.oldStore,
			Fts:            s.fts,
			SubManager:     s.subManager,
			DbPath:         filepath.Join(dir, "objects.db"),
			FulltextQueue:  s,
		})
		s.spaceIndexes[spaceId] = store
	}
	return store
}

func (s *dsObjectStore) getAnyStoreConfig() *anystore.Config {
	return &anystore.Config{
		Namespace:               s.anyStoreConfig.Namespace,
		ReadConnections:         s.anyStoreConfig.ReadConnections,
		SQLiteConnectionOptions: maps.Clone(s.anyStoreConfig.SQLiteConnectionOptions),
		SyncPoolElementMaxSize:  s.anyStoreConfig.SyncPoolElementMaxSize,
	}
}

func (s *dsObjectStore) GetCrdtDb(spaceId string) anystore.DB {
	s.crtdStoreLock.Lock()
	defer s.crtdStoreLock.Unlock()

	db, ok := s.crdtDbs[spaceId]
	if !ok {
		dir := filepath.Join(s.objectStorePath, spaceId)
		err := ensureDirExists(dir)
		if err != nil {
			return nil
		}
		path := filepath.Join(dir, "crdt.db")
		db, err = anystore.Open(s.componentCtx, path, s.getAnyStoreConfig())
		if errors.Is(err, anystore.ErrIncompatibleVersion) {
			_ = os.RemoveAll(path)
			db, err = anystore.Open(s.componentCtx, path, s.getAnyStoreConfig())
		}
		if err != nil {
			return nil
		}
		s.crdtDbs[spaceId] = db
	}
	return db
}

func (s *dsObjectStore) listStores() []spaceindex.Store {
	err := s.preloadExistingObjectStores()
	if err != nil {
		log.Errorf("preloadExistingObjectStores: %v", err)
	}
	s.Lock()
	stores := make([]spaceindex.Store, 0, len(s.spaceIndexes))
	for _, store := range s.spaceIndexes {
		stores = append(stores, store)
	}
	s.Unlock()
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

func (s *dsObjectStore) ListIdsCrossSpace() ([]string, error) {
	return collectCrossSpace(s, func(store spaceindex.Store) ([]string, error) {
		return store.ListIds()
	})
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
