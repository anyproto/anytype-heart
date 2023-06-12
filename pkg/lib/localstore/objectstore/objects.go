package objectstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"

	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/noctxds"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
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

func New(sbtProvider typeprovider.SmartBlockTypeProvider) ObjectStore {
	return &dsObjectStore{
		sbtProvider: sbtProvider,
	}
}

func NewWithLocalstore(ds noctxds.DSTxnBatching) ObjectStore {
	return &dsObjectStore{
		ds: ds,
	}
}

type SourceDetailsFromId interface {
	DetailsFromIdBasedSource(id string) (*types.Struct, error)
}

func (s *dsObjectStore) Init(a *app.App) (err error) {
	s.dsIface = a.MustComponent(datastore.CName).(datastore.Datastore)
	src := a.Component("source")
	if src != nil {
		s.sourceService = a.MustComponent("source").(SourceDetailsFromId)
	}
	fts := a.Component(ftsearch.CName)
	if fts == nil {
		log.Warnf("init objectstore without fulltext")
	} else {
		s.fts = fts.(ftsearch.FTSearch)
	}
	return nil
}

func (s *dsObjectStore) Name() (name string) {
	return CName
}

type ObjectStore interface {
	app.ComponentRunnable
	IndexerStore
	AccountStore
	localstore.Indexable

	SubscribeForAll(callback func(rec database.Record))

	Query(schema schema.Schema, q database.Query) (records []database.Record, total int, err error)
	QueryRaw(f *database.Filters) (records []database.Record, err error)
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
	// underlying storage
	ds            noctxds.DSTxnBatching
	dsIface       datastore.Datastore
	sourceService SourceDetailsFromId

	fts ftsearch.FTSearch

	// serializing page updates
	l sync.Mutex

	onChangeCallback func(record database.Record)

	subscriptions    []database.Subscription
	depSubscriptions []database.Subscription

	sbtProvider typeprovider.SmartBlockTypeProvider
}

func (s *dsObjectStore) EraseIndexes() (err error) {
	for _, idx := range s.Indexes() {
		err = localstore.EraseIndex(idx, s.ds)
		if err != nil {
			return
		}
	}

	err = s.eraseLinks()
	if err != nil {
		log.Errorf("eraseLinks failed: %s", err.Error())
	}

	return
}

func (s *dsObjectStore) eraseLinks() (err error) {
	n, err := removeByPrefix(s.ds, pagesOutboundLinksBase.String())
	if err != nil {
		return err
	}

	log.Infof("eraseLinks: removed %d outbound links", n)
	n, err = removeByPrefix(s.ds, pagesInboundLinksBase.String())
	if err != nil {
		return err
	}

	log.Infof("eraseLinks: removed %d inbound links", n)

	return nil
}

func (s *dsObjectStore) Run(context.Context) (err error) {
	lds, err := s.dsIface.LocalstoreDS()
	s.ds = noctxds.New(lds)
	return
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
	s.l.Lock()
	defer s.l.Unlock()
	subscription.Close()

	for i, sub := range s.subscriptions {
		if sub == subscription {
			s.subscriptions = append(s.subscriptions[:i], s.subscriptions[i+1:]...)
			break
		}
	}
}

func unmarshalDetails(id string, rawValue []byte) (*model.ObjectDetails, error) {
	var details model.ObjectDetails
	if err := proto.Unmarshal(rawValue, &details); err != nil {
		return nil, err
	}

	if details.Details == nil || details.Details.Fields == nil {
		details.Details = &types.Struct{Fields: map[string]*types.Value{}}
	} else {
		pbtypes.StructDeleteEmptyFields(details.Details)
	}
	details.Details.Fields[database.RecordIDField] = pbtypes.ToValue(id)
	return &details, nil
}

func (s *dsObjectStore) SubscribeForAll(callback func(rec database.Record)) {
	s.l.Lock()
	s.onChangeCallback = callback
	s.l.Unlock()
}

