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
	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/ristretto"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/space/typeprovider"
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

	workspacesPrefix = "workspaces"
	currentWorkspace = ds.NewKey("/" + workspacesPrefix + "/current")

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
	s.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
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
	s.db, err = datastoreService.LocalstoreBadger()
	if err != nil {
		return fmt.Errorf("get badger: %w", err)
	}

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

	SubscribeForAll(callback func(rec database.Record))

	Query(schema schema.Schema, q database.Query) (records []database.Record, total int, err error)
	QueryRaw(f *database.Filters, limit int, offset int) (records []database.Record, err error)
	QueryByID(ids []string) (records []database.Record, err error)
	QueryByIDAndSubscribeForChanges(ids []string, subscription database.Subscription) (records []database.Record, close func(), err error)
	QueryObjectIDs(q database.Query, objectTypes []smartblock.SmartBlockType) (ids []string, total int, err error)

	HasIDs(ids ...string) (exists []string, err error)
	GetByIDs(ids ...string) ([]*model.ObjectInfo, error)
	List() ([]*model.ObjectInfo, error)
	ListIds() ([]string, error)

	// UpdateObjectDetails updates existing object or create if not missing. Should be used in order to amend existing indexes based on prev/new value
	// set discardLocalDetailsChanges to true in case the caller doesn't have local details in the State
	UpdateObjectDetails(id string, details *types.Struct) error
	UpdateObjectLinks(id string, links []string) error
	UpdateObjectSnippet(id string, snippet string) error
	UpdatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error

	DeleteObject(id string) error
	DeleteDetails(id string) error
	// EraseIndexes erase all indexes for objectstore. All objects need to be reindexed
	EraseIndexes() error

	GetAggregatedOptions(relationKey string) (options []*model.RelationOption, err error)
	GetDetails(id string) (*model.ObjectDetails, error)
	GetInboundLinksByID(id string) ([]string, error)
	GetOutboundLinksByID(id string) ([]string, error)
	GetRelationByID(id string) (relation *model.Relation, err error)
	GetRelationByKey(key string) (relation *model.Relation, err error)
	GetWithLinksInfoByID(id string) (*model.ObjectInfoWithLinks, error)
	GetObjectType(url string) (*model.ObjectType, error)
	HasObjectType(id string) (bool, error)
	GetObjectTypes(urls []string) (ots []*model.ObjectType, err error)
}

type IndexerStore interface {
	AddToIndexQueue(id string) error
	ListIDsFromFullTextQueue() ([]string, error)
	RemoveIDsFromFullTextQueue(ids []string)
	FTSearch() ftsearch.FTSearch

	// GetChecksums Used to get information about localstore state and decide do we need to reindex some objects
	GetChecksums() (checksums *model.ObjectStoreChecksums, err error)
	// SaveChecksums Used to save checksums and force reindex counter
	SaveChecksums(checksums *model.ObjectStoreChecksums) (err error)

	GetLastIndexedHeadsHash(id string) (headsHash string, err error)
	SaveLastIndexedHeadsHash(id string, headsHash string) (err error)
}

type AccountStore interface {
	GetAccountStatus() (status *coordinatorproto.SpaceStatusPayload, err error)
	SaveAccountStatus(status *coordinatorproto.SpaceStatusPayload) (err error)

	GetCurrentWorkspaceID() (string, error)
	SetCurrentWorkspaceID(workspaceID string) (err error)
	RemoveCurrentWorkspaceID() (err error)
}

var ErrNotAnObject = fmt.Errorf("not an object")

type dsObjectStore struct {
	sourceService SourceDetailsFromID

	cache *ristretto.Cache
	db    *badger.DB

	fts ftsearch.FTSearch

	sbtProvider typeprovider.SmartBlockTypeProvider

	sync.RWMutex
	onChangeCallback func(record database.Record)
	subscriptions    []database.Subscription
}

func (s *dsObjectStore) EraseIndexes() error {
	outboundRemoved, inboundRemoved, err := s.eraseLinks()
	if err != nil {
		log.Errorf("eraseLinks failed: %s", err)
	}
	log.Infof("eraseLinks: removed %d outbound links", outboundRemoved)
	log.Infof("eraseLinks: removed %d inbound links", inboundRemoved)
	return nil
}

