package objectstore

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	cafePb "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var log = logging.Logger("anytype-localstore")

const CName = "objectstore"

var (
	// ObjectInfo is stored in db key pattern:
	pagesPrefix        = "pages"
	pagesDetailsBase   = ds.NewKey("/" + pagesPrefix + "/details")
	pagesRelationsBase = ds.NewKey("/" + pagesPrefix + "/relations")     // store the list of full relation model for /objectId
	setRelationsBase   = ds.NewKey("/" + pagesPrefix + "/set/relations") // store the list of full relation model for /setId

	pagesSnippetBase       = ds.NewKey("/" + pagesPrefix + "/snippet")
	pagesInboundLinksBase  = ds.NewKey("/" + pagesPrefix + "/inbound")
	pagesOutboundLinksBase = ds.NewKey("/" + pagesPrefix + "/outbound")
	indexQueueBase         = ds.NewKey("/" + pagesPrefix + "/index")
	bundledChecksums       = ds.NewKey("/" + pagesPrefix + "/checksum")
	indexedHeadsState      = ds.NewKey("/" + pagesPrefix + "/headsstate")

	cafeConfig = ds.NewKey("/" + pagesPrefix + "/cafeconfig")

	workspacesPrefix = "workspaces"
	currentWorkspace = ds.NewKey("/" + workspacesPrefix + "/current")

	relationsPrefix = "relations"
	// /relations/options/<relOptionId>: option model
	relationsOptionsBase = ds.NewKey("/" + relationsPrefix + "/options")
	// /relations/relations/<relKey>: relation model
	relationsBase = ds.NewKey("/" + relationsPrefix + "/relations")

	// /relations/objtype_relkey_objid/<objType>/<relKey>/<objId>
	indexObjectTypeRelationObjectId = localstore.Index{
		Prefix: relationsPrefix,
		Name:   "objtype_relkey_objid",
		Keys: func(val interface{}) []localstore.IndexKeyParts {
			if v, ok := val.(*relationObjectType); ok {
				var indexes []localstore.IndexKeyParts
				for _, rk := range v.relationKeys {
					for _, ot := range v.objectTypes {
						otCompact, err := objTypeCompactEncode(ot)
						if err != nil {
							log.Errorf("objtype_relkey_objid index construction error(ot '%s'): %s", ot, err.Error())
							continue
						}

						indexes = append(indexes, localstore.IndexKeyParts([]string{otCompact, rk}))
					}
				}
				return indexes
			}
			return nil
		},
		Unique:             false,
		SplitIndexKeyParts: true,
	}

	// /relations/objtype_relkey_setid/<objType>/<relKey>/<setObjId>
	indexObjectTypeRelationSetId = localstore.Index{
		Prefix: relationsPrefix,
		Name:   "objtype_relkey_setid",
		Keys: func(val interface{}) []localstore.IndexKeyParts {
			if v, ok := val.(*relationObjectType); ok {
				var indexes []localstore.IndexKeyParts
				for _, rk := range v.relationKeys {
					for _, ot := range v.objectTypes {
						otCompact, err := objTypeCompactEncode(ot)
						if err != nil {
							log.Errorf("objtype_relkey_setid index construction error('%s'): %s", ot, err.Error())
							continue
						}

						indexes = append(indexes, localstore.IndexKeyParts([]string{otCompact, rk}))
					}
				}
				return indexes
			}
			return nil
		},
		Unique:             false,
		SplitIndexKeyParts: true,
	}

	// /relations/relkey_optid/<relKey>/<optId>/<objId>
	indexRelationOptionObject = localstore.Index{
		Prefix: pagesPrefix,
		Name:   "relkey_optid",
		Keys: func(val interface{}) []localstore.IndexKeyParts {
			if v, ok := val.(*model.Relation); ok {
				var indexes []localstore.IndexKeyParts
				if v.Format != model.RelationFormat_tag && v.Format != model.RelationFormat_status {
					return nil
				}
				if len(v.SelectDict) == 0 {
					return nil
				}

				for _, opt := range v.SelectDict {
					indexes = append(indexes, localstore.IndexKeyParts([]string{v.Key, opt.Id}))
				}
				return indexes
			}
			return nil
		},
		Unique:             false,
		SplitIndexKeyParts: true,
	}

	// /relations/relkey/<relKey>/<objId>
	indexRelationObject = localstore.Index{
		Prefix: pagesPrefix,
		Name:   "relkey",
		Keys: func(val interface{}) []localstore.IndexKeyParts {
			if v, ok := val.(*model.Relation); ok {
				return []localstore.IndexKeyParts{[]string{v.Key}}
			}
			return nil
		},
		Unique: false,
	}

	// /pages/type/<objType>/<objId>
	indexObjectTypeObject = localstore.Index{
		Prefix: pagesPrefix,
		Name:   "type",
		Keys: func(val interface{}) []localstore.IndexKeyParts {
			if v, ok := val.(*model.ObjectDetails); ok {
				var indexes []localstore.IndexKeyParts
				types := pbtypes.GetStringList(v.Details, bundle.RelationKeyType.String())

				for _, ot := range types {
					otCompact, err := objTypeCompactEncode(ot)
					if err != nil {
						log.Errorf("type index construction error('%s'): %s", ot, err.Error())
						continue
					}
					indexes = append(indexes, localstore.IndexKeyParts([]string{otCompact}))
				}
				return indexes
			}
			return nil
		},
		Unique: false,
		Hash:   false,
	}

	_ ObjectStore = (*dsObjectStore)(nil)
)

func New() ObjectStore {
	return &dsObjectStore{}
}

type DetailInjector interface {
	SetLocalDetails(id string, st *types.Struct)
}

type SourceIdEncodedDetails interface {
	GetDetailsFromIdBasedSource(id string) (*types.Struct, error)
}

func (ls *dsObjectStore) Init(a *app.App) (err error) {
	ls.dsIface = a.MustComponent(datastore.CName).(datastore.Datastore)
	meta := a.Component("meta")
	if meta != nil {
		ls.meta = meta.(DetailInjector)
	}
	s := a.Component("source")
	if s != nil {
		ls.sourceService = a.MustComponent("source").(SourceIdEncodedDetails)
	}
	fts := a.Component(ftsearch.CName)
	if fts == nil {
		log.Warnf("init objectstore without fulltext")
	} else {
		ls.fts = fts.(ftsearch.FTSearch)
	}
	return nil
}

func (ls *dsObjectStore) Name() (name string) {
	return CName
}

