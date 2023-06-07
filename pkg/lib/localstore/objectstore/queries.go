package objectstore

import (
	"fmt"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"

	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// TODO: objstore: no one uses total
func (m *dsObjectStore) Query(sch schema.Schema, q database.Query) (records []database.Record, total int, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	dsq, err := m.buildQuery(sch, q)
	if err != nil {
		return nil, 0, fmt.Errorf("build query: %w", err)
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

		key := ds.NewKey(rec.Key)
		keyList := key.List()
		id := keyList[len(keyList)-1]

		var details *model.ObjectDetails
		details, err = unmarshalDetails(id, rec.Value)
		if err != nil {
			total--
			log.Errorf("failed to unmarshal: %s", err.Error())
			continue
		}
		results = append(results, database.Record{Details: details.Details})
	}

	return results, total, nil
}

func (m *dsObjectStore) buildQuery(sch schema.Schema, q database.Query) (query.Query, error) {
	dsq, err := q.DSQuery(sch)
	if err != nil {
		return query.Query{}, fmt.Errorf("init datastore query: %w", err)
	}
	dsq.Offset = 0
	dsq.Limit = 0
	dsq.Prefix = pagesDetailsBase.String() + "/"
	discardSystemObjects := newSmartblockTypesFilter(m.sbtProvider, true, []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeHome,
	})
	dsq.Filters = append([]query.Filter{discardSystemObjects}, dsq.Filters...)

	if q.FullText != "" {
		dsq, err = m.makeFTSQuery(q.FullText, dsq)
		if err != nil {
			return query.Query{}, fmt.Errorf("append full text search query: %w", err)
		}
	}
	return dsq, nil
}

func (m *dsObjectStore) QueryRaw(f *database.Filters) (records []database.Record, err error) {
	dsq := query.Query{
		Filters: []query.Filter{f},
	}
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	dsq.Prefix = pagesDetailsBase.String() + "/"

	res, err := txn.Query(dsq)
	if err != nil {
		return nil, fmt.Errorf("error when querying ds: %w", err)
	}

	for rec := range res.Next() {
		key := ds.NewKey(rec.Key)
		keyList := key.List()
		id := keyList[len(keyList)-1]

		var details *model.ObjectDetails
		details, err = unmarshalDetails(id, rec.Value)
		if err != nil {
			log.Errorf("failed to unmarshal: %s", err.Error())
			continue
		}
		records = append(records, database.Record{Details: details.Details})
	}
	return
}

// TODO: objstore: no one uses total
func (m *dsObjectStore) QueryObjectIds(q database.Query, smartBlockTypes []smartblock.SmartBlockType) (ids []string, total int, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	dsq, err := m.buildQuery(nil, q)
	if err != nil {
		return
	}
	if len(smartBlockTypes) > 0 {
		dsq.Filters = append([]query.Filter{newSmartblockTypesFilter(m.sbtProvider, false, smartBlockTypes)}, dsq.Filters...)
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

		if q.Limit > 0 && len(ids) >= q.Limit {
			continue
		}

		key := ds.NewKey(rec.Key)
		keyList := key.List()
		id := keyList[len(keyList)-1]
		ids = append(ids, id)
	}
	return ids, total, nil
}

func (m *dsObjectStore) QueryById(ids []string) (records []database.Record, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	for _, id := range ids {
		if sbt, err := m.sbtProvider.Type(id); err == nil {
			if indexDetails, _ := sbt.Indexable(); !indexDetails && m.sourceService != nil {
				details, err := m.sourceService.DetailsFromIdBasedSource(id)
				if err != nil {
					log.Errorf("QueryByIds failed to GetDetailsFromIdBasedSource id: %s", id)
					continue
				}
				details.Fields[database.RecordIDField] = pbtypes.ToValue(id)
				records = append(records, database.Record{Details: details})
				continue
			}
		}
		v, err := txn.Get(pagesDetailsBase.ChildString(id))
		if err != nil {
			log.Infof("QueryByIds failed to find id: %s", id)
			continue
		}

		var details *model.ObjectDetails
		details, err = unmarshalDetails(id, v)
		if err != nil {
			log.Errorf("QueryByIds failed to unmarshal id: %s", id)
			continue
		}
		records = append(records, database.Record{Details: details.Details})
	}

	return
}

func (m *dsObjectStore) QueryByIdAndSubscribeForChanges(ids []string, sub database.Subscription) (records []database.Record, close func(), err error) {
	m.l.Lock()
	defer m.l.Unlock()

	if sub == nil {
		err = fmt.Errorf("subscription func is nil")
		return
	}
	sub.Subscribe(ids)
	records, err = m.QueryById(ids)
	if err != nil {
		// can mean only the datastore is already closed, so we can resign and return
		log.Errorf("QueryByIdAndSubscribeForChanges failed to query ids: %v", err)
		return nil, nil, err
	}

	close = func() {
		m.closeAndRemoveSubscription(sub)
	}

	m.addSubscriptionIfNotExists(sub)

	return
}
