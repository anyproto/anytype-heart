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
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-localstore")

const CName = "objectstore"

var (
	ErrObjectNotFound = errors.New("object not found")
	ErrNotAnObject    = fmt.Errorf("not an object")

	_ ObjectStore = (*dsObjectStore)(nil)
)

// nolint: interfacebloat
type ObjectStore interface {
	app.ComponentRunnable
	IndexerStore
	AccountStore
	VirtualSpacesStore
	SpaceNameGetter

	SubscribeForAll(callback func(rec database.Record))

	// Query adds implicit filters on isArchived, isDeleted and objectType relations! To avoid them use QueryRaw
	Query(q database.Query) (records []database.Record, err error)

	QueryRaw(f *database.Filters, limit int, offset int) (records []database.Record, err error)
	QueryByID(ids []string) (records []database.Record, err error)
	QueryByIDAndSubscribeForChanges(ids []string, subscription database.Subscription) (records []database.Record, close func(), err error)
	QueryObjectIDs(q database.Query) (ids []string, total int, err error)
	QueryIterate(q database.Query, proc func(details *domain.Details)) error

	HasIDs(ids ...string) (exists []string, err error)
	GetByIDs(spaceID string, ids []string) ([]*database.ObjectInfo, error)
	List(spaceID string, includeArchived bool) ([]*database.ObjectInfo, error)
	ListIds() ([]string, error)
	ListIdsBySpace(spaceId string) ([]string, error)

	// UpdateObjectDetails updates existing object or create if not missing. Should be used in order to amend existing indexes based on prev/new value
	// set discardLocalDetailsChanges to true in case the caller doesn't have local details in the State
	UpdateObjectDetails(ctx context.Context, id string, details *domain.Details) error
	UpdateObjectLinks(ctx context.Context, id string, links []string) error
	UpdatePendingLocalDetails(id string, proc func(details *domain.Details) (*domain.Details, error)) error
	ModifyObjectDetails(id string, proc func(details *domain.Details) (*domain.Details, bool, error)) error

	DeleteObject(id domain.FullID) error
	DeleteDetails(ctx context.Context, id ...string) error
	DeleteLinks(id ...string) error

	GetDetails(id string) (*domain.Details, error)
	GetObjectByUniqueKey(spaceId string, uniqueKey domain.UniqueKey) (*domain.Details, error)
	GetUniqueKeyById(id string) (key domain.UniqueKey, err error)

	GetInboundLinksByID(id string) ([]string, error)
	GetOutboundLinksByID(id string) ([]string, error)
	GetWithLinksInfoByID(spaceID string, id string) (*model.ObjectInfoWithLinks, error)

	SetActiveView(objectId, blockId, viewId string) error
	SetActiveViews(objectId string, views map[string]string) error
	GetActiveViews(objectId string) (map[string]string, error)

	GetRelationLink(spaceID string, key string) (*model.RelationLink, error)
	FetchRelationByKey(spaceID string, key string) (relation *relationutils.Relation, err error)
	FetchRelationByKeys(spaceId string, keys ...domain.RelationKey) (relations relationutils.Relations, err error)
	FetchRelationByLinks(spaceId string, links pbtypes.RelationLinks) (relations relationutils.Relations, err error)
	ListAllRelations(spaceId string) (relations relationutils.Relations, err error)
	GetRelationByID(id string) (relation *model.Relation, err error)
	GetRelationByKey(spaceId string, key string) (*model.Relation, error)
	GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error)

	GetObjectType(url string) (*model.ObjectType, error)
	BatchProcessFullTextQueue(ctx context.Context, limit int, processIds func(processIds []string) error) error

	WriteTx(ctx context.Context) (anystore.WriteTx, error)
}

type IndexerStore interface {
	AddToIndexQueue(ctx context.Context, id string) error
	ListIDsFromFullTextQueue(limit int) ([]string, error)
	RemoveIDsFromFullTextQueue(ids []string) error
	FTSearch() ftsearch.FTSearch
	GetGlobalChecksums() (checksums *model.ObjectStoreChecksums, err error)

	// GetChecksums Used to get information about localstore state and decide do we need to reindex some objects
	GetChecksums(spaceID string) (checksums *model.ObjectStoreChecksums, err error)
	// SaveChecksums Used to save checksums and force reindex counter
	SaveChecksums(spaceID string, checksums *model.ObjectStoreChecksums) (err error)

	GetLastIndexedHeadsHash(ctx context.Context, id string) (headsHash string, err error)
	SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) (err error)
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

