package localstore

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
)

var (
	// ObjectInfo is stored in db key pattern:
	pagesPrefix        = "pages"
	pagesDetailsBase   = ds.NewKey("/" + pagesPrefix + "/details")
	pagesRelationsBase = ds.NewKey("/" + pagesPrefix + "/relations")

	pagesSnippetBase       = ds.NewKey("/" + pagesPrefix + "/snippet")
	pagesInboundLinksBase  = ds.NewKey("/" + pagesPrefix + "/inbound")
	pagesOutboundLinksBase = ds.NewKey("/" + pagesPrefix + "/outbound")
	indexQueueBase         = ds.NewKey("/" + pagesPrefix + "/index")

	_ ObjectStore = (*dsObjectStore)(nil)
)

var ErrNotAPage = fmt.Errorf("not a page")

var filterNotSystemObjects = &filterObjectTypes{
	objectTypes: []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeHome,
		smartblock.SmartBlockTypeObjectType,
	},
}

type filterObjectTypes struct {
	objectTypes []smartblock.SmartBlockType
}

func (m *filterObjectTypes) Filter(e query.Entry) bool {
	keyParts := strings.Split(e.Key, "/")
	id := keyParts[len(keyParts)-1]

	t, err := smartblock.SmartBlockTypeFromID(id)
	if err != nil {
		log.Errorf("failed to detect smartblock type for %s: %s", id, err.Error())
		return false
	}

	for _, ot := range m.objectTypes {
		if t == ot {
			return true
		}
	}
	return false
}

func NewObjectStore(ds ds.TxnDatastore, fts ftsearch.FTSearch) ObjectStore {
	return &dsObjectStore{ds: ds, fts: fts}
}

type dsObjectStore struct {
	// underlying storage
	ds  ds.TxnDatastore
	fts ftsearch.FTSearch

	// serializing page updates
	l sync.Mutex

	subscriptions    []database.Subscription
	depSubscriptions []database.Subscription
}

func (m *dsObjectStore) QueryAndSubscribeForChanges(schema *schema.Schema, q database.Query, sub database.Subscription) (records []database.Record, close func(), total int, err error) {
	m.l.Lock()
	defer m.l.Unlock()

	records, total, err = m.Query(schema, q)

	var ids []string
	for _, record := range records {
		ids = append(ids, pbtypes.GetString(record.Details, "id"))
	}

	sub.Subscribe(ids)
	m.addSubscriptionIfNotExists(sub)
	close = func() {
		m.closeAndRemoveSubscription(sub)
	}

	return
}

// unsafe, use under mutex
func (m *dsObjectStore) addSubscriptionIfNotExists(sub database.Subscription) {
	for _, s := range m.subscriptions {
		if s == sub {
			return
		}
	}
	log.Debugf("objStore subscription add %p", sub)
	m.subscriptions = append(m.subscriptions, sub)
}

