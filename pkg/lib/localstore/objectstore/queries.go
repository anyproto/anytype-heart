package objectstore

import (
	"fmt"
	"math"
	"sort"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	jsonFormatter "github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch/jsonhighlighter/json"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	text2 "github.com/anyproto/anytype-heart/util/text"
)

const (
	// minFulltextScore trim fulltext results with score lower than this value in case there are no highlight ranges available
	minFulltextScore = 0.02
)

func (s *dsObjectStore) Query(q database.Query) ([]database.Record, error) {
	recs, err := s.performQuery(q)
	return recs, err
}

// getObjectsWithObjectInRelation returns objects that have a relation with the given object in the value, while also matching the given filters
func (s *dsObjectStore) getObjectsWithObjectInRelation(relationKey, objectId string, limit int, params database.Filters) ([]database.Record, error) {
	return s.queryRaw(func(g *types.Struct) bool {
		listValue := pbtypes.StringList([]string{objectId})
		optionFilter := database.FilterAllIn{relationKey, listValue.GetListValue()}
		if !optionFilter.FilterObject(g) {
			return false
		}
		return params.FilterObj.FilterObject(g)
	}, params.Order, limit, 0)
}

func (s *dsObjectStore) getInjectedResults(details *types.Struct, score float64, path domain.ObjectPath, maxLength int, params database.Filters) []database.Record {
	var injectedResults []database.Record
	id := pbtypes.GetString(details, bundle.RelationKeyId.String())
	if path.RelationKey != bundle.RelationKeyName.String() {
		// inject only in case we match the name
		return nil
	}
	var (
		relationKey string
		err         error
	)

	isDeleted := pbtypes.GetBool(details, bundle.RelationKeyIsDeleted.String())
	isArchived := pbtypes.GetBool(details, bundle.RelationKeyIsArchived.String())
	if isDeleted || isArchived {
		return nil
	}

	switch model.ObjectTypeLayout(pbtypes.GetInt64(details, bundle.RelationKeyLayout.String())) {
	case model.ObjectType_relationOption:
		relationKey = pbtypes.GetString(details, bundle.RelationKeyRelationKey.String())
	case model.ObjectType_objectType:
		relationKey = bundle.RelationKeyType.String()
	default:
		return nil
	}
	recs, err := s.getObjectsWithObjectInRelation(relationKey, id, maxLength, params)
	if err != nil {
		log.Errorf("getInjectedResults failed to get objects with object in relation: %v", err)
		return nil
	}

	for _, rec := range recs {
		metaInj := model.SearchMeta{
			RelationKey:     relationKey,
			RelationDetails: pbtypes.StructFilterKeys(details, []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyType.String(), bundle.RelationKeyLayout.String(), bundle.RelationKeyRelationOptionColor.String()}),
		}

		detailsCopy := pbtypes.CopyStruct(rec.Details, false)
		// set the same score as original object
		detailsCopy.Fields[database.RecordScoreField] = pbtypes.Float64(score)
		injectedResults = append(injectedResults, database.Record{
			Details: detailsCopy,
			Meta:    metaInj,
		})

		if len(injectedResults) == maxLength {
			break
		}
	}

	return injectedResults
}