type dsObjectStore struct {
	oldStore oldstore.Service

	repoPath         string
	anyStoreConfig   *anystore.Config
	sourceService    SourceDetailsFromID
	anyStore         anystore.DB
	objects          anystore.Collection
	fulltextQueue    anystore.Collection
	links            anystore.Collection
	headsState       anystore.Collection
	system           anystore.Collection
	activeViews      anystore.Collection
	indexerChecksums anystore.Collection
	virtualSpaces    anystore.Collection
	pendingDetails   anystore.Collection

	arenaPool          *fastjson.ArenaPool
	collatorBufferPool *collatorBufferPool

	fts ftsearch.FTSearch

	sync.RWMutex
	onChangeCallback      func(record database.Record)
	subscriptions         []database.Subscription
	onLinksUpdateCallback func(info LinksUpdateInfo)

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func New() ObjectStore {
	ctx, cancel := context.WithCancel(context.Background())
	return &dsObjectStore{
		componentCtx:       ctx,
		componentCtxCancel: cancel,
	}
}

type SourceDetailsFromID interface {
	DetailsFromIdBasedSource(id string) (*domain.Details, error)
}

func (s *dsObjectStore) Init(a *app.App) (err error) {
	src := a.Component("source")
	if src != nil {
		s.sourceService = a.MustComponent("source").(SourceDetailsFromID)
	}
	fts := a.Component(ftsearch.CName)
	if fts == nil {
		log.Warnf("init objectstore without fulltext")
	} else {
		s.fts = fts.(ftsearch.FTSearch)
	}
	s.arenaPool = &fastjson.ArenaPool{}
	s.collatorBufferPool = newCollatorBufferPool()
	s.repoPath = app.MustComponent[wallet.Wallet](a).RepoPath()
	s.anyStoreConfig = app.MustComponent[configProvider](a).GetAnyStoreConfig()
	s.oldStore = app.MustComponent[oldstore.Service](a)

	return nil
}

func (s *dsObjectStore) Name() (name string) {
	return CName
}

func (s *dsObjectStore) Run(ctx context.Context) error {
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
	objects, err := store.Collection(ctx, "objects")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open objects collection: %w", err))
	}
	fulltextQueue, err := store.Collection(ctx, "fulltext_queue")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open fulltextQueue collection: %w", err))
	}
	links, err := store.Collection(ctx, "links")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open links collection: %w", err))
	}
	headsState, err := store.Collection(ctx, "headsState")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open headsState collection: %w", err))
	}
	system, err := store.Collection(ctx, "system")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open system collection: %w", err))
	}
	activeViews, err := store.Collection(ctx, "activeViews")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open activeViews collection: %w", err))
	}
	indexerChecksums, err := store.Collection(ctx, "indexerChecksums")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open indexerChecksums collection: %w", err))
	}
	virtualSpaces, err := store.Collection(ctx, "virtualSpaces")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open virtualSpaces collection: %w", err))
	}
	pendingDetails, err := store.Collection(ctx, "pendingDetails")
	if err != nil {
		return errors.Join(store.Close(), fmt.Errorf("open pendingDetails collection: %w", err))
	}
	s.anyStore = store

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
	s.fulltextQueue = fulltextQueue
	s.links = links
	s.headsState = headsState
	s.system = system
	s.activeViews = activeViews
	s.indexerChecksums = indexerChecksums
	s.virtualSpaces = virtualSpaces
	s.pendingDetails = pendingDetails

	return nil
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
	return coll.EnsureIndex(ctx, toCreate...)
}

func (s *dsObjectStore) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return s.anyStore.WriteTx(ctx)
}

func (s *dsObjectStore) Close(_ context.Context) (err error) {
	s.componentCtxCancel()
	if s.objects != nil {
		err = errors.Join(err, s.objects.Close())
	}
	// TODO Close collections
	if s.anyStore != nil {
		err = errors.Join(err, s.anyStore.Close())
	}
	return err
}

// unsafe, use under mutex
func (s *dsObjectStore) addSubscriptionIfNotExists(sub database.Subscription) (existed bool) {
	for _, s := range s.subscriptions {
		if s == sub {
			return true
		}
	}

	s.subscriptions = append(s.subscriptions, sub)
	return false
}

func (s *dsObjectStore) closeAndRemoveSubscription(subscription database.Subscription) {
	s.Lock()
	defer s.Unlock()
	subscription.Close()

	for i, sub := range s.subscriptions {
		if sub == subscription {
			s.subscriptions = append(s.subscriptions[:i], s.subscriptions[i+1:]...)
			break
		}
	}
}

func (s *dsObjectStore) SubscribeForAll(callback func(rec database.Record)) {
	s.Lock()
	s.onChangeCallback = callback
	s.Unlock()
}