type ObjectStore interface {
	app.ComponentRunnable
	localstore.Indexable
	database.Reader

	// CreateObject create or overwrite an existing object. Should be used if
	CreateObject(id string, details *types.Struct, relations *model.Relations, links []string, snippet string) error
	// UpdateObjectDetails updates existing object or create if not missing. Should be used in order to amend existing indexes based on prev/new value
	// set discardLocalDetailsChanges to true in case the caller doesn't have local details in the State
	UpdateObjectDetails(id string, details *types.Struct, relations *model.Relations, discardLocalDetailsChanges bool) error
	InjectObjectDetails(id string, details *types.Struct) (mergedDetails *types.Struct, err error)
	UpdateObjectLinks(id string, links []string) error
	UpdateObjectSnippet(id string, snippet string) error

	DeleteObject(id string) error
	RemoveRelationFromCache(key string) error

	UpdateRelationsInSet(setId, objType, creatorId string, relations []*model.Relation) error

	GetWithLinksInfoByID(id string) (*model.ObjectInfoWithLinks, error)
	GetOutboundLinksById(id string) ([]string, error)
	GetWithOutboundLinksInfoById(id string) (*model.ObjectInfoWithOutboundLinks, error)
	GetDetails(id string) (*model.ObjectDetails, error)
	GetAggregatedOptions(relationKey string, objectType string) (options []*model.RelationOption, err error)

	HasIDs(ids ...string) (exists []string, err error)
	GetByIDs(ids ...string) ([]*model.ObjectInfo, error)
	List() ([]*model.ObjectInfo, error)
	ListIds() ([]string, error)

	QueryObjectInfo(q database.Query, objectTypes []smartblock.SmartBlockType) (results []*model.ObjectInfo, total int, err error)
	AddToIndexQueue(id string) error
	IndexForEach(f func(id string, tm time.Time) error) error
	FTSearch() ftsearch.FTSearch

	// EraseIndexes erase all indexes for objectstore.. All objects needs to be reindexed
	EraseIndexes() error

	// GetChecksums Used to get information about localstore state and decide do we need to reindex some objects
	GetChecksums() (checksums *model.ObjectStoreChecksums, err error)
	// SaveChecksums Used to save checksums and force reindex counter
	SaveChecksums(checksums *model.ObjectStoreChecksums) (err error)

	GetLastIndexedHeadsHash(id string) (headsHash string, err error)
	SaveLastIndexedHeadsHash(id string, headsHash string) (err error)

	GetCafeConfig() (cfg *cafePb.GetConfigResponseConfig, err error)
	SaveCafeConfig(cfg *cafePb.GetConfigResponseConfig) (err error)

	GetCurrentWorkspaceId() (string, error)
	SetCurrentWorkspaceId(threadId string) (err error)
	RemoveCurrentWorkspaceId() (err error)
}

type relationOption struct {
	relationKey string
	optionId    string
}

type relationObjectType struct {
	relationKeys []string
	objectTypes  []string
}

var ErrNotAnObject = fmt.Errorf("not an object")

var filterNotSystemObjects = &filterSmartblockTypes{
	smartBlockTypes: []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeHome,
	},
	not: true,
}

type filterSmartblockTypes struct {
	smartBlockTypes []smartblock.SmartBlockType
	not             bool
}

type RelationWithObjectType struct {
	objectType string
	relation   *model.Relation
}

func (m *filterSmartblockTypes) Filter(e query.Entry) bool {
	keyParts := strings.Split(e.Key, "/")
	id := keyParts[len(keyParts)-1]

	t, err := smartblock.SmartBlockTypeFromID(id)
	if err != nil {
		log.Errorf("failed to detect smartblock type for %s: %s", id, err.Error())
		return false
	}

	for _, ot := range m.smartBlockTypes {
		if t == ot {
			return !m.not
		}
	}
	return m.not
}

type dsObjectStore struct {
	// underlying storage
	ds            ds.TxnDatastore
	dsIface       datastore.Datastore
	sourceService SourceIdEncodedDetails

	meta DetailInjector // TODO: remove after we will migrate to the objectStore subscriptions

	fts ftsearch.FTSearch

	// serializing page updates
	l sync.Mutex

	subscriptions    []database.Subscription
	depSubscriptions []database.Subscription
}

func (m *dsObjectStore) GetCurrentWorkspaceId() (string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return "", fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	val, err := txn.Get(currentWorkspace)
	if err != nil {
		return "", err
	}
	return string(val), nil
}

func (m *dsObjectStore) SetCurrentWorkspaceId(threadId string) (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Put(currentWorkspace, []byte(threadId)); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}

func (m *dsObjectStore) RemoveCurrentWorkspaceId() (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Delete(currentWorkspace); err != nil {
		return fmt.Errorf("failed to delete from ds: %w", err)
	}

	return txn.Commit()
}

func (m *dsObjectStore) GetCafeConfig() (cfg *cafePb.GetConfigResponseConfig, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	var cafecfg cafePb.GetConfigResponseConfig
	if val, err := txn.Get(cafeConfig); err != nil {
		return nil, err
	} else if err := proto.Unmarshal(val, &cafecfg); err != nil {
		return nil, err
	}

	return &cafecfg, nil
}

func (m *dsObjectStore) SaveCafeConfig(cfg *cafePb.GetConfigResponseConfig) (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	b, err := cfg.Marshal()
	if err != nil {
		return err
	}

	if err := txn.Put(cafeConfig, b); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}

func (m *dsObjectStore) EraseIndexes() (err error) {
	for _, idx := range m.Indexes() {
		err = localstore.EraseIndex(idx, m.ds)
		if err != nil {
			return
		}
	}
	err = m.eraseStoredRelations()
	if err != nil {
		log.Errorf("eraseStoredRelations failed: %s", err.Error())
	}

	err = m.eraseLinks()
	if err != nil {
		log.Errorf("eraseLinks failed: %s", err.Error())
	}

	return
}

func (m *dsObjectStore) eraseStoredRelations() (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return err
	}

	defer txn.Discard()
	res, err := localstore.GetKeys(txn, setRelationsBase.String(), 0)
	if err != nil {
		return err
	}

	keys, err := localstore.ExtractKeysFromResults(res)
	if err != nil {
		return err
	}

	for _, key := range keys {
		err = txn.Delete(ds.NewKey(key))
		if err != nil {
			log.Errorf("eraseStoredRelations: failed to delete key %s: %s", key, err.Error())
		}
	}
	return txn.Commit()
}

func (m *dsObjectStore) eraseLinks() (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return err
	}
	defer txn.Discard()
	n, err := removeByPrefix(txn, pagesOutboundLinksBase.String())
	if err != nil {
		return err
	}

	log.Infof("eraseLinks: removed %d outbound links", n)
	n, err = removeByPrefix(txn, pagesInboundLinksBase.String())
	if err != nil {
		return err
	}

	log.Infof("eraseLinks: removed %d inbound links", n)

	return txn.Commit()
}

func (m *dsObjectStore) Run() (err error) {
	m.ds, err = m.dsIface.LocalstoreDS()
	return
}

func (m *dsObjectStore) Close() (err error) {
	return nil
}

func (m *dsObjectStore) AggregateObjectIdsForOptionAndRelation(relationKey, optId string) (objectsIds []string, err error) {
	txn, err := m.ds.NewTransaction(true)
	defer txn.Discard()

	res, err := localstore.GetKeysByIndexParts(txn, pagesPrefix, indexRelationOptionObject.Name, []string{relationKey, optId}, "/", false, 0)
	if err != nil {
		return nil, err
	}

	return localstore.GetLeavesFromResults(res)
}

func (m *dsObjectStore) AggregateObjectIdsByOptionForRelation(relationKey string) (objectsByOptionId map[string][]string, err error) {
	txn, err := m.ds.NewTransaction(true)
	defer txn.Discard()

	res, err := localstore.GetKeysByIndexParts(txn, pagesPrefix, indexRelationOptionObject.Name, []string{relationKey}, "/", false, 0)
	if err != nil {
		return nil, err
	}

	keys, err := localstore.ExtractKeysFromResults(res)
	if err != nil {
		return nil, err
	}

	objectsByOptionId = make(map[string][]string)

	for _, key := range keys {
		optionId, err := localstore.CarveKeyParts(key, -2, -1)
		if err != nil {
			return nil, err
		}
		objId, err := localstore.CarveKeyParts(key, -1, 0)
		if err != nil {
			return nil, err
		}

		if _, exists := objectsByOptionId[optionId]; !exists {
			objectsByOptionId[optionId] = []string{}
		}

		objectsByOptionId[optionId] = append(objectsByOptionId[optionId], objId)
	}
	return
}

// GetAggregatedOptions returns aggregated options for specific relation. Options have a specific scope
func (m *dsObjectStore) GetAggregatedOptions(relationKey string, objectType string) (options []*model.RelationOption, err error) {
	objectsByOptionId, err := m.AggregateObjectIdsByOptionForRelation(relationKey)
	if err != nil {
		return nil, err
	}

	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, err
	}

	for optId, objIds := range objectsByOptionId {
		var scope = model.RelationOption_relation
		for _, objId := range objIds {
			exists, err := isObjectBelongToType(txn, objId, objectType)
			if err != nil {
				return nil, err
			}

			if exists {
				scope = model.RelationOption_local
				break
			}
		}
		opt, err := getOption(txn, optId)
		if err != nil {
			return nil, err
		}
		opt.Scope = scope
		options = append(options, opt)
	}

	return
}

