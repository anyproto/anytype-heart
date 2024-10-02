package objectstore

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"golang.org/x/text/collate"

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
func (s *dsObjectStore) getObjectsWithObjectInRelation(relationKey domain.RelationKey, objectId string, limit int, params database.Filters) ([]database.Record, error) {
	f := database.FiltersAnd{
		database.FilterAllIn{Key: relationKey, Strings: []string{objectId}},
		params.FilterObj,
	}
	return s.queryAnyStore(f, params.Order, uint(limit), 0)
}

func (s *dsObjectStore) getInjectedResults(details *domain.Details, score float64, path domain.ObjectPath, maxLength int, params database.Filters) []database.Record {
	var injectedResults []database.Record
	id := details.GetString(bundle.RelationKeyId)
	if path.RelationKey != bundle.RelationKeyName.String() {
		// inject only in case we match the name
		return nil
	}
	var (
		relationKey string
		err         error
	)

	isDeleted := details.GetBool(bundle.RelationKeyIsDeleted)
	isArchived := details.GetBool(bundle.RelationKeyIsArchived)
	if isDeleted || isArchived {
		return nil
	}

	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyLayout))
	switch layout {
	case model.ObjectType_relationOption:
		relationKey = details.GetString(bundle.RelationKeyRelationKey)
	case model.ObjectType_objectType:
		relationKey = bundle.RelationKeyType.String()
	default:
		return nil
	}
	recs, err := s.getObjectsWithObjectInRelation(domain.RelationKey(relationKey), id, maxLength, params)
	if err != nil {
		log.Errorf("getInjectedResults failed to get objects with object in relation: %v", err)
		return nil
	}

	for _, rec := range recs {
		relDetails := pbtypes.StructFilterKeys(details.ToProto(), []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyType.String(),
			bundle.RelationKeyLayout.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		})
		metaInj := model.SearchMeta{
			RelationKey:     relationKey,
			RelationDetails: relDetails,
		}

		detailsCopy := rec.Details.Copy()
		// set the same score as original object
		detailsCopy.SetFloat(database.RecordScoreField, score)
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

func (s *dsObjectStore) queryAnyStore(filter database.Filter, order database.Order, limit uint, offset uint) ([]database.Record, error) {
	anystoreFilter := filter.AnystoreFilter()
	var sortsArg []any
	if order != nil {
		sorts := order.AnystoreSort()
		if sorts != nil {
			sortsArg = []any{sorts}
		}
	}
	var records []database.Record
	query := s.objects.Find(anystoreFilter).Sort(sortsArg...).Offset(offset).Limit(limit)
	now := time.Now()
	defer func() {
		// Debug slow queries
		if false {
			dur := time.Since(now)
			if dur.Milliseconds() > 100 {
				explain := ""
				if exp, expErr := query.Explain(s.componentCtx); expErr == nil {
					for _, idx := range exp.Indexes {
						if idx.Used {
							explain += fmt.Sprintf("index: %s %d ", idx.Name, idx.Weight)
						}
					}
				}
				fmt.Printf(
					"SLOW QUERY:\t%v\nFilter:\t%s\nNum results:\t%d\nExplain:\t%s\nSorts:\t%#v\n",
					dur, anystoreFilter, len(records), explain, sortsArg,
				)
			}
		}
	}()
	// TODO Some problem with parsing anystoreFilter.String()
	// TODO It's because of empty And/Or filter
	iter, err := query.Iter(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("find: %w", err)
	}
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(fmt.Errorf("get doc: %w", err), iter.Close())
		}
		details, err := domain.JsonToProto(doc.Value())
		if err != nil {
			return nil, errors.Join(fmt.Errorf("json to proto: %w", err), iter.Close())
		}
		records = append(records, database.Record{Details: details})
	}
	err = iter.Err()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("iterate: %w", err), iter.Close())
	}
	err = iter.Close()
	if err != nil {
		return nil, fmt.Errorf("close iterator: %w", err)
	}
	return records, nil
}