func (m *dsObjectStore) closeAndRemoveSubscription(sub database.Subscription) {
	m.l.Lock()
	defer m.l.Unlock()
	sub.Close()

	for i, s := range m.subscriptions {
		if s == sub {
			log.Debugf("objStore subscription remove %p", s)
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
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	dsq := q.DSQuery(sch)
	dsq.Offset = 0
	dsq.Limit = 0
	dsq.Prefix = pagesDetailsBase.String() + "/"
	dsq.Filters = append([]query.Filter{filterNotSystemObjects}, dsq.Filters...)
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

func (m *dsObjectStore) QueryObjectInfo(q database.Query, objectTypes []smartblock.SmartBlockType) (results []*model.ObjectInfo, total int, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	dsq := q.DSQuery(nil)
	dsq.Offset = 0
	dsq.Limit = 0
	dsq.Prefix = pagesDetailsBase.String() + "/"
	if len(objectTypes) > 0 {
		dsq.Filters = append([]query.Filter{&filterObjectTypes{objectTypes}}, dsq.Filters...)
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
		oi, err := getObjectInfo(txn, id)
		if err != nil {
			return nil, 0, err
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

func (m *dsObjectStore) AggregateRelations(sch *schema.Schema) (relations []*pbrelation.Relation, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	q := database.Query{}
	dsq := q.DSQuery(sch)
	dsq.Offset = 0
	dsq.Limit = 0
	dsq.Prefix = pagesRelationsBase.String() + "/"
	dsq.Filters = append([]query.Filter{&filterObjectTypes{}}, dsq.Filters...)
	res, err := txn.Query(dsq)
	if err != nil {
		return nil, fmt.Errorf("error when querying ds: %w", err)
	}

	var relationsKeysMaps map[string]struct{}

	for rec := range res.Next() {
		var rels pbrelation.Relations
		if err = proto.Unmarshal(rec.Value, &rels); err != nil {
			log.Errorf("failed to unmarshal: %s", err.Error())
			continue
		}
		for i, rel := range rels.Relations {
			if _, exists := relationsKeysMaps[rel.Key]; exists {
				// todo: aggregate select dictionary?
				continue
			}

			relationsKeysMaps[rel.Key] = struct{}{}
			relations = append(relations, rels.Relations[i])
		}
	}

	return relations, nil
}

/*func (m *dsObjectStore) AddObject(page *model.ObjectInfoWithOutboundLinksIDs) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	detailsKey := pagesDetailsBase.ChildString(page.Id)
	relationsKey := pagesRelationsBase.ChildString(page.Id)
	snippetKey := pagesSnippetBase.ChildString(page.Id)

	if exists, err := txn.Has(detailsKey); err != nil {
		return err
	} else if exists {
		return ErrDuplicateKey
	}

	page.Info.Details.Fields["type"] = pbtypes.StringList(page.Info.ObjectTypeUrls)
	b, err := proto.Marshal(page.Info.Details)
	if err != nil {
		return err
	}
	if err = txn.Put(detailsKey, b); err != nil {
		return err
	}

	b, err = proto.Marshal(page.Info.Relations)
	if err != nil {
		return err
	}

	if err = txn.Put(relationsKey, b); err != nil {
		return err
	}

	for _, key := range pageLinkKeys(page.Id, nil, page.OutboundLinks) {
		if err = txn.Put(key, nil); err != nil {
			return err
		}
	}

	if err = txn.Put(snippetKey, []byte(page.Info.Snippet)); err != nil {
		return err
	}

	return txn.Commit()
}*/

func (m *dsObjectStore) DeleteObject(id string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	for _, k := range []ds.Key{
		pagesDetailsBase.ChildString(id),
		pagesSnippetBase.ChildString(id),
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

func (m *dsObjectStore) GetWithLinksInfoByID(id string) (*model.ObjectInfoWithLinks, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	pages, err := getPagesInfo(txn, []string{id})
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

	inbound, err := getPagesInfo(txn, inboundIds)
	if err != nil {
		return nil, err
	}

	outbound, err := getPagesInfo(txn, outboundsIds)
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

func (m *dsObjectStore) GetWithOutboundLinksInfoById(id string) (*model.ObjectInfoWithOutboundLinks, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	pages, err := getPagesInfo(txn, []string{id})
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

	outbound, err := getPagesInfo(txn, outboundsIds)
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

	return getDetails(txn, id)
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

	return getPagesInfo(txn, ids)
}

func (m *dsObjectStore) GetByIDs(ids ...string) ([]*model.ObjectInfo, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return getPagesInfo(txn, ids)
}

func diffSlices(a, b []string) (removed []string, added []string) {
	var amap = map[string]struct{}{}
	var bmap = map[string]struct{}{}

	for _, item := range a {
		amap[item] = struct{}{}
	}

	for _, item := range b {
		if _, exists := amap[item]; !exists {
			added = append(added, item)
		}
		bmap[item] = struct{}{}
	}

	for _, item := range a {
		if _, exists := bmap[item]; !exists {
			removed = append(removed, item)
		}
	}
	return
}

func (m *dsObjectStore) UpdateObject(id string, details *types.Struct, relations *pbrelation.Relations, links []string, snippet string) error {
	m.l.Lock()
	defer m.l.Unlock()

	log.Errorf("UpdateObject %s: %v", id, details)

	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if details != nil || len(snippet) > 0 {
		exInfo, _ := getObjectInfo(txn, id)
		if exInfo != nil {
			if exInfo.Details.Equal(details) {
				// skip updating details
				details = nil
			}

			if exInfo.Snippet == snippet {
				// skip updating snippet
				snippet = ""
			}
		}
	}

	var addedLinks, removedLinks []string

	if links != nil {
		exLinks, _ := findOutboundLinks(txn, id)
		removedLinks, addedLinks = diffSlices(exLinks, links)
	}

	if details != nil {
		if err = m.updateDetails(txn, id, &model.ObjectDetails{Details: details}); err != nil {
			return err
		}
	}

	if relations != nil {
		if err = m.updateRelations(txn, id, relations); err != nil {
			return err
		}
	}

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

	if len(snippet) > 0 {
		if err = m.updateSnippet(txn, id, snippet); err != nil {
			return err
		}
	}

	err = txn.Commit()
	if err != nil {
		return err
	}

	return nil
}

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

func (m *dsObjectStore) IndexForEach(f func(id string, tm time.Time) error) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	res, err := txn.Query(query.Query{Prefix: indexQueueBase.String()})
	if err != nil {
		return fmt.Errorf("error query txn in datastore: %w", err)
	}
	defer res.Close()
	for entry := range res.Next() {
		id := extractIdFromKey(entry.Key)
		ts, _ := binary.Varint(entry.Value)
		if indexErr := f(id, time.Unix(ts, 0)); indexErr != nil {
			log.Warnf("can't index '%s': %v", id, indexErr)
			continue
		}
		if err := txn.Delete(indexQueueBase.ChildString(id)); err != nil {
			return err
		}
	}
	return txn.Commit()
}

func (m *dsObjectStore) updateDetails(txn ds.Txn, id string, details *model.ObjectDetails) error {
	detailsKey := pagesDetailsBase.ChildString(id)
	b, err := proto.Marshal(details)
	if err != nil {
		return err
	}

	err = txn.Put(detailsKey, b)
	if err != nil {
		return err
	}

	if details.Details != nil && details.Details.Fields != nil {
		m.sendUpdatesToSubscriptions(id, details.Details)
	}

	return nil
}

func (m *dsObjectStore) updateRelations(txn ds.Txn, id string, relations *pbrelation.Relations) error {
	relationsKey := pagesRelationsBase.ChildString(id)
	b, err := proto.Marshal(relations)
	if err != nil {
		return err
	}

	return txn.Put(relationsKey, b)
}

func (m *dsObjectStore) updateSnippet(txn ds.Txn, id string, snippet string) error {
	snippetKey := pagesSnippetBase.ChildString(id)
	return txn.Put(snippetKey, []byte(snippet))
}

func (m *dsObjectStore) Prefix() string {
	return pagesPrefix
}

func (m *dsObjectStore) Indexes() []Index {
	return nil
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

func (m *dsObjectStore) Close() {
	if m.fts != nil {
		m.fts.Close()
	}
}

/* internal */

func getDetails(txn ds.Txn, id string) (*model.ObjectDetails, error) {
	var details model.ObjectDetails
	if val, err := txn.Get(pagesDetailsBase.ChildString(id)); err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get details: %w", err)
	} else if err := proto.Unmarshal(val, &details); err != nil {
		return nil, err
	}

	return &details, nil
}

func getObjectInfo(txn ds.Txn, id string) (*model.ObjectInfo, error) {
	sbt, err := smartblock.SmartBlockTypeFromID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to extract smartblock type: %w", err)
	}
	if sbt == smartblock.SmartBlockTypeArchive {
		return nil, ErrNotAPage
	}

	var details model.ObjectDetails
	if val, err := txn.Get(pagesDetailsBase.ChildString(id)); err != nil {
		return nil, fmt.Errorf("failed to get details: %w", err)
	} else if err := proto.Unmarshal(val, &details); err != nil {
		return nil, fmt.Errorf("failed to unmarshal details: %w", err)
	}

	var relations pbrelation.Relations
	if val, err := txn.Get(pagesRelationsBase.ChildString(id)); err != nil {
		if err != ds.ErrNotFound {
			return nil, fmt.Errorf("failed to get relations: %w", err)
		}
	} else if err := proto.Unmarshal(val, &relations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal relations: %w", err)
	}

	if details.Details == nil || details.Details.Fields == nil {
		details.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	details.Details.Fields["id"] = pbtypes.String(id)

	var objectTypes []string
	// remove hardcoded type
	// todo: maybe we should move it to a separate key?
	if details.Details != nil && details.Details.Fields != nil && details.Details.Fields["type"] != nil {
		vals := details.Details.Fields["type"].GetListValue()
		for _, val := range vals.Values {
			objectTypes = append(objectTypes, val.GetStringValue())
		}
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
		Details:         details.Details,
		Relations:       &relations,
		Snippet:         snippet,
		HasInboundLinks: hasInbound,
		ObjectTypeUrls:  objectTypes,
	}, nil
}

func getPagesInfo(txn ds.Txn, ids []string) ([]*model.ObjectInfo, error) {
	var pages []*model.ObjectInfo
	for _, id := range ids {
		info, err := getObjectInfo(txn, id)
		if err != nil {
			if strings.HasSuffix(err.Error(), "key not found") || err == ErrNotAPage {
				continue
			}
			return nil, err
		}
		pages = append(pages, info)
	}

	return pages, nil
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
	inboundLinks, err := CountAllKeysFromResults(inboundResults)
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

	return GetLeavesFromResults(results)
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
