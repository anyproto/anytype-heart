package spaceindex

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/domain"
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
	ErrObjectNotFound      = fmt.Errorf("object not found in space index")
	ErrNotAnObject         = fmt.Errorf("not an object")
	ErrSpaceNotInitialized = fmt.Errorf("space index not initialized")
	ErrSpaceRemoved        = fmt.Errorf("space index has been permanently removed")
)

// sharedEmptyStore is a singleton used by all uninitialized proxies
var sharedEmptyStore = &emptyStore{}

// OutgoingLink represents a link from this object to another object
type OutgoingLink struct {
	TargetID    string // ID of the target object
	BlockID     string // Block ID where the link originates (empty for relation links)
	RelationKey string // Relation key (empty for block links)
}

// IncomingLink represents a link to this object from another object
type IncomingLink struct {
	SourceID    string // ID of the source object that links to this object
	BlockID     string // Block ID where the link originates (empty for relation links)
	RelationKey string // Relation key (empty for block links)
}

type Store interface {
	SpaceId() string
	Close() error
	Init() error
	Deactivate() error // Close and mark as permanently removed (prevents re-initialization, does NOT delete from disk)

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
	UpdateObjectDetails(ctx context.Context, id string, details *domain.Details) error
	SubscribeForAll(callback func(rec database.Record))
	// UpdaeObjectLinks is deprecated, use UpdateObjectLinksDetailed instead
	UpdateObjectLinks(ctx context.Context, id string, links []string) error
	UpdateObjectLinksDetailed(ctx context.Context, id string, outgoingLinks []OutgoingLink) error
	UpdatePendingLocalDetails(id string, proc func(details *domain.Details) (*domain.Details, error)) error
	ModifyObjectDetails(id string, proc func(details *domain.Details) (*domain.Details, bool, error), upsert bool) error

	DeleteObject(id string) error
	DeleteDetails(ctx context.Context, ids []string) error
	DeleteLinks(ids []string) error

	GetDetails(id string) (*domain.Details, error)
	GetObjectByUniqueKey(uniqueKey domain.UniqueKey) (*domain.Details, error)
	GetUniqueKeyById(id string) (key domain.UniqueKey, err error)

	GetInboundLinksById(id string) ([]string, error)
	GetOutboundLinksById(id string) ([]string, error)
	GetOutboundLinksDetailedById(id string) ([]OutgoingLink, error)
	GetOutboundLinksDetailedIterator(f func(id string, links []OutgoingLink) bool) error
	GetInboundLinksDetailedById(id string) ([]IncomingLink, error)

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
	SaveLastIndexedHeadsHashWithFtQueueCtr(ctx context.Context, id string, headsHash string, ftQueueCtr uint64) (err error)
	GetHeadsWithFtQueueCtrGreaterThan(ctx context.Context, threshold uint64) ([]HeadsStateEntry, error)
	ClearHeadsState(ctx context.Context) error

	WriteTx(ctx context.Context) (anystore.WriteTx, error)
}

type SourceDetailsFromID interface {
	DetailsFromIdBasedSource(id domain.FullID) (*domain.Details, error)
}

type FulltextQueue interface {
	FtQueueMarkAsIndexed(ids []domain.FullID, state uint64) error
	AddToIndexQueue(ctx context.Context, ids ...domain.FullID) (uint64, int, error)
	ListIdsFromFullTextQueue(spaceIds []string, limit uint) ([]domain.FullTextQueuedObject, error)
	ClearFullTextQueue(spaceIds []string) error
}

// storeProxy delegates to either sharedEmptyStore (before init) or dsObjectStore (after init).
// This allows SpaceIndex() to always return the same instance that works correctly
// once initialized.
//
// Concurrency note: If Close() or Remove() is called while a query is in progress,
// the in-flight query may receive a "database is closed" error rather than results.
// This is expected behavior - callers should handle such errors gracefully during shutdown.
// We intentionally don't nil realStore in Close()/Remove() to avoid panics from concurrent access.
type storeProxy struct {
	spaceId     string
	initialized atomic.Bool
	removed     atomic.Bool    // permanently removed, prevents re-initialization
	realStore   *dsObjectStore // only accessed when initialized is true
	deps        Deps
	initMu      sync.Mutex
	ctx         context.Context

	// onChangeCallback is stored here so it survives until dsObjectStore is created
	onChangeCallback func(rec database.Record)
}

