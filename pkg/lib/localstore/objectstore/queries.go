package objectstore

import (
	"fmt"
	"sort"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/dgraph-io/badger/v4"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *dsObjectStore) Query(q database.Query) ([]database.Record, error) {
	filters, err := s.buildQuery(q)
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}
	recs, err := s.QueryRaw(filters, q.Limit, q.Offset)
	return recs, err
}

func (s *dsObjectStore) QueryRaw(filters *database.Filters, limit int, offset int) ([]database.Record, error) {
	if filters == nil || filters.FilterObj == nil {
		return nil, fmt.Errorf("filter cannot be nil or unitialized")
	}

	var records []database.Record
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = pagesDetailsBase.Bytes()
		iterator := txn.NewIterator(opts)
		defer iterator.Close()

		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			it := iterator.Item()
			details, err := s.extractDetailsFromItem(it)
			if err != nil {
				return err
			}
			rec := database.Record{Details: details.Details}
			// todo: pass the inner block/relation to the result

			if filters.FilterObj != nil && filters.FilterObj.FilterObject(rec) {
				records = append(records, rec)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if offset >= len(records) {
		return nil, nil
	}
	if filters.Order != nil {
		sort.Slice(records, func(i, j int) bool {
			return filters.Order.Compare(records[i], records[j]) == -1
		})
	}
	if limit > 0 {
		upperBound := offset + limit
		if upperBound > len(records) {
			upperBound = len(records)
		}
		return records[offset:upperBound], nil
	}
	return records[offset:], nil
}

func (s *dsObjectStore) buildQuery(q database.Query) (*database.Filters, error) {
	filters, err := database.NewFilters(q, s)
	if err != nil {
		return nil, fmt.Errorf("new filters: %w", err)
	}

	if q.FullText != "" {
		filters, err = s.makeFTSQuery(q.FullText, filters)
		if err != nil {
			return nil, fmt.Errorf("append full text search query: %w", err)
		}
	}
	return filters, nil
}

func (s *dsObjectStore) makeFTSQuery(text string, filters *database.Filters) (*database.Filters, error) {
	if s.fts == nil {
		return filters, fmt.Errorf("fullText search not configured")
	}
	results, err := s.fts.Search(getSpaceIDFromFilter(filters.FilterObj), text)
	if err != nil {
		return filters, fmt.Errorf("fullText search: %w", err)
	}

	var resultsByObjectId = make(map[string][]*search.DocumentMatch)
	for _, result := range results {
		path, err := domain.NewFromPath(result.ID)
		if err != nil {
			return filters, fmt.Errorf("fullText search: %w", err)
		}
		if _, ok := resultsByObjectId[path.ObjectId]; !ok {
			resultsByObjectId[path.ObjectId] = make([]*search.DocumentMatch, 0, 1)
		}

		resultsByObjectId[path.ObjectId] = append(resultsByObjectId[path.ObjectId], result)
	}
	for objectId := range resultsByObjectId {
		sort.Slice(resultsByObjectId[objectId], func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
	}

	// select only the best block/relation result for each object
	var objectResults = make([]*search.DocumentMatch, 0, len(resultsByObjectId))
	for _, objectPerBlockResults := range resultsByObjectId {
		if len(objectPerBlockResults) == 0 {
			continue
		}
		objectResults = append(objectResults, objectPerBlockResults[0])
	}

	sort.Slice(objectResults, func(i, j int) bool {
		return objectResults[i].Score > objectResults[j].Score
	})

	var objectIds = make([]string, 0, len(objectResults))
	for _, result := range objectResults {
		path, err := domain.NewFromPath(result.ID)
		if err != nil {
			return filters, fmt.Errorf("fullText search: %w", err)
		}
		objectIds = append(objectIds, path.ObjectId)
	}

	idsQuery := newIdsFilter(objectIds)
	filters.FilterObj = database.FiltersAnd{filters.FilterObj, idsQuery}
	filters.Order = database.SetOrder(append([]database.Order{idsQuery}, filters.Order))
	return filters, nil
}

func getSpaceIDFromFilter(fltr database.Filter) (spaceID string) {
	switch f := fltr.(type) {
	case database.FilterEq:
		if f.Key == bundle.RelationKeySpaceId.String() {
			return f.Value.GetStringValue()
		}
	case database.FiltersAnd:
		spaceID = iterateOverAndFilters(f)
	}
	return spaceID
}

func iterateOverAndFilters(fs []database.Filter) (spaceID string) {
	for _, f := range fs {
		if spaceID = getSpaceIDFromFilter(f); spaceID != "" {
			return spaceID
		}
	}
	return ""
}

// TODO: objstore: no one uses total
func (s *dsObjectStore) QueryObjectIDs(q database.Query) (ids []string, total int, err error) {
	filters, err := s.buildQuery(q)
	if err != nil {
		return nil, 0, fmt.Errorf("build query: %w", err)
	}
	recs, err := s.QueryRaw(filters, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query raw: %w", err)
	}
	ids = make([]string, 0, len(recs))
	for _, rec := range recs {
		ids = append(ids, pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()))
	}
	return ids, 0, nil
}

func (s *dsObjectStore) QueryByID(ids []string) (records []database.Record, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			// Don't use spaceID because expected objects are virtual
			if sbt, err := typeprovider.SmartblockTypeFromID(id); err == nil {
				if indexDetails, _ := sbt.Indexable(); !indexDetails && s.sourceService != nil {
					details, err := s.sourceService.DetailsFromIdBasedSource(id)
					if err != nil {
						log.Errorf("QueryByIds failed to GetDetailsFromIdBasedSource id: %s", id)
						continue
					}
					details.Fields[database.RecordIDField] = pbtypes.ToValue(id)
					records = append(records, database.Record{Details: details})
					continue
				}
			}
			it, err := txn.Get(pagesDetailsBase.ChildString(id).Bytes())
			if err != nil {
				log.Infof("QueryByIds failed to find id: %s", id)
				continue
			}

			details, err := s.extractDetailsFromItem(it)
			if err != nil {
				log.Errorf("QueryByIds failed to extract details: %s", id)
				continue
			}
			records = append(records, database.Record{Details: details.Details})
		}
		return nil
	})
	return
}

func (s *dsObjectStore) QueryByIDAndSubscribeForChanges(ids []string, sub database.Subscription) (records []database.Record, closeFunc func(), err error) {
	s.Lock()
	defer s.Unlock()

	if sub == nil {
		err = fmt.Errorf("subscription func is nil")
		return
	}
	sub.Subscribe(ids)
	records, err = s.QueryByID(ids)
	if err != nil {
		// can mean only the datastore is already closed, so we can resign and return
		log.Errorf("QueryByIDAndSubscribeForChanges failed to query ids: %v", err)
		return nil, nil, err
	}

	closeFunc = func() {
		s.closeAndRemoveSubscription(sub)
	}

	s.addSubscriptionIfNotExists(sub)
	return
}