func (s *dsObjectStore) GetRelationByID(id string) (*model.Relation, error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

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

func (s *dsObjectStore) DeleteDetails(id string) error {
	s.l.Lock()
	defer s.l.Unlock()

	txn, err := s.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	// todo: remove all indexes with this object
	for _, k := range []ds.Key{
		pagesSnippetBase.ChildString(id),
		pagesDetailsBase.ChildString(id),
	} {
		if err = txn.Delete(k); err != nil {
			return err
		}
	}

	return txn.Commit()
}

// DeleteObject removes all details, leaving only id and isDeleted
func (s *dsObjectStore) DeleteObject(id string) error {
	// do not completely remove object details, so we can distinguish links to deleted and not-yet-loaded objects
	err := s.UpdateObjectDetails(id, &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():        pbtypes.String(id),
			bundle.RelationKeyIsDeleted.String(): pbtypes.Bool(true), // maybe we can store the date instead?
		},
	})
	if err != nil {
		if !errors.Is(err, ErrDetailsNotChanged) {
			return fmt.Errorf("failed to overwrite details and relations: %w", err)
		}
	}

	s.l.Lock()
	defer s.l.Unlock()
	txn, err := s.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	// todo: remove all indexes with this object
	for _, k := range []ds.Key{
		pagesSnippetBase.ChildString(id),
		indexQueueBase.ChildString(id),
		indexedHeadsState.ChildString(id),
	} {
		if err = txn.Delete(k); err != nil {
			return err
		}
	}

	_, err = removeByPrefixInTx(txn, pagesInboundLinksBase.String()+"/"+id+"/")
	if err != nil {
		return err
	}

	_, err = removeByPrefixInTx(txn, pagesOutboundLinksBase.String()+"/"+id+"/")
	if err != nil {
		return err
	}

	if s.fts != nil {
		err = s.removeFromIndexQueue(id)
		if err != nil {
			log.Errorf("error removing %s from index queue: %s", id, err)
		}
		if err := s.fts.Delete(id); err != nil {
			return err
		}
	}
	return txn.Commit()
}

func (s *dsObjectStore) GetWithLinksInfoByID(id string) (*model.ObjectInfoWithLinks, error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	pages, err := s.getObjectsInfo(txn, []string{id})
	if err != nil {
		return nil, err
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("page not found")
	}
	page := pages[0]

	inboundIds, err := findInboundLinks(txn, id)
	if err != nil {
		return nil, err
	}

	outboundsIds, err := findOutboundLinks(txn, id)
	if err != nil {
		return nil, err
	}

	inbound, err := s.getObjectsInfo(txn, inboundIds)
	if err != nil {
		return nil, err
	}

	outbound, err := s.getObjectsInfo(txn, outboundsIds)
	if err != nil {
		return nil, err
	}

	return &model.ObjectInfoWithLinks{
		Id:   id,
		Info: page,
		Links: &model.ObjectLinksInfo{
			Inbound:  inbound,
			Outbound: outbound,
		},
	}, nil
}

func (s *dsObjectStore) GetOutboundLinksByID(id string) ([]string, error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return findOutboundLinks(txn, id)
}

func (s *dsObjectStore) GetInboundLinksByID(id string) ([]string, error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return findInboundLinks(txn, id)
}

func (s *dsObjectStore) GetDetails(id string) (*model.ObjectDetails, error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return getObjectDetails(txn, id)
}

func (s *dsObjectStore) List() ([]*model.ObjectInfo, error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	ids, err := findByPrefix(txn, pagesDetailsBase.String()+"/", 0)
	if err != nil {
		return nil, err
	}

	return s.getObjectsInfo(txn, ids)
}

func (s *dsObjectStore) HasIDs(ids ...string) (exists []string, err error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	for _, id := range ids {
		if exist, err := hasObjectId(txn, id); err != nil {
			return nil, err
		} else if exist {
			exists = append(exists, id)
		}
	}
	return exists, nil
}

func (s *dsObjectStore) GetByIDs(ids ...string) ([]*model.ObjectInfo, error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return s.getObjectsInfo(txn, ids)
}

func (s *dsObjectStore) ListIds() ([]string, error) {
	txn, err := s.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return findByPrefix(txn, pagesDetailsBase.String()+"/", 0)
}

func (s *dsObjectStore) Prefix() string {
	return pagesPrefix
}

func (s *dsObjectStore) Indexes() []localstore.Index {
	return []localstore.Index{}
}

// TODO objstore: Just use dependency injection
func (s *dsObjectStore) FTSearch() ftsearch.FTSearch {
	return s.fts
}

func (s *dsObjectStore) makeFTSQuery(text string, dsq query.Query) (query.Query, error) {
	if s.fts == nil {
		return dsq, fmt.Errorf("fullText search not configured")
	}
	ids, err := s.fts.Search(text)
	if err != nil {
		return dsq, err
	}
	idsQuery := newIdsFilter(ids)
	dsq.Filters = append([]query.Filter{idsQuery}, dsq.Filters...)
	dsq.Orders = append([]query.Order{idsQuery}, dsq.Orders...)
	return dsq, nil
}

// getObjectDetails returns empty(not nil) details when not found in the DS
func getObjectDetails(txn noctxds.Txn, id string) (*model.ObjectDetails, error) {
	val, err := txn.Get(pagesDetailsBase.ChildString(id))
	if err != nil {
		if err != ds.ErrNotFound {
			return nil, fmt.Errorf("failed to get relations: %w", err)
		}
		// return empty details in case not found
		return &model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil
	}

	details, err := unmarshalDetails(id, val)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal details: %w", err)
	}

	return details, nil
}