var _ Store = (*storeProxy)(nil)

type dsObjectStore struct {
	spaceId string
	db      anystore.DB
	objects anystore.Collection
	links   anystore.Collection

	headsState     anystore.Collection
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
	onChangeCallback func(rec database.Record)
	dbLockRemove     func() error
}

type Deps struct {
	DbProvider    anystoreprovider.Provider
	Fts           ftsearch.FTSearch
	SourceService SourceDetailsFromID
	SubManager    *SubscriptionManager
	FulltextQueue FulltextQueue
}

// New creates a new Store that delegates to sharedEmptyStore until Init() is called.
func New(componentCtx context.Context, spaceId string, deps Deps) Store {
	return &storeProxy{
		spaceId: spaceId,
		deps:    deps,
		ctx:     componentCtx,
	}
}

func (p *storeProxy) Init() error {
	p.initMu.Lock()
	defer p.initMu.Unlock()

	if p.removed.Load() {
		return ErrSpaceRemoved
	}

	if p.initialized.Load() {
		return nil
	}

	store := newDsObjectStore(p.ctx, p.spaceId, p.deps)
	if err := store.init(); err != nil {
		return err
	}

	// Transfer the callback that was registered before Init()
	if p.onChangeCallback != nil {
		store.SubscribeForAll(p.onChangeCallback)
	}

	p.realStore = store         // non-atomic write
	p.initialized.Store(true)   // atomic write creates synchronization point
	return nil
}

func (p *storeProxy) IsInitialized() bool {
	return p.initialized.Load()
}

func (p *storeProxy) SpaceId() string {
	return p.spaceId
}

func (p *storeProxy) Close() error {
	p.initMu.Lock()
	defer p.initMu.Unlock()

	if !p.initialized.Load() {
		return nil
	}

	// Mark uninitialized FIRST - new queries will use sharedEmptyStore
	p.initialized.Store(false)

	// Close real store - in-flight queries will get "database closed" error.
	// We intentionally don't nil realStore to avoid panic from concurrent access.
	if p.realStore != nil {
		return p.realStore.Close()
	}

	return nil
}

// Deactivate closes the store and marks it as permanently removed.
// After deactivation, Init() will return ErrSpaceRemoved.
// This does NOT delete data from disk - use dsObjectStore.DeleteSpaceIndex() for full deletion.
func (p *storeProxy) Deactivate() error {
	p.initMu.Lock()
	defer p.initMu.Unlock()

	// Mark as removed FIRST - prevents any new Init() calls
	p.removed.Store(true)
	p.initialized.Store(false)

	// Close real store if it exists - in-flight queries will get "database closed" error.
	// We intentionally don't nil realStore to avoid panic from concurrent access.
	if p.realStore != nil {
		return p.realStore.Close()
	}

	return nil
}

// All interface methods check initialized and delegate to realStore or sharedEmptyStore

func (p *storeProxy) Query(q database.Query) ([]database.Record, error) {
	if p.initialized.Load() {
		return p.realStore.Query(q)
	}
	return sharedEmptyStore.Query(q)
}

func (p *storeProxy) QueryRaw(f *database.Filters, limit int, offset int) ([]database.Record, error) {
	if p.initialized.Load() {
		return p.realStore.QueryRaw(f, limit, offset)
	}
	return sharedEmptyStore.QueryRaw(f, limit, offset)
}

func (p *storeProxy) QueryByIds(ids []string) ([]database.Record, error) {
	if p.initialized.Load() {
		return p.realStore.QueryByIds(ids)
	}
	return sharedEmptyStore.QueryByIds(ids)
}

func (p *storeProxy) QueryByIdsAndSubscribeForChanges(ids []string, subscription database.Subscription) ([]database.Record, func(), error) {
	if p.initialized.Load() {
		return p.realStore.QueryByIdsAndSubscribeForChanges(ids, subscription)
	}
	return sharedEmptyStore.QueryByIdsAndSubscribeForChanges(ids, subscription)
}

func (p *storeProxy) QueryObjectIds(q database.Query) ([]string, int, error) {
	if p.initialized.Load() {
		return p.realStore.QueryObjectIds(q)
	}
	return sharedEmptyStore.QueryObjectIds(q)
}