// GetDetails returns empty struct without errors in case details are not found
// todo: get rid of this or change the name method!
func (s *dsObjectStore) GetDetails(id string) (*domain.Details, error) {
	doc, err := s.objects.FindId(s.componentCtx, id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return domain.NewDetails(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	details, err := domain.JsonToProto(doc.Value())
	if err != nil {
		return nil, fmt.Errorf("unmarshal details: %w", err)
	}
	return details, nil
}

func (s *dsObjectStore) GetUniqueKeyById(id string) (domain.UniqueKey, error) {
	details, err := s.GetDetails(id)
	if err != nil {
		return nil, err
	}
	rawUniqueKey, ok := details.TryString(bundle.RelationKeyUniqueKey)
	if !ok {
		return nil, fmt.Errorf("object does not have unique key in details")
	}
	return domain.UnmarshalUniqueKey(rawUniqueKey)
}

func (s *dsObjectStore) List(spaceID string, includeArchived bool) ([]*database.ObjectInfo, error) {
	var filters []database.FilterRequest
	if spaceID != "" {
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeySpaceId,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(spaceID),
		})
	}
	if includeArchived {
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyIsArchived,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Bool(true),
		})
	}
	ids, _, err := s.QueryObjectIDs(database.Query{
		Filters: filters,
	})
	if err != nil {
		return nil, fmt.Errorf("query object ids: %w", err)
	}
	return s.GetByIDs(spaceID, ids)
}

func (s *dsObjectStore) HasIDs(ids ...string) (exists []string, err error) {
	for _, id := range ids {
		_, err := s.objects.FindId(s.componentCtx, id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return nil, fmt.Errorf("get %s: %w", id, err)
		}
		if err == nil {
			exists = append(exists, id)
		}
	}
	return exists, err
}

func (s *dsObjectStore) GetByIDs(spaceID string, ids []string) ([]*database.ObjectInfo, error) {
	return s.getObjectsInfo(s.componentCtx, spaceID, ids)
}

func (s *dsObjectStore) ListIdsBySpace(spaceId string) ([]string, error) {
	ids, _, err := s.QueryObjectIDs(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeySpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(spaceId),
			},
		},
	})
	return ids, err
}

func (s *dsObjectStore) ListIds() ([]string, error) {
	var ids []string
	iter, err := s.objects.Find(nil).Iter(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("find all: %w", err)
	}
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(fmt.Errorf("get doc: %w", err), iter.Close())
		}
		id := doc.Value().GetStringBytes("id")
		ids = append(ids, string(id))
	}
	err = iter.Err()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("iterate: %w", err), iter.Close())
	}
	return ids, iter.Close()
}

// TODO objstore: Just use dependency injection
func (s *dsObjectStore) FTSearch() ftsearch.FTSearch {
	return s.fts
}

func (s *dsObjectStore) getObjectInfo(ctx context.Context, spaceID string, id string) (*database.ObjectInfo, error) {
	details, err := s.sourceService.DetailsFromIdBasedSource(id)
	if err == nil {
		details.SetString(bundle.RelationKeyId, id)
		return &database.ObjectInfo{
			Id:      id,
			Details: details,
		}, nil
	}

	doc, err := s.objects.FindId(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	details, err = domain.JsonToProto(doc.Value())
	if err != nil {
		return nil, fmt.Errorf("unmarshal details: %w", err)
	}
	snippet := details.GetString(bundle.RelationKeySnippet)

	return &database.ObjectInfo{
		Id:      id,
		Details: details,
		Snippet: snippet,
	}, nil
}

func (s *dsObjectStore) getObjectsInfo(ctx context.Context, spaceID string, ids []string) ([]*database.ObjectInfo, error) {
	objects := make([]*database.ObjectInfo, 0, len(ids))
	for _, id := range ids {
		info, err := s.getObjectInfo(ctx, spaceID, id)
		if err != nil {
			if errors.Is(err, anystore.ErrDocNotFound) || errors.Is(err, ErrObjectNotFound) || errors.Is(err, ErrNotAnObject) {
				continue
			}
			return nil, err
		}
		if f := info.Details; f != nil {
			// skip deleted objects
			if v, ok := f.TryBool(bundle.RelationKeyIsDeleted); ok && v {
				continue
			}
		}
		objects = append(objects, info)
	}

	return objects, nil
}

func (s *dsObjectStore) GetObjectByUniqueKey(spaceId string, uniqueKey domain.UniqueKey) (*domain.Details, error) {
	records, err := s.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey,
				Value:       domain.String(uniqueKey.Marshal()),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId,
				Value:       domain.String(spaceId),
			},
		},
		Limit: 2,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, ErrObjectNotFound
	}

	if len(records) > 1 {
		// should never happen
		return nil, fmt.Errorf("multiple objects with unique key %s", uniqueKey)
	}

	return records[0].Details, nil
}
