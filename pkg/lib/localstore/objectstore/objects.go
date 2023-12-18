package objectstore

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/ristretto"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-localstore")
var ErrDetailsNotChanged = errors.New("details not changed")

const CName = "objectstore"

var (
	// ObjectInfo is stored in db key pattern:
	pagesPrefix        = "pages"
	pagesDetailsBase   = ds.NewKey("/" + pagesPrefix + "/details")
	pendingDetailsBase = ds.NewKey("/" + pagesPrefix + "/pending")

	pagesSnippetBase       = ds.NewKey("/" + pagesPrefix + "/snippet")
	pagesInboundLinksBase  = ds.NewKey("/" + pagesPrefix + "/inbound")
	pagesOutboundLinksBase = ds.NewKey("/" + pagesPrefix + "/outbound")
	indexQueueBase         = ds.NewKey("/" + pagesPrefix + "/index")
	bundledChecksums       = ds.NewKey("/" + pagesPrefix + "/checksum")
	indexedHeadsState      = ds.NewKey("/" + pagesPrefix + "/headsstate")

	accountPrefix = "account"
	accountStatus = ds.NewKey("/" + accountPrefix + "/status")

	spacePrefix   = "space"
	virtualSpaces = ds.NewKey("/" + spacePrefix + "/virtual")

	ErrObjectNotFound = errors.New("object not found")

	_ ObjectStore = (*dsObjectStore)(nil)
)

func New() ObjectStore {
	return &dsObjectStore{}
}

type SourceDetailsFromID interface {
	DetailsFromIdBasedSource(id string) (*types.Struct, error)
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
	datastoreService := a.MustComponent(datastore.CName).(datastore.Datastore)
	s.db, err = datastoreService.LocalStorage()
	if err != nil {
		return fmt.Errorf("get badger: %w", err)
	}

	s.backlinksUpdateCh = make(chan BacklinksUpdateInfo)

	return s.initCache()
}

func (s *dsObjectStore) initCache() error {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10_000_000,
		MaxCost:     100_000_000,
		BufferItems: 64,
	})
	if err != nil {
		return fmt.Errorf("init cache: %w", err)
	}
	s.cache = cache
	return nil
}

func (s *dsObjectStore) Name() (name string) {
	return CName
}

// nolint: interfacebloat
type ObjectStore interface {
	app.ComponentRunnable
	IndexerStore
	AccountStore
	VirtualSpacesStore

	SubscribeForAll(callback func(rec database.Record))

	Query(q database.Query) (records []database.Record, total int, err error)
	QueryRaw(f *database.Filters, limit int, offset int) (records []database.Record, err error)
	QueryByID(ids []string) (records []database.Record, err error)
	QueryByIDAndSubscribeForChanges(ids []string, subscription database.Subscription) (records []database.Record, close func(), err error)
	QueryObjectIDs(q database.Query) (ids []string, total int, err error)

	HasIDs(ids ...string) (exists []string, err error)
	GetByIDs(spaceID string, ids []string) ([]*model.ObjectInfo, error)
	List(spaceID string, includeArchived bool) ([]*model.ObjectInfo, error)
	ListIds() ([]string, error)
	ListIdsBySpace(spaceId string) ([]string, error)

	// UpdateObjectDetails updates existing object or create if not missing. Should be used in order to amend existing indexes based on prev/new value
	// set discardLocalDetailsChanges to true in case the caller doesn't have local details in the State
	UpdateObjectDetails(id string, details *types.Struct) error
	UpdateObjectLinks(id string, links []string) error
	UpdateObjectSnippet(id string, snippet string) error
	UpdatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error
	ModifyObjectDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error

	DeleteObject(id string) error
	DeleteDetails(id ...string) error
	// EraseIndexes erase all indexes for objectstore. All objects need to be reindexed
	EraseIndexes(spaceId string) error

	GetDetails(id string) (*model.ObjectDetails, error)
	GetObjectByUniqueKey(spaceId string, uniqueKey domain.UniqueKey) (*model.ObjectDetails, error)
	GetUniqueKeyById(id string) (key domain.UniqueKey, err error)

	SubscribeBacklinksUpdate() (infoCh <-chan BacklinksUpdateInfo, closeFunc func())

	GetInboundLinksByID(id string) ([]string, error)
	GetOutboundLinksByID(id string) ([]string, error)
	GetWithLinksInfoByID(spaceID string, id string) (*model.ObjectInfoWithLinks, error)

	GetRelationLink(spaceID string, key string) (*model.RelationLink, error)
	FetchRelationByKey(spaceID string, key string) (relation *relationutils.Relation, err error)
	FetchRelationByKeys(spaceId string, keys ...string) (relations relationutils.Relations, err error)
	FetchRelationByLinks(spaceId string, links pbtypes.RelationLinks) (relations relationutils.Relations, err error)
	ListAllRelations(spaceId string) (relations relationutils.Relations, err error)
	GetRelationByID(id string) (relation *model.Relation, err error)
	GetRelationByKey(key string) (*model.Relation, error)

	GetObjectType(url string) (*model.ObjectType, error)
	BatchProcessFullTextQueue(limit int, processIds func(processIds []string) error) error
}