func (p *storeProxy) QueryIterate(q database.Query, proc func(details *domain.Details)) error {
	if p.initialized.Load() {
		return p.realStore.QueryIterate(q, proc)
	}
	return sharedEmptyStore.QueryIterate(q, proc)
}

func (p *storeProxy) IterateAll(proc func(doc *anyenc.Value) error) error {
	if p.initialized.Load() {
		return p.realStore.IterateAll(proc)
	}
	return sharedEmptyStore.IterateAll(proc)
}

func (p *storeProxy) HasIds(ids []string) ([]string, error) {
	if p.initialized.Load() {
		return p.realStore.HasIds(ids)
	}
	return sharedEmptyStore.HasIds(ids)
}

func (p *storeProxy) GetInfosByIds(ids []string) ([]*database.ObjectInfo, error) {
	if p.initialized.Load() {
		return p.realStore.GetInfosByIds(ids)
	}
	return sharedEmptyStore.GetInfosByIds(ids)
}

func (p *storeProxy) List(includeArchived bool) ([]*database.ObjectInfo, error) {
	if p.initialized.Load() {
		return p.realStore.List(includeArchived)
	}
	return sharedEmptyStore.List(includeArchived)
}

func (p *storeProxy) ListIds() ([]string, error) {
	if p.initialized.Load() {
		return p.realStore.ListIds()
	}
	return sharedEmptyStore.ListIds()
}

func (p *storeProxy) ListFullIds() ([]domain.FullID, error) {
	if p.initialized.Load() {
		return p.realStore.ListFullIds()
	}
	return sharedEmptyStore.ListFullIds()
}

func (p *storeProxy) UpdateObjectDetails(ctx context.Context, id string, details *domain.Details) error {
	if p.initialized.Load() {
		return p.realStore.UpdateObjectDetails(ctx, id, details)
	}
	return sharedEmptyStore.UpdateObjectDetails(ctx, id, details)
}

func (p *storeProxy) SubscribeForAll(callback func(rec database.Record)) {
	// Store the callback in the proxy so it survives until dsObjectStore is created.
	// We use initMu to protect onChangeCallback write, but release it before calling
	// realStore.SubscribeForAll to avoid deadlock with Init() which also holds initMu
	// while acquiring dsObjectStore.lock.
	p.initMu.Lock()
	p.onChangeCallback = callback
	initialized := p.initialized.Load()
	realStore := p.realStore
	p.initMu.Unlock()

	// Forward to realStore if already initialized.
	// Note: There's a race window here where Init() might complete between the check
	// and the call, causing double registration. However, this is harmless as the
	// callback just gets overwritten with the same value.
	if initialized && realStore != nil {
		realStore.SubscribeForAll(callback)
	}
}

func (p *storeProxy) UpdateObjectLinks(ctx context.Context, id string, links []string) error {
	if p.initialized.Load() {
		return p.realStore.UpdateObjectLinks(ctx, id, links)
	}
	return sharedEmptyStore.UpdateObjectLinks(ctx, id, links)
}

func (p *storeProxy) UpdateObjectLinksDetailed(ctx context.Context, id string, outgoingLinks []OutgoingLink) error {
	if p.initialized.Load() {
		return p.realStore.UpdateObjectLinksDetailed(ctx, id, outgoingLinks)
	}
	return sharedEmptyStore.UpdateObjectLinksDetailed(ctx, id, outgoingLinks)
}

func (p *storeProxy) UpdatePendingLocalDetails(id string, proc func(details *domain.Details) (*domain.Details, error)) error {
	if p.initialized.Load() {
		return p.realStore.UpdatePendingLocalDetails(id, proc)
	}
	return sharedEmptyStore.UpdatePendingLocalDetails(id, proc)
}

func (p *storeProxy) ModifyObjectDetails(id string, proc func(details *domain.Details) (*domain.Details, bool, error), upsert bool) error {
	if p.initialized.Load() {
		return p.realStore.ModifyObjectDetails(id, proc, upsert)
	}
	return sharedEmptyStore.ModifyObjectDetails(id, proc, upsert)
}

func (p *storeProxy) DeleteObject(id string) error {
	if p.initialized.Load() {
		return p.realStore.DeleteObject(id)
	}
	return sharedEmptyStore.DeleteObject(id)
}

func (p *storeProxy) DeleteDetails(ctx context.Context, ids []string) error {
	if p.initialized.Load() {
		return p.realStore.DeleteDetails(ctx, ids)
	}
	return sharedEmptyStore.DeleteDetails(ctx, ids)
}

