package spaceindex

import (
	"context"
	"errors"
	"fmt"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/indexer/indexerparams"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("objectstore.spaceindex")

var (
	ErrObjectNotFound = fmt.Errorf("object not found in space index")
	ErrNotAnObject    = fmt.Errorf("not an object")
)

type Store interface {
	SpaceId() string
	Close() error
	Init() error

	// Query adds implicit filters on isArchived, isDeleted and objectType relations! To avoid them use QueryRaw
	Query(q database.Query) (records []database.Record, err error)
	QueryRaw(f *database.Filters, limit int, offset int) (records []database.Record, err error)
	QueryByIds(ids []string) (records []database.Record, err error)
	QueryByIdsAndSubscribeForChanges(ids []string, subscription database.Subscription) (records []database.Record, close func(), err error)
	QueryObjectIds(q database.Query) (ids []string, total int, err error)
	QueryIterate(q database.Query, proc func(details *domain.Details)) error
	IterateAll(proc func(doc *anyenc.Value) error) error
	HasIds(ids []string) (exists []string, err error)
	GetInfosByIds(ids []string) ([]*database.ObjectInfo, error)
	List(includeArchived bool) ([]*database.ObjectInfo, error)

	ListIds() ([]string, error)
	ListFullIds() ([]domain.FullID, error)

	// UpdateObjectDetails updates existing object or create if not missing. Should be used in order to amend existing indexes based on prev/new value
	// set discardLocalDetailsChanges to true in case the caller doesn't have local details in the State
	UpdateObjectDetails(ctx context.Context, id string, details *domain.Details, batch *indexerparams.IndexBatch) error
	SubscribeForAll(callback func(rec database.Record, batch *indexerparams.IndexBatch))
	UpdateObjectLinks(ctx context.Context, id string, links []string) error
	UpdatePendingLocalDetails(id string, proc func(details *domain.Details) (*domain.Details, error)) error
	ModifyObjectDetails(id string, proc func(details *domain.Details) (*domain.Details, bool, error)) error

	DeleteObject(id string) error
	DeleteDetails(ctx context.Context, ids []string) error
	DeleteLinks(ids []string) error

	GetDetails(id string) (*domain.Details, error)
	GetObjectByUniqueKey(uniqueKey domain.UniqueKey) (*domain.Details, error)
	GetUniqueKeyById(id string) (key domain.UniqueKey, err error)

	GetInboundLinksById(id string) ([]string, error)
	GetOutboundLinksById(id string) ([]string, error)
	GetWithLinksInfoById(id string) (*model.ObjectInfoWithLinks, error)

	SetActiveView(objectId, blockId, viewId string) error
	SetActiveViews(objectId string, views map[string]string) error
	GetActiveViews(objectId string) (map[string]string, error)

	GetRelationLink(key string) (*model.RelationLink, error)
	FetchRelationByKey(key string) (relation *relationutils.Relation, err error)
	FetchRelationByKeys(keys ...domain.RelationKey) (relations relationutils.Relations, err error)
	FetchRelationByLinks(links pbtypes.RelationLinks) (relations relationutils.Relations, err error)
	ListAllRelations() (relations relationutils.Relations, err error)
	GetRelationById(id string) (relation *model.Relation, err error)
	GetRelationByKey(key string) (*model.Relation, error)
	GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error)
	ListRelationOptions(relationKey domain.RelationKey) (options []*model.RelationOption, err error)

	GetObjectType(id string) (*model.ObjectType, error)

	GetLastIndexedHeadsHash(ctx context.Context, id string) (headsHash string, err error)
	SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) (err error)

	WriteTx(ctx context.Context) (anystore.WriteTx, error)
}

type SourceDetailsFromID interface {
	DetailsFromIdBasedSource(id domain.FullID) (*domain.Details, error)
}

type FulltextQueue interface {
	FtQueueMarkAsIndexed(ids []domain.FullID, state uint64) error
	AddToIndexQueue(ctx context.Context, ids ...domain.FullID) error
	ListIdsFromFullTextQueue(spaceIds []string, limit uint) ([]domain.FullID, error)
	ClearFullTextQueue(spaceIds []string) error
}

type dsObjectStore struct {
	spaceId    string
	db         anystore.DB
	objects    anystore.Collection
	links      anystore.Collection
	headsState anystore.Collection

	activeViews    anystore.Collection
	pendingDetails anystore.Collection
	collections    []anystore.Collection

	// Deps
	fts           ftsearch.FTSearch
	sourceService SourceDetailsFromID
	subManager    *SubscriptionManager
	fulltextQueue FulltextQueue
	dbProvider    anystoreprovider.Provider

	componentCtx       context.Context
	arenaPool          *anyenc.ArenaPool
	collatorBufferPool *collatorBufferPool

	// State
	lock             sync.RWMutex
	subscriptions    []database.Subscription
	onChangeCallback func(rec database.Record, batch *indexerparams.IndexBatch)
	dbLockRemove     func() error
}