func (s *dsObjectStore) queryRaw(filter func(g *types.Struct) bool, order database.Order, limit int, offset int) ([]database.Record, error) {
	var (
		records []database.Record
		err     error
	)

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("badger iterator panic: %v", r)
		}
	}()

	err = s.db.View(func(txn *badger.Txn) error {
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

			if filter == nil || filter(details.Details) {
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
	if order != nil {
		sort.Slice(records, func(i, j int) bool {
			return order.Compare(records[i].Details, records[j].Details) == -1
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

func (s *dsObjectStore) QueryRaw(filters *database.Filters, limit int, offset int) ([]database.Record, error) {
	if filters == nil || filters.FilterObj == nil {
		return nil, fmt.Errorf("filter cannot be nil or unitialized")
	}
	return s.queryRaw(filters.FilterObj.FilterObject, filters.Order, limit, offset)
}

func (s *dsObjectStore) QueryFromFulltext(results []database.FulltextResult, params database.Filters, limit int, offset int) ([]database.Record, error) {
	records := make([]database.Record, 0, len(results))
	resultObjectMap := make(map[string]struct{})
	// we assume that results are already sorted by score DESC.
	// this mean we use map to ignore duplicates without checking score
	err := s.db.View(func(txn *badger.Txn) error {
		for _, res := range results {
			// Don't use spaceID because expected objects are virtual
			if sbt, err := typeprovider.SmartblockTypeFromID(res.Path.ObjectId); err == nil {
				if indexDetails, _ := sbt.Indexable(); !indexDetails && s.sourceService != nil {
					details, err := s.sourceService.DetailsFromIdBasedSource(res.Path.ObjectId)
					if err != nil {
						log.Errorf("QueryByIds failed to GetDetailsFromIdBasedSource id: %s", res.Path.ObjectId)
						continue
					}
					details.Fields[database.RecordIDField] = pbtypes.ToValue(res.Path.ObjectId)
					details.Fields[database.RecordScoreField] = pbtypes.ToValue(res.Score)
					rec := database.Record{Details: details}
					if params.FilterObj == nil || params.FilterObj.FilterObject(rec.Details) {
						resultObjectMap[res.Path.ObjectId] = struct{}{}
						records = append(records, rec)
					}
					continue
				}
			}
			it, err := txn.Get(pagesDetailsBase.ChildString(res.Path.ObjectId).Bytes())
			if err != nil {
				log.Infof("QueryByIds failed to find id: %s", res.Path.ObjectId)
				continue
			}

			detailsNoCopy, err := s.extractDetailsFromItem(it)
			if err != nil {
				log.Errorf("QueryByIds failed to extract details: %s", res.Path.ObjectId)
				continue
			}
			details := pbtypes.CopyStruct(detailsNoCopy.Details, false)
			details.Fields[database.RecordScoreField] = pbtypes.ToValue(res.Score)

			rec := database.Record{Details: details}
			if params.FilterObj == nil || params.FilterObj.FilterObject(rec.Details) {
				rec.Meta = res.Model()
				if _, ok := resultObjectMap[res.Path.ObjectId]; !ok {
					records = append(records, rec)
					resultObjectMap[res.Path.ObjectId] = struct{}{}
				}
			}

			injectedResults := s.getInjectedResults(details, res.Score, res.Path, 10, params)
			if len(injectedResults) == 0 {
				continue
			}
			// for now, we only allow one injected result per object
			// this may happen when we for example have a match in the different tags of the same object,
			// or we may already have a better match for the same object but in block
			injectedResults = lo.Filter(injectedResults, func(item database.Record, _ int) bool {
				id := pbtypes.GetString(item.Details, bundle.RelationKeyId.String())
				if _, ok := resultObjectMap[id]; !ok {
					resultObjectMap[id] = struct{}{}
					return true
				}
				return false
			})

			records = append(records, injectedResults...)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	if offset >= len(records) {
		return nil, nil
	}
	if params.Order != nil {
		sort.Slice(records, func(i, j int) bool {
			return params.Order.Compare(records[i].Details, records[j].Details) == -1
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

func (s *dsObjectStore) performQuery(q database.Query) (records []database.Record, err error) {
	filters, err := database.NewFilters(q, s)
	if err != nil {
		return nil, fmt.Errorf("new filters: %w", err)
	}
	if q.FullText != "" {
		highlighter := q.Highlighter
		if highlighter == "" {
			highlighter = ftsearch.DefaultHighlightFormatter
		}

		fulltextResults, err := s.performFulltextSearch(q.FullText, highlighter, filters)
		if err != nil {
			return nil, fmt.Errorf("perform fulltext search: %w", err)
		}

		return s.QueryFromFulltext(fulltextResults, *filters, q.Limit, q.Offset)
	}
	return s.QueryRaw(filters, q.Limit, q.Offset)
}

// jsonHighlightToRanges converts json highlight to runes ranges
// input ranges are positions of bytes, the returned ranges are position of runes
func jsonHighlightToRanges(s string) (text string, ranges []*model.Range) {
	fragment, err := jsonFormatter.UnmarshalFromString(s)
	if err != nil {
		log.Warnf("Failed to unmarshal json highlight: %v", err)
		// fallback to plain text without ranges
		return string(fragment.Text), nil
	}
	// sort ranges, because we need to have a guarantee that they are not overlapping
	slices.SortFunc(fragment.Ranges, func(a, b [2]int) int {
		if a[0] < b[0] {
			return -1
		}
		if a[0] > b[0] {
			return 1
		}
		return 0
	})

	for i, rangesAr := range fragment.Ranges {
		if i > 0 && fragment.Ranges[i-1][1] >= rangesAr[0] {
			// overlapping ranges
			continue
		}
		if rangesAr[0] < 0 || rangesAr[1] < 0 {
			continue
		}
		if rangesAr[0] > rangesAr[1] {
			continue
		}
		if rangesAr[0] == rangesAr[1] {
			continue
		}
		if rangesAr[0] > len(fragment.Text) || rangesAr[1] > len(fragment.Text) {
			continue
		}
		if rangesAr[0] > math.MaxInt32 || rangesAr[1] > math.MaxInt32 {
			continue
		}

		ranges = append(ranges, &model.Range{
			From: int32(text2.UTF16RuneCount(fragment.Text[:rangesAr[0]])),
			To:   int32(text2.UTF16RuneCount(fragment.Text[:rangesAr[1]])),
		})
	}

	return string(fragment.Text), ranges
}

func (s *dsObjectStore) performFulltextSearch(text string, highlightFormatter ftsearch.HighlightFormatter, filters *database.Filters) ([]database.FulltextResult, error) {
	spaceID := getSpaceIDFromFilter(filters.FilterObj)
	bleveResults, err := s.fts.Search(spaceID, highlightFormatter, text)
	if err != nil {
		return nil, fmt.Errorf("fullText search: %w", err)
	}

	var resultsByObjectId = make(map[string][]*search.DocumentMatch)
	for _, result := range bleveResults {
		path, err := domain.NewFromPath(result.ID)
		if err != nil {
			return nil, fmt.Errorf("fullText search: %w", err)
		}
		if _, ok := resultsByObjectId[path.ObjectId]; !ok {
			resultsByObjectId[path.ObjectId] = make([]*search.DocumentMatch, 0, 1)
		}

		resultsByObjectId[path.ObjectId] = append(resultsByObjectId[path.ObjectId], result)
	}

	for objectId := range resultsByObjectId {
		sort.Slice(resultsByObjectId[objectId], func(i, j int) bool {
			if bleveResults[i].Score > bleveResults[j].Score {
				return true
			}
			// to make the search deterministic in case we have the same-score results we can prioritize the one with the higher ID
			// e.g. we have 2 matches:
			// 1. Block "Done" (id "b/id")
			// 2. Relation Status: "Done" (id "r/status")
			// if the score is the same, lets prioritize the relation, as it has more context for this short result
			// Usually, blocks are naturally longer than relations and will have a lower score
			if bleveResults[i].Score == bleveResults[j].Score {
				return bleveResults[i].ID > bleveResults[j].ID
			}
			return false
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

	var results = make([]database.FulltextResult, 0, len(objectResults))
	for _, result := range objectResults {
		path, err := domain.NewFromPath(result.ID)
		if err != nil {
			return nil, fmt.Errorf("fullText search: %w", err)
		}
		var highlight string
		for _, v := range result.Fragments {
			if len(v) > 0 {
				highlight = v[0]
				break
			}
		}
		res := database.FulltextResult{
			Path:      path,
			Highlight: highlight,
			Score:     result.Score,
		}
		if highlightFormatter == ftsearch.JSONHighlightFormatter {
			res.Highlight, res.HighlightRanges = jsonHighlightToRanges(highlight)
		}
		if result.Score < minFulltextScore && len(res.HighlightRanges) == 0 {
			continue
		}
		results = append(results, res)

	}

	return results, nil
}

func getSpaceIDFromFilter(fltr database.Filter) (spaceID string) {
	switch f := fltr.(type) {
	case database.FilterEq:
		if f.Key == bundle.RelationKeySpaceId.String() {
			return f.Value.GetStringValue()
		}
	case database.FilterIn:
		if f.Key == bundle.RelationKeySpaceId.String() {
			values := f.Value.GetValues()
			if len(values) == 1 {
				return values[0].GetStringValue()
			} else {
				return ""
			}
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
	recs, err := s.performQuery(q)
	if err != nil {
		return nil, 0, fmt.Errorf("build query: %w", err)
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