func (p *storeProxy) DeleteLinks(ids []string) error {
	if p.initialized.Load() {
		return p.realStore.DeleteLinks(ids)
	}
	return sharedEmptyStore.DeleteLinks(ids)
}

func (p *storeProxy) GetDetails(id string) (*domain.Details, error) {
	if p.initialized.Load() {
		return p.realStore.GetDetails(id)
	}
	return sharedEmptyStore.GetDetails(id)
}

func (p *storeProxy) GetObjectByUniqueKey(uniqueKey domain.UniqueKey) (*domain.Details, error) {
	if p.initialized.Load() {
		return p.realStore.GetObjectByUniqueKey(uniqueKey)
	}
	return sharedEmptyStore.GetObjectByUniqueKey(uniqueKey)
}

func (p *storeProxy) GetUniqueKeyById(id string) (domain.UniqueKey, error) {
	if p.initialized.Load() {
		return p.realStore.GetUniqueKeyById(id)
	}
	return sharedEmptyStore.GetUniqueKeyById(id)
}

func (p *storeProxy) GetInboundLinksById(id string) ([]string, error) {
	if p.initialized.Load() {
		return p.realStore.GetInboundLinksById(id)
	}
	return sharedEmptyStore.GetInboundLinksById(id)
}

func (p *storeProxy) GetOutboundLinksDetailedById(id string) ([]OutgoingLink, error) {
	if p.initialized.Load() {
		return p.realStore.GetOutboundLinksDetailedById(id)
	}
	return sharedEmptyStore.GetOutboundLinksDetailedById(id)
}

func (p *storeProxy) GetOutboundLinksDetailedIterator(f func(id string, links []OutgoingLink) bool) error {
	if p.initialized.Load() {
		return p.realStore.GetOutboundLinksDetailedIterator(f)
	}
	return sharedEmptyStore.GetOutboundLinksDetailedIterator(f)
}

func (p *storeProxy) GetInboundLinksDetailedById(id string) ([]IncomingLink, error) {
	if p.initialized.Load() {
		return p.realStore.GetInboundLinksDetailedById(id)
	}
	return sharedEmptyStore.GetInboundLinksDetailedById(id)
}

func (p *storeProxy) GetOutboundLinksById(id string) ([]string, error) {
	if p.initialized.Load() {
		return p.realStore.GetOutboundLinksById(id)
	}
	return sharedEmptyStore.GetOutboundLinksById(id)
}

func (p *storeProxy) GetWithLinksInfoById(id string) (*model.ObjectInfoWithLinks, error) {
	if p.initialized.Load() {
		return p.realStore.GetWithLinksInfoById(id)
	}
	return sharedEmptyStore.GetWithLinksInfoById(id)
}

func (p *storeProxy) SetActiveView(objectId, blockId, viewId string) error {
	if p.initialized.Load() {
		return p.realStore.SetActiveView(objectId, blockId, viewId)
	}
	return sharedEmptyStore.SetActiveView(objectId, blockId, viewId)
}

func (p *storeProxy) SetActiveViews(objectId string, views map[string]string) error {
	if p.initialized.Load() {
		return p.realStore.SetActiveViews(objectId, views)
	}
	return sharedEmptyStore.SetActiveViews(objectId, views)
}

func (p *storeProxy) GetActiveViews(objectId string) (map[string]string, error) {
	if p.initialized.Load() {
		return p.realStore.GetActiveViews(objectId)
	}
	return sharedEmptyStore.GetActiveViews(objectId)
}

func (p *storeProxy) GetRelationLink(key string) (*model.RelationLink, error) {
	if p.initialized.Load() {
		return p.realStore.GetRelationLink(key)
	}
	return sharedEmptyStore.GetRelationLink(key)
}

func (p *storeProxy) FetchRelationByKey(key string) (*relationutils.Relation, error) {
	if p.initialized.Load() {
		return p.realStore.FetchRelationByKey(key)
	}
	return sharedEmptyStore.FetchRelationByKey(key)
}

func (p *storeProxy) FetchRelationByKeys(keys ...domain.RelationKey) (relationutils.Relations, error) {
	if p.initialized.Load() {
		return p.realStore.FetchRelationByKeys(keys...)
	}
	return sharedEmptyStore.FetchRelationByKeys(keys...)
}