func (s *dsObjectStore) QueryRaw(filters *database.Filters, limit int, offset int) ([]database.Record, error) {
	if filters == nil || filters.FilterObj == nil {
		return nil, fmt.Errorf("filter cannot be nil or unitialized")
	}
	return s.queryAnyStore(filters.FilterObj, filters.Order, uint(limit), uint(offset))
}

func (s *dsObjectStore) QueryFromFulltext(results []database.FulltextResult, params database.Filters, limit int, offset int, ftsSearch string) ([]database.Record, error) {
	records := make([]database.Record, 0, len(results))
	resultObjectMap := make(map[string]struct{})
	// we assume that results are already sorted by score DESC.
	// this mean we use map to ignore duplicates without checking score
	for _, res := range results {
		// Don't use spaceID because expected objects are virtual
		if sbt, err := typeprovider.SmartblockTypeFromID(res.Path.ObjectId); err == nil {
			if indexDetails, _ := sbt.Indexable(); !indexDetails && s.sourceService != nil {
				details, err := s.sourceService.DetailsFromIdBasedSource(res.Path.ObjectId)
				if err != nil {
					log.Errorf("QueryByIds failed to GetDetailsFromIdBasedSource id: %s", res.Path.ObjectId)
					continue
				}
				details.SetString(bundle.RelationKeyId, res.Path.ObjectId)
				details.SetFloat(database.RecordScoreField, res.Score)
				rec := database.Record{Details: details}
				if params.FilterObj == nil || params.FilterObj.FilterObject(rec.Details) {
					resultObjectMap[res.Path.ObjectId] = struct{}{}
					records = append(records, rec)
				}
				continue
			}
		}
		doc, err := s.objects.FindId(s.componentCtx, res.Path.ObjectId)
		if err != nil {
			log.Errorf("QueryByIds failed to find id: %s", res.Path.ObjectId)
			continue
		}
		details, err := domain.JsonToProto(doc.Value())
		if err != nil {
			log.Errorf("QueryByIds failed to extract details: %s", res.Path.ObjectId)
			continue
		}
		details.SetFloat(database.RecordScoreField, res.Score)

		rec := database.Record{Details: details}
		if params.FilterObj == nil || params.FilterObj.FilterObject(rec.Details) {
			rec.Meta = res.Model()
			if rec.Meta.Highlight == "" {
				title := details.GetString(bundle.RelationKeyName)
				index := strings.Index(strings.ToLower(title), strings.ToLower(ftsSearch))
				titleArr := []byte(title)
				if index != -1 {
					from := int32(text2.UTF16RuneCount(titleArr[:index]))
					rec.Meta.HighlightRanges = []*model.Range{{
						From: int32(text2.UTF16RuneCount(titleArr[:from])),
						To:   from + int32(text2.UTF16RuneCount([]byte(ftsSearch)))}}
					rec.Meta.Highlight = title
				}
			}
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
			id := item.Details.GetString(bundle.RelationKeyId)
			if _, ok := resultObjectMap[id]; !ok {
				resultObjectMap[id] = struct{}{}
				return true
			}
			return false
		})

		records = append(records, injectedResults...)
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
	arena := s.arenaPool.Get()
	defer s.arenaPool.Put(arena)

	collatorBuffer := s.collatorBufferPool.get()
	defer s.collatorBufferPool.put(collatorBuffer)

	q.FullText = strings.TrimSpace(q.FullText)
	filters, err := database.NewFilters(q, s, arena, collatorBuffer)
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

		return s.QueryFromFulltext(fulltextResults, *filters, q.Limit, q.Offset, q.FullText)
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
	spaceIds := getSpaceIdsFromFilter(filters.FilterObj)
	bleveResults, err := s.fts.Search(spaceIds, highlightFormatter, text)
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
		if highlightFormatter == ftsearch.JSONHighlightFormatter && highlight != "" {
			res.Highlight, res.HighlightRanges = jsonHighlightToRanges(highlight)
		}
		if result.Score < minFulltextScore && len(res.HighlightRanges) == 0 {
			continue
		}
		results = append(results, res)

	}

	return results, nil
}