func (s *dsObjectStore) eraseLinks() (outboundRemoved int, inboundRemoved int, err error) {
	err = retryOnConflict(func() error {
		txn := s.db.NewTransaction(true)
		defer txn.Discard()
		txn, outboundRemoved, err = s.removeByPrefixInTx(txn, pagesOutboundLinksBase.String())
		if err != nil {
			return fmt.Errorf("remove all outbound links: %w", err)
		}
		txn, inboundRemoved, err = s.removeByPrefixInTx(txn, pagesInboundLinksBase.String())
		if err != nil {
			return fmt.Errorf("remove all inbound links: %w", err)
		}
		return txn.Commit()
	})
	return
}

func (s *dsObjectStore) Run(context.Context) (err error) {
	return nil
}

func (s *dsObjectStore) Close(_ context.Context) (err error) {
	return nil
}

// GetAggregatedOptions returns aggregated options for specific relation. Options have a specific scope
func (s *dsObjectStore) GetAggregatedOptions(relationKey string) (options []*model.RelationOption, err error) {
	// todo: add workspace
	records, _, err := s.Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Value:       pbtypes.String(relationKey),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyType.String(),
				Value:       pbtypes.String(bundle.TypeKeyRelationOption.URL()),
			},
		},
	})

	for _, rec := range records {
		options = append(options, relationutils.OptionFromStruct(rec.Details).RelationOption)
	}
	return
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

func (s *dsObjectStore) GetRelationByID(id string) (*model.Relation, error) {
	det, err := s.GetDetails(id)
	if err != nil {
		return nil, err
	}

	if pbtypes.GetString(det.GetDetails(), bundle.RelationKeyType.String()) != bundle.TypeKeyRelation.URL() {
		return nil, fmt.Errorf("object %s is not a relation", id)
	}

	rel := relationutils.RelationFromStruct(det.GetDetails())
	return rel.Relation, nil
}

// GetRelationByKey is deprecated, should be used from relationService
func (s *dsObjectStore) GetRelationByKey(key string) (*model.Relation, error) {
	// todo: should pass workspace
	q := database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Value:       pbtypes.String(key),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyType.String(),
				Value:       pbtypes.String(bundle.TypeKeyRelation.URL()),
			},
		},
	}

	records, _, err := s.Query(nil, q)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, ds.ErrNotFound
	}

	rel := relationutils.RelationFromStruct(records[0].Details)

	return rel.Relation, nil
}

func (s *dsObjectStore) GetWithLinksInfoByID(id string) (*model.ObjectInfoWithLinks, error) {
	var res *model.ObjectInfoWithLinks
	err := s.db.View(func(txn *badger.Txn) error {
		pages, err := s.getObjectsInfo(txn, []string{id})
		if err != nil {
			return err
		}

		if len(pages) == 0 {
			return fmt.Errorf("page not found")
		}
		page := pages[0]

		inboundIds, err := findInboundLinks(txn, id)
		if err != nil {
			return fmt.Errorf("find inbound links: %w", err)
		}
		outboundsIds, err := findOutboundLinks(txn, id)
		if err != nil {
			return fmt.Errorf("find outbound links: %w", err)
		}

		inbound, err := s.getObjectsInfo(txn, inboundIds)
		if err != nil {
			return err
		}

		outbound, err := s.getObjectsInfo(txn, outboundsIds)
		if err != nil {
			return err
		}

		res = &model.ObjectInfoWithLinks{
			Id:   id,
			Info: page,
			Links: &model.ObjectLinksInfo{
				Inbound:  inbound,
				Outbound: outbound,
			},
		}
		return nil
	})
	return res, err
}

func (s *dsObjectStore) GetOutboundLinksByID(id string) ([]string, error) {
	var links []string
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		links, err = findOutboundLinks(txn, id)
		return err
	})
	return links, err
}

func (s *dsObjectStore) GetInboundLinksByID(id string) ([]string, error) {
	var links []string
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		links, err = findInboundLinks(txn, id)
		return err
	})
	return links, err
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
	if isNotFound(err) {
		return &model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil
	}

	if err != nil {
		return nil, err
	}
	return details, nil
}