type IndexerStore interface {
	AddToIndexQueue(id string) error
	ListIDsFromFullTextQueue() ([]string, error)
	RemoveIDsFromFullTextQueue(ids []string)
	FTSearch() ftsearch.FTSearch
	GetGlobalChecksums() (checksums *model.ObjectStoreChecksums, err error)

	// GetChecksums Used to get information about localstore state and decide do we need to reindex some objects
	GetChecksums(spaceID string) (checksums *model.ObjectStoreChecksums, err error)
	// SaveChecksums Used to save checksums and force reindex counter
	SaveChecksums(spaceID string, checksums *model.ObjectStoreChecksums) (err error)

	GetLastIndexedHeadsHash(id string) (headsHash string, err error)
	SaveLastIndexedHeadsHash(id string, headsHash string) (err error)
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

var ErrNotAnObject = fmt.Errorf("not an object")

type dsObjectStore struct {
	sourceService SourceDetailsFromID

	cache *ristretto.Cache
	db    *badger.DB

	fts ftsearch.FTSearch

	sync.RWMutex
	onChangeCallback  func(record database.Record)
	subscriptions     []database.Subscription
	backlinksUpdateCh chan BacklinksUpdateInfo
}

func (s *dsObjectStore) EraseIndexes(spaceId string) error {
	ids, err := s.ListIdsBySpace(spaceId)
	if err != nil {
		return fmt.Errorf("list ids by space: %w", err)
	}
	err = badgerhelper.RetryOnConflict(func() error {
		txn := s.db.NewTransaction(true)
		defer txn.Discard()
		for _, id := range ids {
			txn, err = s.eraseLinksForObject(txn, id)
			if err != nil {
				return fmt.Errorf("erase links for object %s: %w", id, err)
			}
		}
		return txn.Commit()
	})
	return err
}

func (s *dsObjectStore) Run(context.Context) (err error) {
	return nil
}

func (s *dsObjectStore) Close(_ context.Context) (err error) {
	return nil
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
func (s *dsObjectStore) GetDetails(id string) (*model.ObjectDetails, error) {
	var details *model.ObjectDetails
	err := s.db.View(func(txn *badger.Txn) error {
		it, err := txn.Get(pagesDetailsBase.ChildString(id).Bytes())
		if err != nil {
			return fmt.Errorf("get details: %w", err)
		}
		details, err = s.extractDetailsFromItem(it)
		return err
	})
	if badgerhelper.IsNotFound(err) {
		return &model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil
	}

	if err != nil {
		return nil, err
	}
	return details, nil
}

func (s *dsObjectStore) GetUniqueKeyById(id string) (domain.UniqueKey, error) {
	details, err := s.GetDetails(id)
	if err != nil {
		return nil, err
	}
	rawUniqueKey := pbtypes.GetString(details.Details, bundle.RelationKeyUniqueKey.String())
	if rawUniqueKey == "" {
		return nil, fmt.Errorf("object %s does not have %s set in details", id, bundle.RelationKeyUniqueKey.String())
	}
	return domain.UnmarshalUniqueKey(rawUniqueKey)
}

func (s *dsObjectStore) List(spaceID string, includeArchived bool) ([]*model.ObjectInfo, error) {
	filters := []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeySpaceId.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(spaceID),
		},
	}
	if includeArchived {
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyIsArchived.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.Bool(true),
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
	err = s.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			_, err := txn.Get(pagesDetailsBase.ChildString(id).Bytes())
			if err != nil && err != badger.ErrKeyNotFound {
				return fmt.Errorf("get %s: %w", id, err)
			}
			if err == nil {
				exists = append(exists, id)
			}
		}
		return nil
	})
	return exists, err
}

func (s *dsObjectStore) GetByIDs(spaceID string, ids []string) ([]*model.ObjectInfo, error) {
	var infos []*model.ObjectInfo
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		infos, err = s.getObjectsInfo(txn, spaceID, ids)
		return err
	})
	return infos, err
}

func (s *dsObjectStore) ListIdsBySpace(spaceId string) ([]string, error) {
	ids, _, err := s.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	return ids, err
}