func getSpaceIdsFromFilter(fltr database.Filter) []string {
	switch f := fltr.(type) {
	case database.FilterEq:
		if f.Key == bundle.RelationKeySpaceId {
			return []string{f.Value.String()}
		}
	case database.FilterIn:
		if f.Key == bundle.RelationKeySpaceId {
			spaceIds := make([]string, 0, len(f.Value))
			for _, v := range f.Value {
				spaceIds = append(spaceIds, v.String())
			}
			return spaceIds
		}
	case database.FiltersAnd:
		return iterateOverAndFilters(f)
	}
	return nil
}

func iterateOverAndFilters(fs []database.Filter) []string {
	var spaceIds []string
	for _, f := range fs {
		spaceIds = append(spaceIds, getSpaceIdsFromFilter(f)...)
	}
	return spaceIds
}

// TODO: objstore: no one uses total
func (s *dsObjectStore) QueryObjectIDs(q database.Query) (ids []string, total int, err error) {
	recs, err := s.performQuery(q)
	if err != nil {
		return nil, 0, fmt.Errorf("build query: %w", err)
	}
	ids = make([]string, 0, len(recs))
	for _, rec := range recs {
		id, ok := rec.Details.TryString(bundle.RelationKeyId)
		if ok {
			ids = append(ids, id)
		}
	}
	return ids, len(recs), nil
}

func (s *dsObjectStore) QueryByID(ids []string) (records []database.Record, err error) {
	for _, id := range ids {
		// Don't use spaceID because expected objects are virtual
		if sbt, err := typeprovider.SmartblockTypeFromID(id); err == nil {
			if indexDetails, _ := sbt.Indexable(); !indexDetails && s.sourceService != nil {
				details, err := s.sourceService.DetailsFromIdBasedSource(id)
				if err != nil {
					log.Errorf("QueryByIds failed to GetDetailsFromIdBasedSource id: %s", id)
					continue
				}
				details.SetString(bundle.RelationKeyId, id)
				records = append(records, database.Record{Details: details})
				continue
			}
		}
		doc, err := s.objects.FindId(s.componentCtx, id)
		if err != nil {
			log.Infof("QueryByIds failed to find id: %s", id)
			continue
		}
		details, err := domain.JsonToProto(doc.Value())
		if err != nil {
			log.Errorf("QueryByIds failed to extract details: %s", id)
			continue
		}
		records = append(records, database.Record{Details: details})
	}
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

type collatorBufferPool struct {
	pool *sync.Pool
}

func newCollatorBufferPool() *collatorBufferPool {
	return &collatorBufferPool{
		pool: &sync.Pool{
			New: func() interface{} {
				return &collate.Buffer{}
			},
		},
	}
}

func (p *collatorBufferPool) get() *collate.Buffer {
	return p.pool.Get().(*collate.Buffer)
}

func (p *collatorBufferPool) put(b *collate.Buffer) {
	b.Reset()
	p.pool.Put(b)
}

func (s *dsObjectStore) QueryIterate(q database.Query, proc func(details *types.Struct)) (err error) {
	arena := s.arenaPool.Get()
	defer s.arenaPool.Put(arena)

	collatorBuffer := s.collatorBufferPool.get()
	defer s.collatorBufferPool.put(collatorBuffer)

	filters, err := database.NewFilters(q, s, arena, collatorBuffer)
	if err != nil {
		return fmt.Errorf("new filters: %w", err)
	}

	anystoreFilter := filters.FilterObj.AnystoreFilter()
	query := s.objects.Find(anystoreFilter)

	iter, err := query.Iter(s.componentCtx)
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}

	defer func() {
		if iterCloseErr := iter.Close(); iterCloseErr != nil {
			err = errors.Join(iterCloseErr, err)
		}
	}()

	for iter.Next() {
		var doc anystore.Doc
		doc, err = iter.Doc()
		if err != nil {
			err = fmt.Errorf("get doc: %w", err)
			return
		}

		var details *types.Struct
		details, err = pbtypes.JsonToProto(doc.Value())
		if err != nil {
			err = fmt.Errorf("json to proto: %w", err)
			return
		}
		proc(details)
	}
	err = iter.Err()
	if err != nil {
		err = fmt.Errorf("iterate: %w", err)
		return
	}
	return
}
