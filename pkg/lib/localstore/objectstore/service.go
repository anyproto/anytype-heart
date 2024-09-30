package objectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceobjects"
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

	SubscribeForAll(callback func(rec database.Record))
	ListIdsCrossSpace() ([]string, error)
	BatchProcessFullTextQueue(ctx context.Context, limit int, processIds func(processIds []string) error) error

	AccountStore
	VirtualSpacesStore
	IndexerStore
}

// nolint: interfacebloat
type ObjectStore interface {
	app.ComponentRunnable

	SpaceId(spaceId string) spaceobjects.Store

	SpaceNameGetter
	CrossSpace
}

type IndexerStore interface {
	AddToIndexQueue(ctx context.Context, id string) error
	ListIDsFromFullTextQueue(limit int) ([]string, error)
	RemoveIDsFromFullTextQueue(ids []string) error
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
	anyStoreConfig *anystore.Config

	anyStore anystore.DB

	indexerChecksums anystore.Collection
	virtualSpaces    anystore.Collection
	system           anystore.Collection
	fulltextQueue    anystore.Collection

	arenaPool *fastjson.ArenaPool

	fts                 ftsearch.FTSearch
	subManager          *spaceobjects.SubscriptionManager
	sourceService       spaceobjects.SourceDetailsFromID
	oldStore            oldstore.Service
	techSpaceIdProvider TechSpaceIdProvider

	sync.Mutex
	stores map[string]spaceobjects.Store

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func New() ObjectStore {
	ctx, cancel := context.WithCancel(context.Background())
	return &dsObjectStore{
		componentCtx:       ctx,
		componentCtxCancel: cancel,
		subManager:         &spaceobjects.SubscriptionManager{},
		stores:             map[string]spaceobjects.Store{},
	}
}

func (s *dsObjectStore) Init(a *app.App) (err error) {
	s.sourceService = app.MustComponent[spaceobjects.SourceDetailsFromID](a)
	fts := a.Component(ftsearch.CName)
	if fts == nil {
		log.Warnf("init objectstore without fulltext")
	} else {
		s.fts = fts.(ftsearch.FTSearch)
	}
	s.arenaPool = &fastjson.ArenaPool{}
	s.repoPath = app.MustComponent[wallet.Wallet](a).RepoPath()
	s.anyStoreConfig = app.MustComponent[configProvider](a).GetAnyStoreConfig()
	s.oldStore = app.MustComponent[oldstore.Service](a)
	s.techSpaceIdProvider = app.MustComponent[TechSpaceIdProvider](a)

	return nil
}

func (s *dsObjectStore) Name() (name string) {
	return CName
}

func (s *dsObjectStore) Run(ctx context.Context) error {
	s.techSpaceId = s.techSpaceIdProvider.TechSpaceId()

	dbDir := filepath.Join(s.repoPath, "objectstore")
	_, err := os.Stat(dbDir)
	if errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(dbDir, 0700)
		if err != nil {
			return fmt.Errorf("create db dir: %w", err)
		}
	}
	return s.runDatabase(ctx, filepath.Join(dbDir, "objects.db"))
}

func (s *dsObjectStore) runDatabase(ctx context.Context, path string) error {
	store, err := anystore.Open(ctx, path, s.anyStoreConfig)
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

	s.fulltextQueue = fulltextQueue
	s.system = system
	s.indexerChecksums = indexerChecksums
	s.virtualSpaces = virtualSpaces

	return nil
}

func (s *dsObjectStore) Close(_ context.Context) (err error) {
	s.componentCtxCancel()
	// TODO Close collections
	if s.anyStore != nil {
		err = errors.Join(err, s.anyStore.Close())
	}
	return err
}

func (s *dsObjectStore) SpaceId(spaceId string) spaceobjects.Store {
	// TODO Check spaceId
	s.Lock()
	store, ok := s.stores[spaceId]
	if !ok {
		store = spaceobjects.New(s.componentCtx, spaceId, spaceobjects.Deps{
			AnyStoreConfig: s.anyStoreConfig,
			SourceService:  s.sourceService,
			OldStore:       s.oldStore,
			Fts:            s.fts,
			SubManager:     s.subManager,
			DbPath:         filepath.Join(s.repoPath, "objectstore", fmt.Sprintf("%s.db", spaceId)),
			FulltextQueue:  s,
		})
		s.stores[spaceId] = store
	}
	s.Unlock()
	return store
}

func (s *dsObjectStore) SubscribeForAll(callback func(rec database.Record)) {
	s.subManager.SubscribeForAll(callback)
}

func (s *dsObjectStore) listStores() []spaceobjects.Store {
	s.Lock()
	stores := make([]spaceobjects.Store, 0, len(s.stores))
	for _, store := range s.stores {
		stores = append(stores, store)
	}
	s.Unlock()
	return stores
}

func collectCrossSpace[T any](s *dsObjectStore, proc func(store spaceobjects.Store) ([]T, error)) ([]T, error) {
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
	return collectCrossSpace(s, func(store spaceobjects.Store) ([]string, error) {
		return store.ListIds()
	})
}

func (s *dsObjectStore) QueryByIdCrossSpace(ids []string) ([]database.Record, error) {
	return collectCrossSpace(s, func(store spaceobjects.Store) ([]database.Record, error) {
		return store.QueryByID(ids)
	})
}

func (s *dsObjectStore) QueryCrossSpace(q database.Query) ([]database.Record, error) {
	return collectCrossSpace(s, func(store spaceobjects.Store) ([]database.Record, error) {
		return store.Query(q)
	})
}

func (s *dsObjectStore) QueryRawCrossSpace(filters *database.Filters, limit int, offset int) ([]database.Record, error) {
	return collectCrossSpace(s, func(store spaceobjects.Store) ([]database.Record, error) {
		return store.QueryRaw(filters, limit, offset)
	})
}

func (s *dsObjectStore) SubscribeLinksUpdate(callback func(info spaceobjects.LinksUpdateInfo)) {
	s.subManager.SubscribeLinksUpdate(callback)
}
