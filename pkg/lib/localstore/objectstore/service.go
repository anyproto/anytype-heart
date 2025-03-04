package objectstore

import (
	"context"
	"errors"
	"fmt"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceresolverstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	BatchProcessFullTextQueue(ctx context.Context, limit int, processIds func(processIds []string) error) error

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

	AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error
	GetFileKeys(fileId domain.FileId) (map[string]string, error)
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
	defer s.lock.Unlock()

	return s.getOrInitSpaceIndex(spaceId)
}

func (s *dsObjectStore) getOrInitSpaceIndex(spaceId string) spaceindex.Store {
	store, ok := s.spaceIndexes[spaceId]
	if !ok {
		db, err := s.anystoreProvider.GetSpaceIndexDb(spaceId)
		if err != nil {
			s.spaceIndexes[spaceId] = spaceindex.NewInvalidStore(err)
		} else {
			store = spaceindex.New(s.componentCtx, spaceId, spaceindex.Deps{
				Db:            db,
				SourceService: s.sourceService,
				Fts:           s.fts,
				SubManager:    s.subManager,
				FulltextQueue: s,
			})
			s.spaceIndexes[spaceId] = store
		}

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

		s.lock.Lock()
		defer s.lock.Unlock()
		for _, spaceId := range spaceIds {
			_ = s.getOrInitSpaceIndex(spaceId)
		}
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