func (m *dsObjectStore) QueryAndSubscribeForChanges(schema *schema.Schema, q database.Query, sub database.Subscription) (records []database.Record, close func(), total int, err error) {
	m.l.Lock()
	defer m.l.Unlock()

	records, total, err = m.Query(schema, q)

	var ids []string
	for _, record := range records {
		ids = append(ids, pbtypes.GetString(record.Details, bundle.RelationKeyId.String()))
	}

	sub.Subscribe(ids)
	m.addSubscriptionIfNotExists(sub)
	close = func() {
		m.closeAndRemoveSubscription(sub)
	}

	return
}

// unsafe, use under mutex
func (m *dsObjectStore) addSubscriptionIfNotExists(sub database.Subscription) (existed bool) {
	for _, s := range m.subscriptions {
		if s == sub {
			return true
		}
	}

	m.subscriptions = append(m.subscriptions, sub)
	return false
}

func (m *dsObjectStore) closeAndRemoveSubscription(sub database.Subscription) {
	m.l.Lock()
	defer m.l.Unlock()
	sub.Close()

	for i, s := range m.subscriptions {
		if s == sub {
			m.subscriptions = append(m.subscriptions[:i], m.subscriptions[i+1:]...)
			break
		}
	}
}

func (m *dsObjectStore) QueryByIdAndSubscribeForChanges(ids []string, sub database.Subscription) (records []database.Record, close func(), err error) {
	m.l.Lock()
	defer m.l.Unlock()

	sub.Subscribe(ids)
	records, err = m.QueryById(ids)

	close = func() {
		m.closeAndRemoveSubscription(sub)
	}

	m.addSubscriptionIfNotExists(sub)

	return
}

func (m *dsObjectStore) Query(sch *schema.Schema, q database.Query) (records []database.Record, total int, err error) {
	workspaceId, err := m.GetCurrentWorkspaceId()
	if err == nil {
		q.WorkspaceId = workspaceId
	}

	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	dsq, err := q.DSQuery(sch)
	if err != nil {
		return
	}
	dsq.Offset = 0
	dsq.Limit = 0
	dsq.Prefix = pagesDetailsBase.String() + "/"
	if !q.WithSystemObjects {
		dsq.Filters = append([]query.Filter{filterNotSystemObjects}, dsq.Filters...)
	}
	if len(q.ObjectTypeFilter) > 0 {
		dsq.Filters = append([]query.Filter{m.objectTypeFilter(q.ObjectTypeFilter...)}, dsq.Filters...)
	}
	if q.FullText != "" {
		if dsq, err = m.makeFTSQuery(q.FullText, dsq); err != nil {
			return
		}
	}
	for _, f := range dsq.Filters {
		log.Debugf("query filter: %+v", f)
	}
	res, err := txn.Query(dsq)
	if err != nil {
		return nil, 0, fmt.Errorf("error when querying ds: %w", err)
	}

	var (
		results []database.Record
		offset  = q.Offset
	)

	// We use own limit/offset implementation in order to find out
	// total number of records matching specified filters. Query
	// returns this number for handy pagination on clients.
	for rec := range res.Next() {
		total++

		if offset > 0 {
			offset--
			continue
		}

		if q.Limit > 0 && len(results) >= q.Limit {
			continue
		}

		var details model.ObjectDetails
		if err = proto.Unmarshal(rec.Value, &details); err != nil {
			log.Errorf("failed to unmarshal: %s", err.Error())
			total--
			continue
		}

		key := ds.NewKey(rec.Key)
		keyList := key.List()
		id := keyList[len(keyList)-1]

		if details.Details == nil || details.Details.Fields == nil {
			details.Details = &types.Struct{Fields: map[string]*types.Value{}}
		} else {
			pb.StructDeleteEmptyFields(details.Details)
		}

		details.Details.Fields[database.RecordIDField] = pb.ToValue(id)
		results = append(results, database.Record{Details: details.Details})
	}

	return results, total, nil
}

func (m *dsObjectStore) objectTypeFilter(ots ...string) query.Filter {
	var filter filterSmartblockTypes
	for _, otUrl := range ots {
		if ot, err := bundle.GetTypeByUrl(otUrl); err == nil {
			for _, sbt := range ot.Types {
				filter.smartBlockTypes = append(filter.smartBlockTypes, smartblock.SmartBlockType(sbt))
			}
			continue
		}
		if sbt, err := smartblock.SmartBlockTypeFromID(otUrl); err == nil {
			filter.smartBlockTypes = append(filter.smartBlockTypes, sbt)
		}
	}
	return &filter
}

func (m *dsObjectStore) QueryObjectInfo(q database.Query, objectTypes []smartblock.SmartBlockType) (results []*model.ObjectInfo, total int, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	dsq, err := q.DSQuery(nil)
	if err != nil {
		return
	}
	dsq.Offset = 0
	dsq.Limit = 0
	dsq.Prefix = pagesDetailsBase.String() + "/"
	if len(objectTypes) > 0 {
		dsq.Filters = append([]query.Filter{&filterSmartblockTypes{smartBlockTypes: objectTypes}}, dsq.Filters...)
	}
	if q.FullText != "" {
		if dsq, err = m.makeFTSQuery(q.FullText, dsq); err != nil {
			return
		}
	}
	res, err := txn.Query(dsq)
	if err != nil {
		return nil, 0, fmt.Errorf("error when querying ds: %w", err)
	}

	var (
		offset = q.Offset
	)

	// We use own limit/offset implementation in order to find out
	// total number of records matching specified filters. Query
	// returns this number for handy pagination on clients.
	for rec := range res.Next() {
		if rec.Error != nil {
			return nil, 0, rec.Error
		}
		total++

		if offset > 0 {
			offset--
			continue
		}

		if q.Limit > 0 && len(results) >= q.Limit {
			continue
		}

		key := ds.NewKey(rec.Key)
		keyList := key.List()
		id := keyList[len(keyList)-1]
		oi, err := m.getObjectInfo(txn, id)
		if err != nil {
			// probably details are not yet indexed, let's skip it
			log.Errorf("QueryObjectInfo getObjectInfo error: %s", err.Error())
			total--
			continue
		}
		results = append(results, oi)
	}
	return results, total, nil
}

func (m *dsObjectStore) QueryById(ids []string) (records []database.Record, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	for _, id := range ids {
		v, err := txn.Get(pagesDetailsBase.ChildString(id))
		if err != nil {
			log.Errorf("QueryByIds failed to find id: %s", id)
			continue
		}

		var details model.ObjectDetails
		if err = proto.Unmarshal(v, &details); err != nil {
			log.Errorf("QueryByIds failed to unmarshal id: %s", id)
			continue
		}

		if details.Details == nil || details.Details.Fields == nil {
			details.Details = &types.Struct{Fields: map[string]*types.Value{}}
		}

		details.Details.Fields[database.RecordIDField] = pb.ToValue(id)
		records = append(records, database.Record{Details: details.Details})
	}

	return
}

func (m *dsObjectStore) GetRelation(relationKey string) (*model.Relation, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return getRelation(txn, relationKey)
}

