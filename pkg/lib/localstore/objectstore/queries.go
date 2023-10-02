package objectstore

import (
	"fmt"
	"sort"

	"github.com/dgraph-io/badger/v3"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *dsObjectStore) Query(sch schema.Schema, q database.Query) ([]database.Record, int, error) {
	filters, err := s.buildQuery(sch, q)
	if err != nil {
		return nil, 0, fmt.Errorf("build query: %w", err)
	}
	recs, err := s.QueryRaw(filters, q.Limit, q.Offset)
	return recs, 0, err
}

func (s *dsObjectStore) QueryRaw(filters *database.Filters, limit int, offset int) ([]database.Record, error) {
	if filters == nil || filters.FilterObj == nil {
		return nil, fmt.Errorf("filter cannot be nil or unitialized")
	}

	var records []database.Record
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
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

func (s *dsObjectStore) buildQuery(sch schema.Schema, q database.Query) (*database.Filters, error) {
	filters, err := database.NewFilters(q, sch, s)
	if err != nil {
		return nil, fmt.Errorf("new filters: %w", err)
	}
	discardSystemObjects := newSmartblockTypesFilter(s.sbtProvider, true, []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeHome,
	})
	filters.FilterObj = filter.AndFilters{filters.FilterObj, discardSystemObjects}

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
	ids, err := s.fts.Search(text)
	if err != nil {
		return filters, err
	}
	idsQuery := newIdsFilter(ids)
	filters.FilterObj = filter.AndFilters{filters.FilterObj, idsQuery}
	filters.Order = filter.SetOrder(append([]filter.Order{idsQuery}, filters.Order))
	return filters, nil
}

// TODO: objstore: no one uses total
func (s *dsObjectStore) QueryObjectIDs(q database.Query, smartBlockTypes []smartblock.SmartBlockType) (ids []string, total int, err error) {
	filters, err := s.buildQuery(nil, q)
	if err != nil {
		return nil, 0, fmt.Errorf("build query: %w", err)
	}
	if len(smartBlockTypes) > 0 {
		filters.FilterObj = filter.AndFilters{newSmartblockTypesFilter(s.sbtProvider, false, smartBlockTypes), filters.FilterObj}
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
			if sbt, err := s.sbtProvider.Type(id); err == nil {
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
