package spaceindex

import (
	"context"
	"errors"
	"fmt"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/helper"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("objectstore.spaceindex")

var (
	ErrObjectNotFound = fmt.Errorf("object not found")
	ErrNotAnObject    = fmt.Errorf("not an object")
)

type Store interface {
	SpaceId() string
	Close() error

	// Query adds implicit filters on isArchived, isDeleted and objectType relations! To avoid them use QueryRaw
	Query(q database.Query) (records []database.Record, err error)
	QueryRaw(f *database.Filters, limit int, offset int) (records []database.Record, err error)
	QueryByIds(ids []string) (records []database.Record, err error)
	QueryByIdsAndSubscribeForChanges(ids []string, subscription database.Subscription) (records []database.Record, close func(), err error)
	QueryObjectIds(q database.Query) (ids []string, total int, err error)
	QueryIterate(q database.Query, proc func(details *types.Struct)) error

	HasIds(ids []string) (exists []string, err error)
	GetInfosByIds(ids []string) ([]*model.ObjectInfo, error)
	List(includeArchived bool) ([]*model.ObjectInfo, error)

	ListIds() ([]string, error)

	// UpdateObjectDetails updates existing object or create if not missing. Should be used in order to amend existing indexes based on prev/new value
	// set discardLocalDetailsChanges to true in case the caller doesn't have local details in the State
	UpdateObjectDetails(ctx context.Context, id string, details *types.Struct) error
	SubscribeForAll(callback func(rec database.Record))

	UpdateObjectLinks(ctx context.Context, id string, links []string) error
	UpdatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error
	ModifyObjectDetails(id string, proc func(details *types.Struct) (*types.Struct, bool, error)) error

	DeleteObject(id string) error
	DeleteDetails(ctx context.Context, ids []string) error
	DeleteLinks(ids []string) error

	GetDetails(id string) (*model.ObjectDetails, error)
	GetObjectByUniqueKey(uniqueKey domain.UniqueKey) (*model.ObjectDetails, error)
	GetUniqueKeyById(id string) (key domain.UniqueKey, err error)

	GetInboundLinksById(id string) ([]string, error)
	GetOutboundLinksById(id string) ([]string, error)
	GetWithLinksInfoById(id string) (*model.ObjectInfoWithLinks, error)

	SetActiveView(objectId, blockId, viewId string) error
	SetActiveViews(objectId string, views map[string]string) error
	GetActiveViews(objectId string) (map[string]string, error)

	GetRelationLink(key string) (*model.RelationLink, error)
	FetchRelationByKey(key string) (relation *relationutils.Relation, err error)
	FetchRelationByKeys(keys ...string) (relations relationutils.Relations, err error)
	FetchRelationByLinks(links pbtypes.RelationLinks) (relations relationutils.Relations, err error)
	ListAllRelations() (relations relationutils.Relations, err error)
	GetRelationById(id string) (relation *model.Relation, err error)
	GetRelationByKey(key string) (*model.Relation, error)
	GetRelationFormatByKey(key string) (model.RelationFormat, error)
	ListRelationOptions(relationKey string) (options []*model.RelationOption, err error)

	GetObjectType(id string) (*model.ObjectType, error)

	GetLastIndexedHeadsHash(ctx context.Context, id string) (headsHash string, err error)
	SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) (err error)

	WriteTx(ctx context.Context) (anystore.WriteTx, error)
}

type SourceDetailsFromID interface {
	DetailsFromIdBasedSource(id string) (*types.Struct, error)
}

type FulltextQueue interface {
	RemoveIdsFromFullTextQueue(ids []string) error
	AddToIndexQueue(ctx context.Context, ids ...string) error
	ListIdsFromFullTextQueue(limit int) ([]string, error)
}

type dsObjectStore struct {
	initErr error

	spaceId        string
	db             anystore.DB
	objects        anystore.Collection
	links          anystore.Collection
	headsState     anystore.Collection
	activeViews    anystore.Collection
	pendingDetails anystore.Collection
	collections    []anystore.Collection

	// Deps
	anyStoreConfig *anystore.Config
	fts            ftsearch.FTSearch
	sourceService  SourceDetailsFromID
	oldStore       oldstore.Service
	subManager     *SubscriptionManager
	fulltextQueue  FulltextQueue

	componentCtx       context.Context
	arenaPool          *anyenc.ArenaPool
	collatorBufferPool *collatorBufferPool

	// State
	lock             sync.RWMutex
	subscriptions    []database.Subscription
	onChangeCallback func(rec database.Record)
	dbLockRemove     func() error
}