// ListRelations retrieves all available relations and sort them in this order:
// 1. extraRelations aggregated from object of specific type (scope objectsOfTheSameType)
// 2. relations aggregated from sets of specific type  (scope setsOfTheSameType)
// 3. user-defined relations aggregated from all objects (scope library)
// 4. the rest of bundled relations (scope library)
func (m *dsObjectStore) ListRelations(objType string) ([]*model.Relation, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if objType == "" {
		rels, err := m.listRelations(txn, 0)
		if err != nil {
			return nil, err
		}
		// todo: omit when we will have everything in index
		relsKeys2 := bundle.ListRelationsKeys()
		for _, relKey := range relsKeys2 {
			if pbtypes.HasRelation(rels, relKey.String()) {
				continue
			}

			rel := bundle.MustGetRelation(relKey)
			rel.Scope = model.Relation_library
			rels = append(rels, rel)
		}
		return rels, nil
	}

	rels, err := m.AggregateRelationsFromObjectsOfType(objType)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate relations from objects: %w", err)
	}

	rels2, err := m.AggregateRelationsFromSetsOfType(objType)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate relations from sets: %w", err)
	}

	for i, rel := range rels2 {
		if pbtypes.HasRelation(rels, rel.Key) {
			continue
		}
		rels = append(rels, rels2[i])
	}

	relsKeys, err := m.listRelationsKeys(txn)
	if err != nil {
		return nil, fmt.Errorf("failed to list relations from store index: %w", err)
	}

	// todo: omit when we will have everything in index
	for _, relKey := range relsKeys {
		if pbtypes.HasRelation(rels, relKey) {
			continue
		}
		rel, err := getRelation(txn, relKey)
		if err != nil {
			log.Errorf("relation found in index but failed to retrieve from store")
			continue
		}
		rel.Scope = model.Relation_library
		rels = append(rels, rel)
	}

	relsKeys2 := bundle.ListRelationsKeys()
	for _, relKey := range relsKeys2 {
		if pbtypes.HasRelation(rels, relKey.String()) {
			continue
		}

		rel := bundle.MustGetRelation(relKey)
		rel.Scope = model.Relation_library
		rels = append(rels, rel)
	}

	return rels, nil
}

func (m *dsObjectStore) ListRelationsKeys() ([]string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return m.listRelationsKeys(txn)
}

func (m *dsObjectStore) AggregateRelationsFromObjectsOfType(objType string) ([]*model.Relation, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	var rels []*model.Relation
	objTypeCompact, err := objTypeCompactEncode(objType)
	if err != nil {
		return nil, fmt.Errorf("failed to encode object type '%s': %s", objType, err.Error())
	}
	res, err := localstore.GetKeysByIndexParts(txn, indexObjectTypeRelationObjectId.Prefix, indexObjectTypeRelationObjectId.Name, []string{objTypeCompact}, "/", false, 0)
	if err != nil {
		return nil, err
	}

	relKeys, err := localstore.GetKeyPartFromResults(res, -2, -1, true)
	if err != nil {
		return nil, err
	}

	for _, relKey := range relKeys {
		rel, err := getRelation(txn, relKey)
		if err != nil {
			log.Errorf("relation '%s' found in the index but failed to retreive: %s", relKey, err.Error())
			continue
		}

		rel.Scope = model.Relation_objectsOfTheSameType
		rels = append(rels, rel)
	}

	return rels, nil
}

func (m *dsObjectStore) AggregateRelationsFromSetsOfType(objType string) ([]*model.Relation, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	var rels []*model.Relation
	objTypeCompact, err := objTypeCompactEncode(objType)
	if err != nil {
		return nil, err
	}
	res, err := localstore.GetKeysByIndexParts(txn, indexObjectTypeRelationSetId.Prefix, indexObjectTypeRelationSetId.Name, []string{objTypeCompact}, "/", false, 0)
	if err != nil {
		return nil, err
	}

	relKeys, err := localstore.GetKeyPartFromResults(res, -2, -1, true)
	if err != nil {
		return nil, err
	}

	for _, relKey := range relKeys {
		rel, err := getRelation(txn, relKey)
		if err != nil {
			log.Errorf("relation '%s' found in the index but failed to retreive: %s", relKey, err.Error())
			continue
		}

		rel.Scope = model.Relation_setOfTheSameType
		rels = append(rels, rel)
	}

	return rels, nil
}

func (m *dsObjectStore) DeleteObject(id string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	// todo: remove all indexes with this object
	for _, k := range []ds.Key{
		pagesDetailsBase.ChildString(id),
		pagesSnippetBase.ChildString(id),
		pagesRelationsBase.ChildString(id),
		indexQueueBase.ChildString(id),
	} {
		if err = txn.Delete(k); err != nil {
			return err
		}
	}

	inLinks, err := findInboundLinks(txn, id)
	if err != nil {
		return err
	}

	outLinks, err := findOutboundLinks(txn, id)
	if err != nil {
		return err
	}

	for _, k := range pageLinkKeys(id, inLinks, outLinks) {
		if err := txn.Delete(k); err != nil {
			return err
		}
	}
	if m.fts != nil {
		if err := m.fts.Delete(id); err != nil {
			return err
		}
	}
	return txn.Commit()
}

// RemoveRelationFromCache removes cached relation data
func (m *dsObjectStore) RemoveRelationFromCache(key string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	for _, k := range []ds.Key{
		relationsBase.ChildString(key),
	} {
		if err = txn.Delete(k); err != nil {
			return err
		}
	}

	return txn.Commit()
}