func (s *dsObjectStore) ListIds() ([]string, error) {
	var ids []string
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		ids, err = listIDsByPrefix(txn, pagesDetailsBase.Bytes())
		return err
	})
	return ids, err
}

func (s *dsObjectStore) Prefix() string {
	return pagesPrefix
}

// TODO objstore: Just use dependency injection
func (s *dsObjectStore) FTSearch() ftsearch.FTSearch {
	return s.fts
}

func (s *dsObjectStore) extractDetailsFromItem(it *badger.Item) (*model.ObjectDetails, error) {
	key := it.Key()
	if v, ok := s.cache.Get(key); ok {
		return v.(*model.ObjectDetails), nil
	}
	return s.unmarshalDetailsFromItem(it)
}

func (s *dsObjectStore) unmarshalDetailsFromItem(it *badger.Item) (*model.ObjectDetails, error) {
	var details *model.ObjectDetails
	err := it.Value(func(val []byte) error {
		var err error
		details, err = unmarshalDetails(detailsKeyToID(it.Key()), val)
		if err != nil {
			return fmt.Errorf("unmarshal details: %w", err)
		}
		s.cache.Set(it.Key(), details, int64(details.Size()))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get item value: %w", err)
	}
	return details, nil
}

func unmarshalDetails(id string, rawValue []byte) (*model.ObjectDetails, error) {
	result := &model.ObjectDetails{}
	if err := proto.Unmarshal(rawValue, result); err != nil {
		return nil, err
	}
	if result.Details == nil {
		result.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	if result.Details.Fields == nil {
		result.Details.Fields = map[string]*types.Value{}
	} else {
		pbtypes.StructDeleteEmptyFields(result.Details)
	}
	result.Details.Fields[database.RecordIDField] = pbtypes.ToValue(id)
	return result, nil
}

func detailsKeyToID(key []byte) string {
	return path.Base(string(key))
}

func (s *dsObjectStore) getObjectInfo(txn *badger.Txn, spaceID string, id string) (*model.ObjectInfo, error) {
	details, err := s.sourceService.DetailsFromIdBasedSource(id)
	if err == nil {
		details.Fields[database.RecordIDField] = pbtypes.ToValue(id)
		return &model.ObjectInfo{
			Id:      id,
			Details: details,
		}, nil
	}

	it, err := txn.Get(pagesDetailsBase.ChildString(id).Bytes())
	if err != nil {
		return nil, fmt.Errorf("get details: %w", err)
	}
	detailsModel, err := s.extractDetailsFromItem(it)
	if err != nil {
		return nil, err
	}
	details = detailsModel.Details
	snippet, err := badgerhelper.GetValueTxn(txn, pagesSnippetBase.ChildString(id).Bytes(), bytesToString)
	if err != nil && !badgerhelper.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get snippet: %w", err)
	}

	return &model.ObjectInfo{
		Id:      id,
		Details: details,
		Snippet: snippet,
	}, nil
}

func (s *dsObjectStore) getObjectsInfo(txn *badger.Txn, spaceID string, ids []string) ([]*model.ObjectInfo, error) {
	objects := make([]*model.ObjectInfo, 0, len(ids))
	for _, id := range ids {
		info, err := s.getObjectInfo(txn, spaceID, id)
		if err != nil {
			if badgerhelper.IsNotFound(err) || errors.Is(err, ErrObjectNotFound) || errors.Is(err, ErrNotAnObject) {
				continue
			}
			return nil, err
		}
		if f := info.GetDetails().GetFields(); f != nil {
			// skip deleted objects
			if v := f[bundle.RelationKeyIsDeleted.String()]; v != nil && v.GetBoolValue() {
				continue
			}
		}
		objects = append(objects, info)
	}

	return objects, nil
}

func (s *dsObjectStore) GetObjectByUniqueKey(spaceId string, uniqueKey domain.UniqueKey) (*model.ObjectDetails, error) {
	records, _, err := s.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Value:       pbtypes.String(uniqueKey.Marshal()),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceId),
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

	return &model.ObjectDetails{Details: records[0].Details}, nil
}

func listIDsByPrefix(txn *badger.Txn, prefix []byte) ([]string, error) {
	var ids []string
	err := iterateKeysByPrefixTx(txn, prefix, func(key []byte) {
		ids = append(ids, path.Base(string(key)))
	})
	return ids, err
}

func extractIdFromKey(key string) (id string) {
	i := strings.LastIndexByte(key, '/')
	if i == -1 || len(key)-1 == i {
		return
	}
	return key[i+1:]
}

// bytesToString unmarshalls bytes to string
func bytesToString(b []byte) (string, error) {
	return string(b), nil
}
