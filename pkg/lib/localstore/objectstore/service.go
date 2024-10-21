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

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/helper"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
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
	QueryRawCrossSpace(f *database.Filters, limit int, offset int) (records []database.Record, err error)
	QueryByIdCrossSpace(ids []string) (records []database.Record, err error)

	ListIdsCrossSpace() ([]string, error)
	BatchProcessFullTextQueue(ctx context.Context, limit int, processIds func(processIds []string) error) error

	AccountStore
	VirtualSpacesStore
	IndexerStore
}

type ObjectStore interface {
	app.ComponentRunnable

	SpaceIndex(spaceId string) spaceindex.Store
	GetCrdtDb(spaceId string) anystore.DB

	SpaceNameGetter
	CrossSpace
}

type IndexerStore interface {
	AddToIndexQueue(ctx context.Context, id ...string) error
	ListIdsFromFullTextQueue(limit int) ([]string, error)
	RemoveIdsFromFullTextQueue(ids []string) error
	GetGlobalChecksums() (checksums *model.ObjectStoreChecksums, err error)

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
	repoPath       string
	techSpaceId    string
	anyStoreConfig anystore.Config

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
	spaceIndexes map[string]spaceindex.Store

	crtdStoreLock sync.Mutex
	crdtDbs       map[string]anystore.DB

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
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
	fts := a.Component(ftsearch.CName)
	if fts == nil {
		log.Warnf("init objectstore without fulltext")
	} else {
		s.fts = fts.(ftsearch.FTSearch)
	}
	s.arenaPool = &anyenc.ArenaPool{}
	s.repoPath = app.MustComponent[wallet.Wallet](a).RepoPath()
	s.anyStoreConfig = *app.MustComponent[configProvider](a).GetAnyStoreConfig()
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

	dbDir := s.storeRootDir()
	err := ensureDirExists(dbDir)
	if err != nil {
		return err
	}
	return s.runDatabase(ctx, filepath.Join(dbDir, "objects.db"))
}

f unc (s *dsObjectStore) setDefaultConfig() {
	if s.anyStoreConfig.SQLiteConnectionOptions == nil {
		s.anyStoreConfig.SQLiteConnectionOptions = map[string]string{}
	}

	s.anyStoreConfig.SQLiteConnectionOptions["synchronous"] = "off"
}

func (s *dsObjectStore) storeRootDir() string {
	return filepath.Join(s.repoPath, "objectstore")
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

func (s *dsObjectStore) runDatabase(ctx context.Context, path string) error {
	store, lockRemove, err := helper.OpenDatabaseWithLockCheck(ctx, path, &s.anyStoreConfig)
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

	entries, err := os.ReadDir(s.storeRootDir())
	if err != nil {
		return fmt.Errorf("read spaceindexes dir: %w", err)
	}
	s.Lock()
	defer s.Unlock()
	for _, entry := range entries {
		if entry.IsDir() {
			spaceId := entry.Name()
			s.getOrInitSpaceIndex(spaceId)
		}
	}

	return nil
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
		dir := filepath.Join(s.storeRootDir(), spaceId)
		err := ensureDirExists(dir)
		if err != nil {
			return spaceindex.NewInvalidStore(err)
		}
		store = spaceindex.New(s.componentCtx, spaceId, spaceindex.Deps{
			AnyStoreConfig: &s.anyStoreConfig,
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

func (s *dsObjectStore) GetCrdtDb(spaceId string) anystore.DB {
	s.crtdStoreLock.Lock()
	defer s.crtdStoreLock.Unlock()

	db, ok := s.crdtDbs[spaceId]
	if !ok {
		dir := filepath.Join(s.storeRootDir(), spaceId)
		err := ensureDirExists(dir)
		if err != nil {
			return nil
		}
		path := filepath.Join(dir, "crdt.db")
		db, err = anystore.Open(s.componentCtx, path, &s.anyStoreConfig)
		if errors.Is(err, anystore.ErrIncompatibleVersion) {
			_ = os.RemoveAll(path)
			db, err = anystore.Open(s.componentCtx, path, &s.anyStoreConfig)
		}
		if err != nil {
			return nil
		}
		s.crdtDbs[spaceId] = db
	}
	return db
}

func (s *dsObjectStore) listStores() []spaceindex.Store {
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

func (s *dsObjectStore) QueryRawCrossSpace(filters *database.Filters, limit int, offset int) ([]database.Record, error) {
	return collectCrossSpace(s, func(store spaceindex.Store) ([]database.Record, error) {
		return store.QueryRaw(filters, limit, offset)
	})
}

func (s *dsObjectStore) SubscribeLinksUpdate(callback func(info spaceindex.LinksUpdateInfo)) {
	s.subManager.SubscribeLinksUpdate(callback)
}