func (m *dsObjectStore) GetWithLinksInfoByID(id string) (*model.ObjectInfoWithLinks, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	pages, err := m.getObjectsInfo(txn, []string{id})
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

	inbound, err := m.getObjectsInfo(txn, inboundIds)
	if err != nil {
		return nil, err
	}

	outbound, err := m.getObjectsInfo(txn, outboundsIds)
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

func (m *dsObjectStore) GetOutboundLinksById(id string) ([]string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return findOutboundLinks(txn, id)
}

func (m *dsObjectStore) GetWithOutboundLinksInfoById(id string) (*model.ObjectInfoWithOutboundLinks, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	pages, err := m.getObjectsInfo(txn, []string{id})
	if err != nil {
		return nil, err
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("page not found")
	}
	page := pages[0]

	outboundsIds, err := findOutboundLinks(txn, id)
	if err != nil {
		return nil, err
	}

	outbound, err := m.getObjectsInfo(txn, outboundsIds)
	if err != nil {
		return nil, err
	}

	return &model.ObjectInfoWithOutboundLinks{
		Info:          page,
		OutboundLinks: outbound,
	}, nil
}

func (m *dsObjectStore) GetDetails(id string) (*model.ObjectDetails, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return getObjectDetails(txn, id)
}

func (m *dsObjectStore) List() ([]*model.ObjectInfo, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	ids, err := findByPrefix(txn, pagesDetailsBase.String()+"/", 0)
	if err != nil {
		return nil, err
	}

	return m.getObjectsInfo(txn, ids)
}

func (m *dsObjectStore) HasIDs(ids ...string) (exists []string, err error) {
	txn, err := m.ds.NewTransaction(true)
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

func (m *dsObjectStore) GetByIDs(ids ...string) ([]*model.ObjectInfo, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return m.getObjectsInfo(txn, ids)
}

func (m *dsObjectStore) CreateObject(id string, details *types.Struct, relations *model.Relations, links []string, snippet string) error {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	// init an empty state to skip nil checks later
	before := model.ObjectInfo{
		Details: &types.Struct{Fields: map[string]*types.Value{}},
	}

	err = m.updateObjectDetails(txn, id, before, details, relations)
	if err != nil {
		return err
	}

	err = m.updateObjectLinksAndSnippet(txn, id, links, snippet)
	if err != nil {
		return err
	}
	return txn.Commit()
}

func (m *dsObjectStore) UpdateObjectLinks(id string, links []string) error {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	err = m.updateObjectLinks(txn, id, links)
	if err != nil {
		return err
	}
	return txn.Commit()
}

func (m *dsObjectStore) UpdateObjectSnippet(id string, snippet string) error {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if val, err := txn.Get(pagesSnippetBase.ChildString(id)); err == ds.ErrNotFound || string(val) != snippet {
		if err := m.updateSnippet(txn, id, snippet); err != nil {
			return err
		}
	}
	return txn.Commit()
}

func (m *dsObjectStore) UpdateObjectDetails(id string, details *types.Struct, relations *model.Relations, discardLocalDetailsChanges bool) error {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	var (
		before model.ObjectInfo
	)

	if details != nil || relations != nil {
		exInfo, err := m.getObjectInfo(txn, id)
		if err != nil {
			log.Debugf("UpdateObject failed to get ex state for object %s: %s", id, err.Error())
		}

		if exInfo != nil {
			before = *exInfo
		} else {
			// init an empty state to skip nil checks later
			before = model.ObjectInfo{
				Details: &types.Struct{Fields: map[string]*types.Value{}},
			}
		}

		if discardLocalDetailsChanges && details != nil {
			injectedDetails := pbtypes.StructFilterKeys(before.Details, bundle.LocalRelationsKeys)
			for k, v := range injectedDetails.Fields {
				details.Fields[k] = pbtypes.CopyVal(v)
			}
		}
	}

	err = m.updateObjectDetails(txn, id, before, details, relations)
	if err != nil {
		return err
	}
	err = txn.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (m *dsObjectStore) InjectObjectDetails(id string, details *types.Struct) (mergedDetails *types.Struct, err error) {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	mergedDetails, err = m.injectObjectDetails(txn, id, details)
	if err != nil {
		return mergedDetails, err
	}

	err = txn.Commit()
	if err != nil {
		return mergedDetails, err
	}

	return mergedDetails, nil
}

func (m *dsObjectStore) injectObjectDetails(txn ds.Txn, id string, details *types.Struct) (mergedDetails *types.Struct, err error) {
	detailsBefore, err := getObjectDetails(txn, id)
	if err != nil {
		return nil, err
	}

	if details == nil {
		return detailsBefore.GetDetails(), nil
	}

	mergedDetails = pbtypes.CopyStruct(detailsBefore.GetDetails())
	for k, v := range details.Fields {
		mergedDetails.Fields[k] = v
	}

	err = m.updateDetails(txn, id, detailsBefore, &model.ObjectDetails{Details: mergedDetails})
	if err != nil {
		return mergedDetails, err
	}

	return mergedDetails, nil
}

// GetLastIndexedHeadsHash return empty hash without error if record was not found
func (m *dsObjectStore) GetLastIndexedHeadsHash(id string) (headsHash string, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return "", fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if val, err := txn.Get(indexedHeadsState.ChildString(id)); err != nil && err != ds.ErrNotFound {
		return "", fmt.Errorf("failed to get heads hash: %w", err)
	} else if val == nil {
		return "", nil
	} else {
		return string(val), nil
	}
}

func (m *dsObjectStore) SaveLastIndexedHeadsHash(id string, headsHash string) (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Put(indexedHeadsState.ChildString(id), []byte(headsHash)); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}

func (m *dsObjectStore) GetChecksums() (checksums *model.ObjectStoreChecksums, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	var objChecksum model.ObjectStoreChecksums
	if val, err := txn.Get(bundledChecksums); err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get details: %w", err)
	} else if err := proto.Unmarshal(val, &objChecksum); err != nil {
		return nil, err
	}

	return &objChecksum, nil
}

func (m *dsObjectStore) SaveChecksums(checksums *model.ObjectStoreChecksums) (err error) {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	b, err := checksums.Marshal()
	if err != nil {
		return err
	}

	if err := txn.Put(bundledChecksums, b); err != nil {
		return fmt.Errorf("failed to put into ds: %w", err)
	}

	return txn.Commit()
}

func (m *dsObjectStore) updateLinksBasedLocalRelation(txn ds.Txn, key bundle.RelationKey, exLinks, links []string) error {
	removedLinks, addedLinks := slice.DifferenceRemovedAdded(exLinks, links)

	setDetail := func(id string, val bool) error {
		merged, err := m.injectObjectDetails(txn, id, &types.Struct{Fields: map[string]*types.Value{key.String(): pbtypes.Bool(val)}})
		if err != nil {
			return err
		}

		// inject localDetails into the meta pubsub
		m.meta.SetLocalDetails(id, merged)

		return nil
	}

	var err error
	for _, objId := range removedLinks {
		err = setDetail(objId, false)
		if err != nil {
			return fmt.Errorf("failed to setDetail: %s", err.Error())
		}
	}

	for _, objId := range addedLinks {
		err = setDetail(objId, true)
		if err != nil {
			return fmt.Errorf("failed to setDetail: %s", err.Error())
		}
	}

	return nil
}

func (m *dsObjectStore) updateObjectLinks(txn ds.Txn, id string, links []string) error {
	sbt, err := smartblock.SmartBlockTypeFromID(id)
	if err != nil {
		return fmt.Errorf("failed to extract smartblock type: %w", err)
	}

	exLinks, _ := findOutboundLinks(txn, id)
	if sbt == smartblock.SmartBlockTypeArchive {
		err = m.updateLinksBasedLocalRelation(txn, bundle.RelationKeyIsArchived, exLinks, links)
		if err != nil {
			return err
		}
	} else if sbt == smartblock.SmartBlockTypeHome {
		err = m.updateLinksBasedLocalRelation(txn, bundle.RelationKeyIsFavorite, exLinks, links)
		if err != nil {
			return err
		}
	}

	var addedLinks, removedLinks []string

	removedLinks, addedLinks = slice.DifferenceRemovedAdded(exLinks, links)
	if len(addedLinks) > 0 {
		for _, k := range pageLinkKeys(id, nil, addedLinks) {
			if err := txn.Put(k, nil); err != nil {
				return err
			}
		}
	}

	if len(removedLinks) > 0 {
		for _, k := range pageLinkKeys(id, nil, removedLinks) {
			if err := txn.Delete(k); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *dsObjectStore) updateObjectLinksAndSnippet(txn ds.Txn, id string, links []string, snippet string) error {
	err := m.updateObjectLinks(txn, id, links)
	if err != nil {
		return err
	}

	if val, err := txn.Get(pagesSnippetBase.ChildString(id)); err == ds.ErrNotFound || string(val) != snippet {
		if err := m.updateSnippet(txn, id, snippet); err != nil {
			return err
		}
	}

	return nil
}

func (m *dsObjectStore) updateObjectDetails(txn ds.Txn, id string, before model.ObjectInfo, details *types.Struct, relations *model.Relations) error {
	objTypes := pbtypes.GetStringList(details, bundle.RelationKeyType.String())

	creatorId := pbtypes.GetString(details, bundle.RelationKeyCreator.String())
	if relations != nil && relations.Relations != nil {
		// intentionally do not pass txn, as this tx may be huge
		if err := m.updateObjectRelations(txn, before.ObjectTypeUrls, objTypes, id, creatorId, before.Relations, relations.Relations); err != nil {
			return err
		}
	}

	if details != nil {
		if err := m.updateDetails(txn, id, &model.ObjectDetails{Details: before.Details}, &model.ObjectDetails{Details: details}); err != nil {
			return err
		}
	}

	return nil
}

// should be called under the mutex
func (m *dsObjectStore) sendUpdatesToSubscriptions(id string, details *types.Struct) {
	detCopy := pbtypes.CopyStruct(details)
	detCopy.Fields[database.RecordIDField] = pb.ToValue(id)
	for i := range m.subscriptions {
		go func(sub database.Subscription) {
			_ = sub.Publish(id, detCopy)
		}(m.subscriptions[i])
	}
}

func (m *dsObjectStore) AddToIndexQueue(id string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	var buf [8]byte
	size := binary.PutVarint(buf[:], time.Now().Unix())
	if err = txn.Put(indexQueueBase.ChildString(id), buf[:size]); err != nil {
		return err
	}
	return txn.Commit()
}

func (m *dsObjectStore) removeFromIndexQueue(id string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if err := txn.Delete(indexQueueBase.ChildString(id)); err != nil {
		return fmt.Errorf("failed to remove id from full text index queue: %s", err.Error())
	}

	return txn.Commit()
}

func (m *dsObjectStore) IndexForEach(f func(id string, tm time.Time) error) error {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	res, err := txn.Query(query.Query{Prefix: indexQueueBase.String()})
	if err != nil {
		return fmt.Errorf("error query txn in datastore: %w", err)
	}
	for entry := range res.Next() {
		id := extractIdFromKey(entry.Key)
		ts, _ := binary.Varint(entry.Value)
		indexErr := f(id, time.Unix(ts, 0))
		if indexErr != nil {
			log.Warnf("can't index '%s'(ts %d): %v", id, ts, indexErr)
			// in case indexation is has failed it's better to remove this document from the index
			// so we will not stuck with this object forever
		}

		err = m.removeFromIndexQueue(id)
		if err != nil {
			// if we have the error here we have nothing to do but retry later
			log.Errorf("failed to remove %s(ts %d) from index, will redo the fulltext index: %v", id, ts, err)
		}
	}

	err = res.Close()
	if err != nil {
		return err
	}

	return nil
}

func (m *dsObjectStore) ListIds() ([]string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return findByPrefix(txn, pagesDetailsBase.String()+"/", 0)
}

func (m *dsObjectStore) UpdateRelationsInSet(setId, objType, creatorId string, relations []*model.Relation) error {
	m.l.Lock()
	defer m.l.Unlock()
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return err
	}
	defer txn.Discard()

	relationsBefore, err := getSetRelations(txn, setId)
	if err != nil {
		return err
	}

	err = m.updateSetRelations(txn, setId, objType, creatorId, relationsBefore, relations)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsObjectStore) storeRelations(txn ds.Txn, relations []*model.Relation) error {
	var relBytes []byte
	var err error
	for _, relation := range relations {
		// do not store bundled relations
		if bundle.HasRelation(relation.Key) {
			continue
		}

		relCopy := pbtypes.CopyRelation(relation)
		relCopy.SelectDict = nil

		relationKey := relationsBase.ChildString(relation.Key)
		relBytes, err = proto.Marshal(relCopy)
		if err != nil {
			return err
		}

		err = txn.Put(relationKey, relBytes)
		if err != nil {
			return err
		}
		_, details := bundle.GetDetailsForRelation(false, relCopy)
		id := pbtypes.GetString(details, "id")
		err = m.updateObjectDetails(txn, id, model.ObjectInfo{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, details, nil)
		if err != nil {
			return err
		}

		err = m.updateObjectLinksAndSnippet(txn, id, nil, relation.Description)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *dsObjectStore) updateSetRelations(txn ds.Txn, setId string, setOf string, creatorId string, relationsBefore []*model.Relation, relations []*model.Relation) error {
	if relations == nil {
		return fmt.Errorf("relationsAfter is nil")
	}

	var updatedRelations []*model.Relation
	var updatedOptions []*model.RelationOption

	for _, relAfter := range relations {
		if relBefore := pbtypes.GetRelation(relationsBefore, relAfter.Key); relBefore == nil || !pbtypes.RelationEqual(relBefore, relAfter) {
			if relAfter.Creator != creatorId && !bundle.HasRelation(relAfter.Key) {
				relAfter.Creator = creatorId
			}
			updatedRelations = append(updatedRelations, relAfter)
			if relAfter.Format == model.RelationFormat_status || relAfter.Format == model.RelationFormat_tag {
				if relBefore == nil {
					updatedOptions = append(updatedOptions, relAfter.SelectDict...)
				} else {
					added, updated, _ := pbtypes.RelationSelectDictDiffOmitScope(relBefore.SelectDict, relAfter.SelectDict)
					updatedOptions = append(updatedOptions, append(added, updated...)...)
				}
			}
		}
	}

	err := m.storeRelations(txn, updatedRelations)
	if err != nil {
		return err
	}

	err = storeOptions(txn, updatedOptions)
	if err != nil {
		return err
	}

	relationKeys := pbtypes.GetRelationKeys(relations)
	detailsBefore, err := getObjectDetails(txn, setId)
	setOfOldSl := pbtypes.GetStringList(detailsBefore.GetDetails(), bundle.RelationKeySetOf.String())
	if setOfOldSl == nil {
		err = localstore.AddIndexWithTxn(indexObjectTypeRelationSetId, txn, &relationObjectType{
			relationKeys: relationKeys,
			objectTypes:  []string{setOf},
		}, setId)
		if err != nil {
			return err
		}
	} else {
		// only one source is supported
		setOfOld := setOfOldSl[0]
		err = localstore.UpdateIndexWithTxn(indexObjectTypeRelationSetId, txn, &relationObjectType{
			relationKeys: pbtypes.GetRelationKeys(relationsBefore),
			objectTypes:  []string{setOfOld},
		}, &relationObjectType{
			relationKeys: relationKeys,
			objectTypes:  []string{setOf},
		}, setId)
		if err != nil {
			return err
		}
	}

	if pbtypes.RelationsEqual(relationsBefore, relations) {
		return nil
	}

	b, err := proto.Marshal(&model.Relations{Relations: relations})
	if err != nil {
		return err
	}

	err = txn.Put(setRelationsBase.ChildString(setId), b)
	if err != nil {
		return err
	}

	return nil
}

func (m *dsObjectStore) updateObjectRelations(txn ds.Txn, objTypesBefore []string, objTypesAfter []string, id, creatorId string, relationsBefore []*model.Relation, relationsAfter []*model.Relation) error {
	if relationsAfter == nil {
		return fmt.Errorf("relationsAfter is nil")
	}
	var err error
	var updatedRelations []*model.Relation
	var updatedOptions []*model.RelationOption

	for _, relAfter := range relationsAfter {
		if relBefore := pbtypes.GetRelation(relationsBefore, relAfter.Key); relBefore == nil || !pbtypes.RelationEqual(relBefore, relAfter) {
			if relAfter.Creator != creatorId && !bundle.HasRelation(relAfter.Key) {
				relAfter.Creator = creatorId
			}
			updatedRelations = append(updatedRelations, relAfter)
			// trigger index relation-option-object indexes updates
			if relBefore == nil {
				err = localstore.AddIndexesWithTxn(m, txn, relAfter, id)
				if err != nil {
					return err
				}
			} else {
				err = localstore.UpdateIndexesWithTxn(m, txn, relBefore, relAfter, id)
				if err != nil {
					return err
				}
			}

			if relAfter.Format == model.RelationFormat_status || relAfter.Format == model.RelationFormat_tag {
				if relBefore == nil {
					updatedOptions = append(updatedOptions, relAfter.SelectDict...)
				} else {
					added, updated, _ := pbtypes.RelationSelectDictDiffOmitScope(relBefore.SelectDict, relAfter.SelectDict)
					updatedOptions = append(updatedOptions, append(added, updated...)...)
				}
			}
		}
	}

	err = m.storeRelations(txn, updatedRelations)
	if err != nil {
		return err
	}

	err = storeOptions(txn, updatedOptions)
	if err != nil {
		return err
	}

	err = localstore.UpdateIndexWithTxn(indexObjectTypeRelationObjectId, txn, &relationObjectType{
		relationKeys: pbtypes.GetRelationKeys(relationsBefore),
		objectTypes:  objTypesBefore,
	}, &relationObjectType{
		relationKeys: pbtypes.GetRelationKeys(relationsAfter),
		objectTypes:  objTypesAfter,
	}, id)
	if err != nil {
		return err
	}

	if pbtypes.RelationsEqual(relationsBefore, relationsAfter) {
		return nil
	}

	b, err := proto.Marshal(&model.Relations{Relations: relationsAfter})
	if err != nil {
		return err
	}

	err = txn.Put(pagesRelationsBase.ChildString(id), b)
	if err != nil {
		return err
	}

	return nil
}

func (m *dsObjectStore) updateSnippet(txn ds.Txn, id string, snippet string) error {
	snippetKey := pagesSnippetBase.ChildString(id)
	return txn.Put(snippetKey, []byte(snippet))
}

func (m *dsObjectStore) updateDetails(txn ds.Txn, id string, oldDetails *model.ObjectDetails, newDetails *model.ObjectDetails) error {
	metrics.ObjectDetailsUpdatedCounter.Inc()
	detailsKey := pagesDetailsBase.ChildString(id)

	b, err := proto.Marshal(newDetails)
	if err != nil {
		return err
	}
	err = txn.Put(detailsKey, b)
	if err != nil {
		return err
	}

	if oldDetails.GetDetails().Equal(newDetails.GetDetails()) {
		return nil
	}

	for k, v := range newDetails.GetDetails().GetFields() {
		// todo: remove null cleanup(should be done when receiving from client)
		if _, isNull := v.GetKind().(*types.Value_NullValue); v == nil || isNull {
			if slice.FindPos(bundle.LocalRelationsKeys, k) > -1 || slice.FindPos(bundle.DerivedRelationsKeys, k) > -1 {
				log.Errorf("updateDetails %s: detail nulled %s: %s", id, k, pbtypes.Sprint(v))
			} else {
				log.Errorf("updateDetails %s: localDetail nulled %s: %s", id, k, pbtypes.Sprint(v))
			}
		}
	}

	diff := pbtypes.StructDiff(oldDetails.GetDetails(), newDetails.GetDetails())
	log.Debugf("updateDetails %s: diff %s", id, pbtypes.Sprint(diff))
	err = localstore.UpdateIndexesWithTxn(m, txn, oldDetails, newDetails, id)
	if err != nil {
		return err
	}

	if newDetails != nil && newDetails.Details.Fields != nil {
		m.sendUpdatesToSubscriptions(id, newDetails.Details)
	}

	return nil
}

func storeOptions(txn ds.Txn, options []*model.RelationOption) error {
	var err error
	for _, opt := range options {
		err = storeOption(txn, opt)
		if err != nil {
			return err
		}
	}
	return nil
}

func storeOption(txn ds.Txn, option *model.RelationOption) error {
	b, err := proto.Marshal(option)
	if err != nil {
		return err
	}

	optionKey := relationsOptionsBase.ChildString(option.Id)
	return txn.Put(optionKey, b)
}

func (m *dsObjectStore) Prefix() string {
	return pagesPrefix
}

func (m *dsObjectStore) Indexes() []localstore.Index {
	return []localstore.Index{indexObjectTypeRelationObjectId, indexObjectTypeRelationSetId, indexRelationOptionObject, indexRelationObject, indexObjectTypeObject}
}

func (m *dsObjectStore) FTSearch() ftsearch.FTSearch {
	return m.fts
}

func (m *dsObjectStore) makeFTSQuery(text string, dsq query.Query) (query.Query, error) {
	if m.fts == nil {
		return dsq, fmt.Errorf("fullText search not configured")
	}
	ids, err := m.fts.Search(text)
	if err != nil {
		return dsq, err
	}
	idsQuery := newIdsFilter(ids)
	dsq.Filters = append([]query.Filter{idsQuery}, dsq.Filters...)
	dsq.Orders = append([]query.Order{idsQuery}, dsq.Orders...)
	return dsq, nil
}

func (m *dsObjectStore) listIdsOfType(txn ds.Txn, ot string) ([]string, error) {
	res, err := localstore.GetKeysByIndexParts(txn, pagesPrefix, indexObjectTypeObject.Name, []string{ot}, "", false, 0)
	if err != nil {
		return nil, err
	}

	return localstore.GetLeavesFromResults(res)
}

func (m *dsObjectStore) listRelationsKeys(txn ds.Txn) ([]string, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return findByPrefix(txn, relationsBase.String()+"/", 0)
}

func getRelation(txn ds.Txn, key string) (*model.Relation, error) {
	br, err := bundle.GetRelation(bundle.RelationKey(key))
	if br != nil {
		return br, nil
	}

	res, err := txn.Get(relationsBase.ChildString(key))
	if err != nil {
		return nil, err
	}

	var rel model.Relation
	if err = proto.Unmarshal(res, &rel); err != nil {
		return nil, fmt.Errorf("failed to unmarshal relation: %s", err.Error())
	}

	return &rel, nil
}

func (m *dsObjectStore) listRelations(txn ds.Txn, limit int) ([]*model.Relation, error) {
	var rels []*model.Relation

	res, err := txn.Query(query.Query{
		Prefix:   relationsBase.String(),
		Limit:    limit,
		KeysOnly: false,
	})
	if err != nil {
		return nil, err
	}

	for r := range res.Next() {
		var rel model.Relation
		if err = proto.Unmarshal(r.Value, &rel); err != nil {
			log.Errorf("listRelations failed to unmarshal relation: %s", err.Error())
			continue
		}
		rels = append(rels, &rel)
	}

	return rels, nil
}

func isObjectBelongToType(txn ds.Txn, id, objType string) (bool, error) {
	objTypeCompact, err := objTypeCompactEncode(objType)
	if err != nil {
		return false, err
	}

	return localstore.HasPrimaryKeyByIndexParts(txn, pagesPrefix, indexObjectTypeObject.Name, []string{objTypeCompact}, "", false, id)
}

/* internal */
// getObjectDetails returns empty(not nil) details when not found in the DS
func getObjectDetails(txn ds.Txn, id string) (*model.ObjectDetails, error) {
	var details model.ObjectDetails
	if val, err := txn.Get(pagesDetailsBase.ChildString(id)); err != nil {
		if err != ds.ErrNotFound {
			return nil, fmt.Errorf("failed to get relations: %w", err)
		}
		details.Details = &types.Struct{Fields: map[string]*types.Value{}}
		// return empty details in case not found
	} else if err := proto.Unmarshal(val, &details); err != nil {
		return nil, fmt.Errorf("failed to unmarshal details: %w", err)
	}
	details.Details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)

	for k, v := range details.GetDetails().GetFields() {
		// todo: remove null cleanup(should be done when receiving from client)
		if _, isNull := v.GetKind().(*types.Value_NullValue); v == nil || isNull {
			delete(details.Details.Fields, k)
		}
	}
	return &details, nil
}

func hasObjectId(txn ds.Txn, id string) (bool, error) {
	if exists, err := txn.Has(pagesDetailsBase.ChildString(id)); err != nil {
		return false, fmt.Errorf("failed to get details: %w", err)
	} else {
		return exists, nil
	}
}

// getSetRelations returns the list of relations last time indexed for the set's dataview
func getSetRelations(txn ds.Txn, id string) ([]*model.Relation, error) {
	var relations model.Relations
	if val, err := txn.Get(setRelationsBase.ChildString(id)); err != nil {
		if err != ds.ErrNotFound {
			return nil, fmt.Errorf("failed to get relations: %w", err)
		}
	} else if err := proto.Unmarshal(val, &relations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal relations: %w", err)
	}

	return relations.GetRelations(), nil
}

// getObjectRelations returns the list of relations last time indexed for the object
func getObjectRelations(txn ds.Txn, id string) ([]*model.Relation, error) {
	var relations model.Relations
	if val, err := txn.Get(pagesRelationsBase.ChildString(id)); err != nil {
		if err != ds.ErrNotFound {
			return nil, fmt.Errorf("failed to get relations: %w", err)
		}
	} else if err := proto.Unmarshal(val, &relations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal relations: %w", err)
	}

	return relations.GetRelations(), nil
}

func getOption(txn ds.Txn, optionId string) (*model.RelationOption, error) {
	var opt model.RelationOption
	if val, err := txn.Get(relationsOptionsBase.ChildString(optionId)); err != nil {
		log.Debugf("getOption %s: not found", optionId)
		if err != ds.ErrNotFound {
			return nil, fmt.Errorf("failed to get option from localstore: %w", err)
		}
	} else if err := proto.Unmarshal(val, &opt); err != nil {
		return nil, fmt.Errorf("failed to unmarshal option: %w", err)
	}

	return &opt, nil
}

func getObjectTypeFromDetails(det *types.Struct) ([]string, error) {
	if !pbtypes.HasField(det, bundle.RelationKeyType.String()) {
		return nil, fmt.Errorf("type not found in details")
	}

	return pbtypes.GetStringList(det, bundle.RelationKeyType.String()), nil
}

func (m *dsObjectStore) getObjectInfo(txn ds.Txn, id string) (*model.ObjectInfo, error) {
	sbt, err := smartblock.SmartBlockTypeFromID(id)
	if err != nil {
		log.With("thread", id).Errorf("failed to extract smartblock type %s", id)
		return nil, ErrNotAnObject
	}
	if sbt == smartblock.SmartBlockTypeArchive {
		return nil, ErrNotAnObject
	}

	var details *types.Struct
	if indexDetails, _ := sbt.Indexable(); !indexDetails {
		if m.sourceService != nil {
			details, err = m.sourceService.GetDetailsFromIdBasedSource(id)
			if err != nil {
				return nil, err
			}
		}
	} else {
		detailsWrapped, err := getObjectDetails(txn, id)
		if err != nil {
			return nil, err
		}
		details = detailsWrapped.GetDetails()
	}

	objectTypes, err := getObjectTypeFromDetails(details)
	if err != nil {
		return nil, err
	}

	relations, err := getObjectRelations(txn, id)
	if err != nil {
		return nil, err
	}

	var snippet string
	if val, err := txn.Get(pagesSnippetBase.ChildString(id)); err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get snippet: %w", err)
	} else {
		snippet = string(val)
	}

	// omit decoding page state
	hasInbound, err := hasInboundLinks(txn, id)
	if err != nil {
		return nil, err
	}

	return &model.ObjectInfo{
		Id:              id,
		ObjectType:      sbt.ToProto(),
		Details:         details,
		Relations:       relations,
		Snippet:         snippet,
		HasInboundLinks: hasInbound,
		ObjectTypeUrls:  objectTypes,
	}, nil
}

func (m *dsObjectStore) getObjectsInfo(txn ds.Txn, ids []string) ([]*model.ObjectInfo, error) {
	var objects []*model.ObjectInfo
	for _, id := range ids {
		info, err := m.getObjectInfo(txn, id)
		if err != nil {
			if strings.HasSuffix(err.Error(), "key not found") || err == ErrNotAnObject {
				continue
			}
			return nil, err
		}
		objects = append(objects, info)
	}

	return objects, nil
}

func hasInboundLinks(txn ds.Txn, id string) (bool, error) {
	inboundResults, err := txn.Query(query.Query{
		Prefix:   pagesInboundLinksBase.String() + "/" + id + "/",
		Limit:    1, // we only need to know if there is at least 1 inbound link
		KeysOnly: true,
	})
	if err != nil {
		return false, err
	}

	// max is 1
	inboundLinks, err := localstore.CountAllKeysFromResults(inboundResults)
	return inboundLinks > 0, err
}

// Find to which IDs specified one has outbound links.
func findOutboundLinks(txn ds.Txn, id string) ([]string, error) {
	return findByPrefix(txn, pagesOutboundLinksBase.String()+"/"+id+"/", 0)
}

// Find from which IDs specified one has inbound links.
func findInboundLinks(txn ds.Txn, id string) ([]string, error) {
	return findByPrefix(txn, pagesInboundLinksBase.String()+"/"+id+"/", 0)
}

func findByPrefix(txn ds.Txn, prefix string, limit int) ([]string, error) {
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

func removeByPrefix(txn ds.Txn, prefix string) (int, error) {
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
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func pageLinkKeys(id string, in []string, out []string) []ds.Key {
	var keys = make([]ds.Key, 0, len(in)+len(out))

	// links incoming into specified node id
	for _, from := range in {
		keys = append(keys, inboundLinkKey(from, id), outgoingLinkKey(from, id))
	}

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

func newIdsFilter(ids []string) idsFilter {
	f := make(idsFilter)
	for i, id := range ids {
		f[id] = i
	}
	return f
}

type idsFilter map[string]int

func (f idsFilter) Filter(e query.Entry) bool {
	_, ok := f[extractIdFromKey(e.Key)]
	return ok
}

func (f idsFilter) Compare(a, b query.Entry) int {
	aIndex := f[extractIdFromKey(a.Key)]
	bIndex := f[extractIdFromKey(b.Key)]
	if aIndex == bIndex {
		return 0
	} else if aIndex < bIndex {
		return -1
	} else {
		return 1
	}
}

func extractIdFromKey(key string) (id string) {
	i := strings.LastIndexByte(key, '/')
	if i == -1 || len(key)-1 == i {
		return
	}
	return key[i+1:]
}

// temp func until we move to the proper ids
func objTypeCompactEncode(objType string) (string, error) {
	if strings.HasPrefix(objType, addr.BundledObjectTypeURLPrefix) {
		return objType, nil
	}
	if strings.HasPrefix(objType, "ba") {
		return objType, nil
	}
	return "", fmt.Errorf("invalid objType")
}

func GetObjectType(store ObjectStore, url string) (*model.ObjectType, error) {
	objectType := &model.ObjectType{}
	if strings.HasPrefix(url, addr.BundledObjectTypeURLPrefix) {
		var err error
		objectType, err = bundle.GetTypeByUrl(url)
		if err != nil {
			if err == bundle.ErrNotFound {
				return nil, fmt.Errorf("unknown object type")
			}
			return nil, err
		}
		return objectType, nil
	} else if !strings.HasPrefix(url, "b") {
		return nil, fmt.Errorf("incorrect object type URL format")
	}

	ois, err := store.GetByIDs(url)
	if err != nil {
		return nil, err
	}
	if len(ois) == 0 {
		return nil, fmt.Errorf("object type not found in the index")
	}

	details := ois[0].Details
	//relationKeys := ois[0].RelationKeys
	for _, relId := range pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()) {
		rk, err := pbtypes.RelationIdToKey(relId)
		if err != nil {
			log.Errorf("GetObjectType failed to get relation key from id: %s (%s)", err.Error(), relId)
			continue
		}

		rel, err := store.GetRelation(rk)
		if err != nil {
			log.Errorf("GetObjectType failed to get relation key from id: %s (%s)", err.Error(), relId)
			continue
		}

		relCopy := pbtypes.CopyRelation(rel)
		relCopy.Scope = model.Relation_type

		objectType.Relations = append(objectType.Relations, relCopy)
	}

	objectType.Url = url
	if details != nil && details.Fields != nil {
		if v, ok := details.Fields[bundle.RelationKeyName.String()]; ok {
			objectType.Name = v.GetStringValue()
		}
		if v, ok := details.Fields[bundle.RelationKeyRecommendedLayout.String()]; ok {
			objectType.Layout = model.ObjectTypeLayout(int(v.GetNumberValue()))
		}
		if v, ok := details.Fields[bundle.RelationKeyIconEmoji.String()]; ok {
			objectType.IconEmoji = v.GetStringValue()
		}
	}

	objectType.IsArchived = pbtypes.GetBool(details, bundle.RelationKeyIsArchived.String())
	// we use Page for all custom object types
	objectType.Types = []model.SmartBlockType{model.SmartBlockType_Page}
	return objectType, err
}