type Deps struct {
	AnyStoreConfig *anystore.Config
	Fts            ftsearch.FTSearch
	SourceService  SourceDetailsFromID
	OldStore       oldstore.Service
	SubManager     *SubscriptionManager
	DbPath         string
	FulltextQueue  FulltextQueue
}

func New(componentCtx context.Context, spaceId string, deps Deps) Store {
	s := &dsObjectStore{
		spaceId:            spaceId,
		componentCtx:       componentCtx,
		arenaPool:          &anyenc.ArenaPool{},
		collatorBufferPool: newCollatorBufferPool(),
		anyStoreConfig:     deps.AnyStoreConfig,
		sourceService:      deps.SourceService,
		oldStore:           deps.OldStore,
		fts:                deps.Fts,
		subManager:         deps.SubManager,
		fulltextQueue:      deps.FulltextQueue,
	}

	var err error
	s.db, s.dbLockRemove, err = helper.OpenDatabaseWithLockCheck(componentCtx, deps.DbPath, deps.AnyStoreConfig)
	if err != nil {
		s.initErr = err
		return s
	}
	err = s.initDatabase(componentCtx)
	if err != nil {
		_ = s.dbLockRemove()
		s.db = nil
		s.initErr = err
	}
	return s
}

type LinksUpdateInfo struct {
	LinksFromId    string
	Added, Removed []string
}

var _ Store = (*dsObjectStore)(nil)

func (s *dsObjectStore) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return s.db.WriteTx(ctx)
}

func (s *dsObjectStore) initDatabase(ctx context.Context) error {
	objects, err := s.newCollection(ctx, "objects")
	if err != nil {
		return errors.Join(s.db.Close(), fmt.Errorf("open objects collection: %w", err))
	}
	links, err := s.newCollection(ctx, "links")
	if err != nil {
		return errors.Join(s.db.Close(), fmt.Errorf("open links collection: %w", err))
	}
	headsState, err := s.newCollection(ctx, "headsState")
	if err != nil {
		return errors.Join(s.db.Close(), fmt.Errorf("open headsState collection: %w", err))
	}
	activeViews, err := s.newCollection(ctx, "activeViews")
	if err != nil {
		return errors.Join(s.db.Close(), fmt.Errorf("open activeViews collection: %w", err))
	}
	pendingDetails, err := s.newCollection(ctx, "pendingDetails")
	if err != nil {
		return errors.Join(s.db.Close(), fmt.Errorf("open pendingDetails collection: %w", err))
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
			Name:   "layout",
			Fields: []string{bundle.RelationKeyLayout.String()},
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
	}
	err = s.addIndexes(ctx, objects, objectIndexes)
	if err != nil {
		log.Errorf("ensure object indexes: %s", err)
	}

	linksIndexes := []anystore.IndexInfo{
		{
			Name:   linkOutboundField,
			Fields: []string{linkOutboundField},
		},
	}
	err = s.addIndexes(ctx, links, linksIndexes)
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

	s.lock.Lock()
	err = errors.Join(err, s.db.Checkpoint(context.Background(), true))
	s.lock.Unlock()

	err = errors.Join(err, s.db.Close())
	// remove lock file only after successful close
	err = errors.Join(err, s.dbLockRemove())
	return err
}

func (s *dsObjectStore) addIndexes(ctx context.Context, coll anystore.Collection, indexes []anystore.IndexInfo) error {
	gotIndexes := coll.GetIndexes()
	toCreate := indexes[:0]
	var toDrop []string
	for _, idx := range indexes {
		if !slices.ContainsFunc(gotIndexes, func(i anystore.Index) bool {
			return i.Info().Name == idx.Name
		}) {
			toCreate = append(toCreate, idx)
		}
	}
	for _, idx := range gotIndexes {
		if !slices.ContainsFunc(indexes, func(i anystore.IndexInfo) bool {
			return i.Name == idx.Info().Name
		}) {
			toDrop = append(toDrop, idx.Info().Name)
		}
	}
	if len(toDrop) > 0 {
		for _, indexName := range toDrop {
			if err := coll.DropIndex(ctx, indexName); err != nil {
				return err
			}
		}
	}
	if len(toCreate) > 0 {
		return coll.EnsureIndex(ctx, toCreate...)
	}
	return nil
}

func (s *dsObjectStore) SpaceId() string {
	return s.spaceId
}