func hasObjectId(txn noctxds.Txn, id string) (bool, error) {
	if exists, err := txn.Has(pagesDetailsBase.ChildString(id)); err != nil {
		return false, fmt.Errorf("failed to get details: %w", err)
	} else {
		return exists, nil
	}
}

func (s *dsObjectStore) getObjectInfo(txn noctxds.Txn, id string) (*model.ObjectInfo, error) {
	sbt, err := s.sbtProvider.Type(id)
	if err != nil {
		log.With("thread", id).Errorf("failed to extract smartblock type %s", id) // todo rq: surpess error?
		return nil, ErrNotAnObject
	}
	if sbt == smartblock.SmartBlockTypeArchive {
		return nil, ErrNotAnObject
	}

	var details *types.Struct
	if indexDetails, _ := sbt.Indexable(); !indexDetails {
		if s.sourceService != nil {
			details, err = s.sourceService.DetailsFromIdBasedSource(id)
			if err != nil {
				return nil, ErrObjectNotFound
			}
		}
	} else {
		detailsWrapped, err := getObjectDetails(txn, id)
		if err != nil {
			return nil, err
		}
		details = detailsWrapped.GetDetails()
	}

	var snippet string
	if val, err := txn.Get(pagesSnippetBase.ChildString(id)); err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get snippet: %w", err)
	} else {
		snippet = string(val)
	}

	return &model.ObjectInfo{
		Id:         id,
		ObjectType: sbt.ToProto(),
		Details:    details,
		Snippet:    snippet,
	}, nil
}

func (s *dsObjectStore) getObjectsInfo(txn noctxds.Txn, ids []string) ([]*model.ObjectInfo, error) {
	var objects []*model.ObjectInfo
	for _, id := range ids {
		info, err := s.getObjectInfo(txn, id)
		if err != nil {
			if strings.HasSuffix(err.Error(), "key not found") || err == ErrObjectNotFound || err == ErrNotAnObject {
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
func findOutboundLinks(txn noctxds.Txn, id string) ([]string, error) {
	return findByPrefix(txn, pagesOutboundLinksBase.String()+"/"+id+"/", 0)
}

// Find from which IDs specified one has inbound links.
func findInboundLinks(txn noctxds.Txn, id string) ([]string, error) {
	return findByPrefix(txn, pagesInboundLinksBase.String()+"/"+id+"/", 0)
}

func findByPrefix(txn noctxds.Txn, prefix string, limit int) ([]string, error) {
	results, err := txn.Query(query.Query{
		Prefix:   prefix,
		Limit:    limit,
		KeysOnly: true,
	})
	if err != nil {
		return nil, err
	}

	return localstore.GetLeavesFromResults(results)
}

// removeByPrefix query prefix and then remove keys in multiple TXs
func removeByPrefix(d noctxds.DSTxnBatching, prefix string) (int, error) {
	results, err := d.Query(query.Query{
		Prefix:   prefix,
		KeysOnly: true,
	})
	if err != nil {
		return 0, err
	}
	var keys []ds.Key
	for res := range results.Next() {
		keys = append(keys, ds.NewKey(res.Key))
	}
	b, err := d.Batch()
	if err != nil {
		return 0, err
	}
	var removed int
	for _, key := range keys {
		err := b.Delete(key)
		if err != nil {
			return removed, err
		}
		removed++
	}

	return removed, b.Commit()
}

func removeByPrefixInTx(txn noctxds.Txn, prefix string) (int, error) {
	results, err := txn.Query(query.Query{
		Prefix:   prefix,
		KeysOnly: true,
	})
	if err != nil {
		return 0, err
	}

	var removed int
	for res := range results.Next() {
		err := txn.Delete(ds.NewKey(res.Key))
		if err != nil {
			_ = results.Close()
			return removed, err
		}
		removed++
	}
	return removed, nil
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

func extractIdFromKey(key string) (id string) {
	i := strings.LastIndexByte(key, '/')
	if i == -1 || len(key)-1 == i {
		return
	}
	return key[i+1:]
}

func (s *dsObjectStore) GetObjectType(url string) (*model.ObjectType, error) {
	if strings.HasPrefix(url, addr.BundledObjectTypeURLPrefix) {
		return bundle.GetTypeByUrl(url)
	}
	objectInfos, err := s.GetByIDs(url)
	if err != nil {
		return nil, err
	}
	if len(objectInfos) == 0 {
		return nil, fmt.Errorf("object type not found in the index")
	}
	details := objectInfos[0].Details

	if pbtypes.GetString(details, bundle.RelationKeyType.String()) != bundle.TypeKeyObjectType.URL() {
		return nil, fmt.Errorf("object %s is not an object type", url)
	}

	return s.extractObjectTypeFromDetails(details, url), nil
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