func (s *dsObjectStore) List() ([]*model.ObjectInfo, error) {
	var infos []*model.ObjectInfo
	err := s.db.View(func(txn *badger.Txn) error {
		ids, err := listIDsByPrefix(txn, pagesDetailsBase.Bytes())
		if err != nil {
			return fmt.Errorf("list ids by prefix: %w", err)
		}

		infos, err = s.getObjectsInfo(txn, ids)
		return err
	})
	return infos, err
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

func (s *dsObjectStore) GetByIDs(ids ...string) ([]*model.ObjectInfo, error) {
	var infos []*model.ObjectInfo
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		infos, err = s.getObjectsInfo(txn, ids)
		return err
	})
	return infos, err
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

type order struct {
	filter.Order
}

func (o order) Compare(lhs, rhs interface{}) (comp int) {
	le := lhs.(database.Record)
	re := rhs.(database.Record)

	if o.Order != nil {
		comp = o.Order.Compare(le, re)
	}
	// when order isn't set or equal - sort by id
	if comp == 0 {
		if pbtypes.GetString(le.Details, "id") > pbtypes.GetString(re.Details, "id") {
			return 1
		}
		return -1
	}
	return comp
}

func (o order) CalcScore(_ interface{}) float64 {
	return 0
}

func (s *dsObjectStore) getObjectInfo(txn *badger.Txn, id string) (*model.ObjectInfo, error) {
	sbt, err := s.sbtProvider.Type(id)
	if err != nil {
		log.With("objectID", id).Errorf("failed to extract smartblock type %s", id) // todo rq: surpess error?
		return nil, ErrNotAnObject
	}
	if sbt == smartblock.SmartBlockTypeArchive {
		return nil, ErrNotAnObject
	}

	var details *types.Struct
	if indexDetails, _ := sbt.Indexable(); !indexDetails && s.sourceService != nil {
		details, err = s.sourceService.DetailsFromIdBasedSource(id)
		if err != nil {
			return nil, ErrObjectNotFound
		}
	} else {
		it, err := txn.Get(pagesDetailsBase.ChildString(id).Bytes())
		if err != nil {
			return nil, fmt.Errorf("get details: %w", err)
		}
		detailsModel, err := s.extractDetailsFromItem(it)
		if err != nil {
			return nil, err
		}
		details = detailsModel.Details
	}
	snippet, err := getValueTxn(txn, pagesSnippetBase.ChildString(id).Bytes(), bytesToString)
	if err != nil && !isNotFound(err) {
		return nil, fmt.Errorf("failed to get snippet: %w", err)
	}

	return &model.ObjectInfo{
		Id:         id,
		ObjectType: sbt.ToProto(),
		Details:    details,
		Snippet:    snippet,
	}, nil
}