func (p *storeProxy) FetchRelationByLinks(links pbtypes.RelationLinks) (relationutils.Relations, error) {
	if p.initialized.Load() {
		return p.realStore.FetchRelationByLinks(links)
	}
	return sharedEmptyStore.FetchRelationByLinks(links)
}

func (p *storeProxy) ListAllRelations() (relationutils.Relations, error) {
	if p.initialized.Load() {
		return p.realStore.ListAllRelations()
	}
	return sharedEmptyStore.ListAllRelations()
}

func (p *storeProxy) GetRelationById(id string) (*model.Relation, error) {
	if p.initialized.Load() {
		return p.realStore.GetRelationById(id)
	}
	return sharedEmptyStore.GetRelationById(id)
}

func (p *storeProxy) GetRelationByKey(key string) (*model.Relation, error) {
	if p.initialized.Load() {
		return p.realStore.GetRelationByKey(key)
	}
	return sharedEmptyStore.GetRelationByKey(key)
}

func (p *storeProxy) GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error) {
	if p.initialized.Load() {
		return p.realStore.GetRelationFormatByKey(key)
	}
	return sharedEmptyStore.GetRelationFormatByKey(key)
}

func (p *storeProxy) ListRelationOptions(relationKey domain.RelationKey) ([]*model.RelationOption, error) {
	if p.initialized.Load() {
		return p.realStore.ListRelationOptions(relationKey)
	}
	return sharedEmptyStore.ListRelationOptions(relationKey)
}

func (p *storeProxy) GetObjectType(id string) (*model.ObjectType, error) {
	if p.initialized.Load() {
		return p.realStore.GetObjectType(id)
	}
	return sharedEmptyStore.GetObjectType(id)
}

func (p *storeProxy) GetLastIndexedHeadsHash(ctx context.Context, id string) (string, error) {
	if p.initialized.Load() {
		return p.realStore.GetLastIndexedHeadsHash(ctx, id)
	}
	return sharedEmptyStore.GetLastIndexedHeadsHash(ctx, id)
}

func (p *storeProxy) SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) error {
	if p.initialized.Load() {
		return p.realStore.SaveLastIndexedHeadsHash(ctx, id, headsHash)
	}
	return sharedEmptyStore.SaveLastIndexedHeadsHash(ctx, id, headsHash)
}

func (p *storeProxy) SaveLastIndexedHeadsHashWithFtQueueCtr(ctx context.Context, id string, headsHash string, ftQueueCtr uint64) error {
	if p.initialized.Load() {
		return p.realStore.SaveLastIndexedHeadsHashWithFtQueueCtr(ctx, id, headsHash, ftQueueCtr)
	}
	return sharedEmptyStore.SaveLastIndexedHeadsHashWithFtQueueCtr(ctx, id, headsHash, ftQueueCtr)
}

func (p *storeProxy) GetHeadsWithFtQueueCtrGreaterThan(ctx context.Context, threshold uint64) ([]HeadsStateEntry, error) {
	if p.initialized.Load() {
		return p.realStore.GetHeadsWithFtQueueCtrGreaterThan(ctx, threshold)
	}
	return sharedEmptyStore.GetHeadsWithFtQueueCtrGreaterThan(ctx, threshold)
}

func (p *storeProxy) ClearHeadsState(ctx context.Context) error {
	if p.initialized.Load() {
		return p.realStore.ClearHeadsState(ctx)
	}
	return sharedEmptyStore.ClearHeadsState(ctx)
}

func (p *storeProxy) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	if p.initialized.Load() {
		return p.realStore.WriteTx(ctx)
	}
	return sharedEmptyStore.WriteTx(ctx)
}

// newDsObjectStore creates the internal dsObjectStore (not exported)
func newDsObjectStore(componentCtx context.Context, spaceId string, deps Deps) *dsObjectStore {
	return &dsObjectStore{
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
}

func (s *dsObjectStore) init() error {
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

	// Add sparse index on ftQueueCtr for efficient crash recovery queries
	headsStateIndexes := []anystore.IndexInfo{
		{
			Name:   "ftQueueCtr_idx",
			Fields: []string{ftQueueCtrField},
			Sparse: true, // Many objects don't have FT indexing
		},
	}
	err = anystorehelper.AddIndexes(ctx, headsState, headsStateIndexes)
	if err != nil {
		log.Errorf("ensure headsState indexes: %s", err)
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