type Deps struct {
	DbProvider    anystoreprovider.Provider
	Fts           ftsearch.FTSearch
	SourceService SourceDetailsFromID
	SubManager    *SubscriptionManager
	FulltextQueue FulltextQueue
}

func New(componentCtx context.Context, spaceId string, deps Deps) Store {
	s := &dsObjectStore{
		spaceId:            spaceId,
		componentCtx:       componentCtx,
		arenaPool:          &anyenc.ArenaPool{},
		collatorBufferPool: newCollatorBufferPool(),
		sourceService:      deps.SourceService,
		fts:                deps.Fts,
		subManager:         deps.SubManager,
		fulltextQueue:      deps.FulltextQueue,
		dbProvider:         deps.DbProvider,
	}

	return s
}

func (s *dsObjectStore) Init() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.db != nil {
		return nil
	}

	db, err := s.dbProvider.GetSpaceIndexDb(s.spaceId)
	if err != nil {
		return fmt.Errorf("get crdt db: %w", err)
	}

	s.db = db

	return s.initCollections(s.componentCtx)
}

type LinksUpdateInfo struct {
	LinksFromId    domain.FullID
	Added, Removed []string
}

var _ Store = (*dsObjectStore)(nil)

func (s *dsObjectStore) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return s.db.WriteTx(ctx)
}

func (s *dsObjectStore) initCollections(ctx context.Context) error {
	objects, err := s.newCollection(ctx, "objects")
	if err != nil {
		return fmt.Errorf("open objects collection: %w", err)
	}
	links, err := s.newCollection(ctx, "links")
	if err != nil {
		return fmt.Errorf("open links collection: %w", err)
	}
	headsState, err := s.newCollection(ctx, "headsState")
	if err != nil {
		return fmt.Errorf("open headsState collection: %w", err)
	}
	activeViews, err := s.newCollection(ctx, "activeViews")
	if err != nil {
		return fmt.Errorf("open activeViews collection: %w", err)
	}
	pendingDetails, err := s.newCollection(ctx, "pendingDetails")
	if err != nil {
		return fmt.Errorf("open pendingDetails collection: %w", err)
	}

	objectIndexes := []anystore.IndexInfo{
		{
			Name:   "uniqueKey",
			Fields: []string{bundle.RelationKeyUniqueKey.String()},
		},
		{
			Name:   "source",
			Fields: []string{bundle.RelationKeySource.String()},
		},
		{
			Name:   "resolvedLayout",
			Fields: []string{bundle.RelationKeyResolvedLayout.String()},
		},
		{
			Name:   "type",
			Fields: []string{bundle.RelationKeyType.String()},
		},
		{
			Name:   "relationKey",
			Fields: []string{bundle.RelationKeyRelationKey.String()},
		},
		{
			Name:   "lastModifiedDate",
			Fields: []string{bundle.RelationKeyLastModifiedDate.String()},
		},
		{
			Name:   "fileId",
			Fields: []string{bundle.RelationKeyFileId.String()},
			Sparse: true,
		},
		{
			Name:   "oldAnytypeID",
			Fields: []string{bundle.RelationKeyOldAnytypeID.String()},
			Sparse: true,
		},
		{
			Name:   "fileVariantChecksums",
			Fields: []string{bundle.RelationKeyFileVariantChecksums.String()},
			Sparse: true,
		},
		{
			Name:   "fileSourceChecksum",
			Fields: []string{bundle.RelationKeyFileSourceChecksum.String()},
			Sparse: true,
		},
	}
	err = anystorehelper.AddIndexes(ctx, objects, objectIndexes)
	if err != nil {
		log.Errorf("ensure object indexes: %s", err)
	}

	linksIndexes := []anystore.IndexInfo{
		{
			Name:   linkOutboundField,
			Fields: []string{linkOutboundField},
		},
	}
	err = anystorehelper.AddIndexes(ctx, links, linksIndexes)
	if err != nil {
		log.Errorf("ensure links indexes: %s", err)
	}

	s.objects = objects
	s.links = links
	s.headsState = headsState
	s.activeViews = activeViews
	s.pendingDetails = pendingDetails

	return nil
}

func (s *dsObjectStore) newCollection(ctx context.Context, name string) (anystore.Collection, error) {
	coll, err := s.db.Collection(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("open collection %s: %w", name, err)
	}
	s.collections = append(s.collections, coll)
	return coll, nil
}

func (s *dsObjectStore) Close() error {
	var err error
	for _, col := range s.collections {
		err = errors.Join(err, col.Close())
	}
	return err
}

func (s *dsObjectStore) SpaceId() string {
	return s.spaceId
}