func (s *dsObjectStore) getObjectsInfo(txn *badger.Txn, ids []string) ([]*model.ObjectInfo, error) {
	objects := make([]*model.ObjectInfo, 0, len(ids))
	for _, id := range ids {
		info, err := s.getObjectInfo(txn, id)
		if err != nil {
			if isNotFound(err) || err == ErrObjectNotFound || err == ErrNotAnObject {
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

// Find to which IDs specified one has outbound links.
func findOutboundLinks(txn *badger.Txn, id string) ([]string, error) {
	return listIDsByPrefix(txn, pagesOutboundLinksBase.ChildString(id).Bytes())
}

// Find from which IDs specified one has inbound links.
func findInboundLinks(txn *badger.Txn, id string) ([]string, error) {
	return listIDsByPrefix(txn, pagesInboundLinksBase.ChildString(id).Bytes())
}

func listIDsByPrefix(txn *badger.Txn, prefix []byte) ([]string, error) {
	var ids []string
	err := iterateKeysByPrefixTx(txn, prefix, func(key []byte) {
		ids = append(ids, path.Base(string(key)))
	})
	return ids, err
}

func pageLinkKeys(id string, out []string) []ds.Key {
	keys := make([]ds.Key, 0, 2*len(out))
	// links outgoing from specified node id
	for _, to := range out {
		keys = append(keys, outgoingLinkKey(id, to), inboundLinkKey(id, to))
	}
	return keys
}

func outgoingLinkKey(from, to string) ds.Key {
	return pagesOutboundLinksBase.ChildString(from).ChildString(to)
}

func inboundLinkKey(from, to string) ds.Key {
	return pagesInboundLinksBase.ChildString(to).ChildString(from)
}

func extractIDFromKey(key string) (id string) {
	i := strings.LastIndexByte(key, '/')
	if i == -1 || len(key)-1 == i {
		return
	}
	return key[i+1:]
}

func (s *dsObjectStore) HasObjectType(id string) (bool, error) {
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return bundle.HasObjectTypeID(id), nil
	}

	details, err := s.GetDetails(id)
	if err != nil {
		return false, err
	}

	if pbtypes.IsStructEmpty(details.GetDetails()) {
		return false, nil
	}
	if pbtypes.GetBool(details.GetDetails(), bundle.RelationKeyIsDeleted.String()) {
		return false, nil
	}
	if pbtypes.GetString(details.Details, bundle.RelationKeyType.String()) != bundle.TypeKeyObjectType.URL() {
		return false, nil
	}
	return true, nil
}

func (s *dsObjectStore) GetObjectType(url string) (*model.ObjectType, error) {
	if strings.HasPrefix(url, addr.BundledObjectTypeURLPrefix) {
		return bundle.GetTypeByUrl(url)
	}

	details, err := s.GetDetails(url)
	if err != nil {
		return nil, err
	}

	if pbtypes.IsStructEmpty(details.GetDetails()) {
		return nil, ErrObjectNotFound
	}

	if pbtypes.GetBool(details.GetDetails(), bundle.RelationKeyIsDeleted.String()) {
		return nil, fmt.Errorf("type was removed")
	}

	if pbtypes.GetString(details.Details, bundle.RelationKeyType.String()) != bundle.TypeKeyObjectType.URL() {
		return nil, fmt.Errorf("object %s is not an object type", url)
	}

	return s.extractObjectTypeFromDetails(details.Details, url), nil
}

func (s *dsObjectStore) extractObjectTypeFromDetails(details *types.Struct, url string) *model.ObjectType {
	objectType := &model.ObjectType{}
	s.fillObjectTypeWithRecommendedRelations(details, objectType)
	objectType.Name = pbtypes.GetString(details, bundle.RelationKeyName.String())
	objectType.Layout = model.ObjectTypeLayout(int(pbtypes.GetFloat64(details, bundle.RelationKeyRecommendedLayout.String())))
	objectType.IconEmoji = pbtypes.GetString(details, bundle.RelationKeyIconEmoji.String())
	objectType.Url = url
	objectType.IsArchived = pbtypes.GetBool(details, bundle.RelationKeyIsArchived.String())

	// we use Page for all custom object types
	objectType.Types = []model.SmartBlockType{model.SmartBlockType_Page}
	return objectType
}

func (s *dsObjectStore) fillObjectTypeWithRecommendedRelations(details *types.Struct, objectType *model.ObjectType) {
	// relationKeys := objectInfos[0].RelationKeys
	for _, relationID := range pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()) {
		relationKey, err := pbtypes.RelationIdToKey(relationID)
		if err == nil {
			//nolint:govet
			relation, err := s.GetRelationByKey(relationKey)
			if err == nil {
				objectType.RelationLinks = append(
					objectType.RelationLinks,
					(&relationutils.Relation{Relation: relation}).RelationLink(),
				)
			} else {
				log.Errorf("GetObjectType failed to get relation key from id: %s (%s)", err.Error(), relationID)
			}
		} else {
			log.Errorf("GetObjectType failed to get relation key from id: %s (%s)", err.Error(), relationID)
		}
	}
}

func (s *dsObjectStore) GetObjectTypes(urls []string) (ots []*model.ObjectType, err error) {
	ots = make([]*model.ObjectType, 0, len(urls))
	for _, url := range urls {
		ot, e := s.GetObjectType(url)
		if e != nil {
			err = e
		} else {
			ots = append(ots, ot)
		}
	}
	return
}

// bytesToString unmarshalls bytes to string
func bytesToString(b []byte) (string, error) {
	return string(b), nil
}
