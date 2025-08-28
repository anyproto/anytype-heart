package spaceindex

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	text2 "github.com/anyproto/anytype-heart/util/text"
)

var pluralNameId = domain.ObjectPath{
	ObjectId:    "",
	RelationKey: bundle.RelationKeyPluralName.String(),
}.String()

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
	if path.RelationKey != bundle.RelationKeyName.String() && path.RelationKey != bundle.RelationKeyPluralName.String() {
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

	//nolint:gosec
	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
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
			bundle.RelationKeyResolvedLayout.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		})
		metaInj := model.SearchMeta{
			RelationKey:     relationKey,
			RelationDetails: relDetails,
		}

		detailsCopy := rec.Details.Copy()
		// set the same score as original object
		detailsCopy.SetFloat64(database.RecordScoreField, score)
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
	iter, err := query.Iter(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("find: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		details, err := domain.NewDetailsFromAnyEnc(doc.Value())
		if err != nil {
			return nil, fmt.Errorf("json to proto: %w", err)
		}
		records = append(records, database.Record{Details: details})
	}
	err = iter.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
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
			if _, indexDetails, _ := sbt.Indexable(); !indexDetails && s.sourceService != nil {
				details, err := s.sourceService.DetailsFromIdBasedSource(domain.FullID{
					ObjectID: res.Path.ObjectId,
					SpaceID:  s.SpaceId(),
				})
				if err != nil {
					log.Errorf("QueryByIds failed to GetDetailsFromIdBasedSource id: %s", res.Path.ObjectId)
					continue
				}
				details.SetString(bundle.RelationKeyId, res.Path.ObjectId)
				details.SetFloat64(database.RecordScoreField, res.Score)
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
		details, err := domain.NewDetailsFromAnyEnc(doc.Value())
		if err != nil {
			log.Errorf("QueryByIds failed to extract details: %s", res.Path.ObjectId)
			continue
		}
		details.SetFloat64(database.RecordScoreField, res.Score)

		rec := database.Record{Details: details}
		if params.FilterObj == nil || params.FilterObj.FilterObject(rec.Details) {
			rec.Meta = res.Model()
			if rec.Meta.Highlight == "" {
				title := details.GetString(bundle.RelationKeyPluralName)
				if title == "" {
					title = details.GetString(bundle.RelationKeyName)
				}
				index := strings.Index(strings.ToLower(title), strings.ToLower(ftsSearch))
				titleArr := []byte(title)
				if index != -1 {
					from := int32(text2.UTF16RuneCount(titleArr[:index]))
					rec.Meta.HighlightRanges = []*model.Range{{
						From: from,
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
			// TODO: do we need orderIdsMap here?
			return params.Order.Compare(records[i].Details, records[j].Details, nil) == -1
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

	q.TextQuery = strings.TrimSpace(q.TextQuery)
	filters, err := database.NewFilters(q, s, arena, collatorBuffer)
	if err != nil {
		return nil, fmt.Errorf("new filters: %w", err)
	}
	if q.TextQuery != "" {
		var fulltextResults []database.FulltextResult
		if q.PrefixNameQuery {
			fulltextResults, err = s.performFulltextSearch(func() (results []*ftsearch.DocumentMatch, err error) {
				return s.fts.NamePrefixSearch(q.SpaceId, q.TextQuery)
			})
		} else {
			fulltextResults, err = s.performFulltextSearch(func() (results []*ftsearch.DocumentMatch, err error) {
				return s.fts.Search(q.SpaceId, q.TextQuery)
			})
		}

		if err != nil {
			return nil, fmt.Errorf("perform fulltext search: %w", err)
		}

		return s.QueryFromFulltext(fulltextResults, *filters, q.Limit, q.Offset, q.TextQuery)
	}
	return s.QueryRaw(filters, q.Limit, q.Offset)
}

func (s *dsObjectStore) performFulltextSearch(search func() (results []*ftsearch.DocumentMatch, err error)) ([]database.FulltextResult, error) {
	ftsResults, err := search()
	if err != nil {
		return nil, fmt.Errorf("fullText search: %w", err)
	}

	var resultsByObjectId = make(map[string][]*ftsearch.DocumentMatch)
	for _, result := range ftsResults {
		path, err := domain.NewFromPath(result.ID)
		if err != nil {
			return nil, fmt.Errorf("fullText search: %w", err)
		}
		if _, ok := resultsByObjectId[path.ObjectId]; !ok {
			resultsByObjectId[path.ObjectId] = make([]*ftsearch.DocumentMatch, 0, 1)
		}

		resultsByObjectId[path.ObjectId] = append(resultsByObjectId[path.ObjectId], result)
	}

	for objectId := range resultsByObjectId {
		cur := resultsByObjectId[objectId]
		slices.SortFunc(cur, func(a, b *ftsearch.DocumentMatch) int {
			if a.Score == b.Score {
				// to make the search deterministic in case we have the same-score results we can prioritize the one with the higher ID
				// e.g. we have 2 matches:
				// 1. Block "Done" (id "b/id")
				// 2. Relation Status: "Done" (id "r/status")
				// if the score is the same, lets prioritize the relation, as it has more context for this short result
				// Usually, blocks are naturally longer than relations and will have a lower score
				return strings.Compare(b.ID, a.ID)
			}
			return int(b.Score - a.Score)
		})
	}

	// select only the best block/relation result for each object
	var objectResults = make([]*ftsearch.DocumentMatch, 0, len(resultsByObjectId))
	for _, objectPerBlockResults := range resultsByObjectId {
		if len(objectPerBlockResults) == 0 {
			continue
		}
		objectResults = append(objectResults, preferPluralNameRelation(objectPerBlockResults))
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
		var ranges []*model.Range
		for _, v := range result.Fragments {
			if len(v.Ranges) > 0 {
				highlight = v.Text
				ranges = convertToHighlightRanges(v.Ranges, highlight)
				break
			}
		}
		res := database.FulltextResult{
			Path:      path,
			Highlight: highlight,
			Score:     result.Score,
		}
		if highlight != "" {
			res.Highlight, res.HighlightRanges = highlight, ranges
		}
		if result.Score < minFulltextScore && len(res.HighlightRanges) == 0 {
			continue
		}
		results = append(results, res)

	}

	return results, nil
}

func preferPluralNameRelation(objectPerBlockResults []*ftsearch.DocumentMatch) *ftsearch.DocumentMatch {
	doc, found := lo.Find(objectPerBlockResults, func(item *ftsearch.DocumentMatch) bool {
		return strings.HasSuffix(item.ID, pluralNameId)
	})
	if !found {
		doc = objectPerBlockResults[0]
	}
	return doc
}

func convertToHighlightRanges(ranges [][]int, highlight string) []*model.Range {
	var highlightRanges []*model.Range

	byteToRuneIndex := make([]int, len(highlight)+1)
	for i := range byteToRuneIndex {
		byteToRuneIndex[i] = text2.UTF16RuneCountString(highlight[:i])
	}

	for _, r := range ranges {
		if len(r) == 2 {
			fromByte := r[0]
			toByte := r[1]

			if fromByte < 0 || toByte > len(highlight) {
				continue
			}

			fromRune := byteToRuneIndex[fromByte]
			toRune := byteToRuneIndex[toByte]

			highlightRange := &model.Range{
				From: int32(fromRune),
				To:   int32(toRune),
			}
			highlightRanges = append(highlightRanges, highlightRange)
		}
	}

	return highlightRanges
}

// TODO: objstore: no one uses total
func (s *dsObjectStore) QueryObjectIds(q database.Query) (ids []string, total int, err error) {
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

func (s *dsObjectStore) QueryByIds(ids []string) (records []database.Record, err error) {
	for _, id := range ids {
		// Don't use spaceID because expected objects are virtual
		if sbt, err := typeprovider.SmartblockTypeFromID(id); err == nil {
			if _, indexDetails, _ := sbt.Indexable(); !indexDetails && s.sourceService != nil {
				details, err := s.sourceService.DetailsFromIdBasedSource(domain.FullID{
					ObjectID: id,
					SpaceID:  s.SpaceId(),
				})
				if err != nil {
					log.With("id", id).Errorf("QueryByIds failed to GetDetailsFromIdBasedSource id: %s", err.Error())
					continue
				}
				details.SetString(bundle.RelationKeyId, id)
				records = append(records, database.Record{Details: details})
				continue
			}
		}
		doc, err := s.objects.FindId(s.componentCtx, id)
		if err != nil {
			log.With("id", id).Infof("QueryByIds failed to find id: %s", err.Error())
			continue
		}
		details, err := domain.NewDetailsFromAnyEnc(doc.Value())
		if err != nil {
			log.With("id", id).Errorf("QueryByIds failed to extract details: %s", err.Error())
			continue
		}
		records = append(records, database.Record{Details: details})
	}
	return
}

func (s *dsObjectStore) QueryByIdsAndSubscribeForChanges(ids []string, sub database.Subscription) (records []database.Record, closeFunc func(), err error) {
	if sub == nil {
		err = fmt.Errorf("subscription func is nil")
		return
	}
	sub.Subscribe(ids)
	records, err = s.QueryByIds(ids)
	if err != nil {
		// can mean only the datastore is already closed, so we can resign and return
		log.Errorf("QueryByIdsAndSubscribeForChanges failed to query ids: %v", err)
		return nil, nil, err
	}

	closeFunc = func() {
		s.closeAndRemoveSubscription(sub)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

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

func (s *dsObjectStore) QueryIterate(q database.Query, proc func(details *domain.Details)) (err error) {
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
	defer iter.Close()

	for iter.Next() {
		var doc anystore.Doc
		doc, err = iter.Doc()
		if err != nil {
			err = fmt.Errorf("get doc: %w", err)
			return
		}

		var details *domain.Details
		details, err = domain.NewDetailsFromAnyEnc(doc.Value())
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

func (s *dsObjectStore) IterateAll(proc func(doc *anyenc.Value) error) error {
	iter, err := s.objects.Find(nil).Iter(s.componentCtx)
	if err != nil {
		return fmt.Errorf("iterate all ids: %w", err)
	}
	defer iter.Close()

	const maxErrorsToLog = 5
	var loggedErrors int

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			if loggedErrors < maxErrorsToLog {
				log.With("error", err).Error("IterateAll: get doc")
				loggedErrors++
			}
			continue
		}
		err = proc(doc.Value())
		if err != nil {
			return err
		}
	}
	err = iter.Err()
	if err != nil {
		return fmt.Errorf("iterate: %w", err)
	}
	return nil
}

func (s *dsObjectStore) ListIds() ([]string, error) {
	var ids []string
	iter, err := s.objects.Find(nil).Iter(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("find all: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		id := doc.Value().GetStringBytes("id")
		ids = append(ids, string(id))
	}
	err = iter.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}
	return ids, nil
}

func (s *dsObjectStore) ListFullIds() ([]domain.FullID, error) {
	var ids []domain.FullID
	iter, err := s.objects.Find(nil).Iter(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("find all: %w", err)
	}
	defer iter.Close()
	idKey := bundle.RelationKeyId.String()
	spaceIdKey := bundle.RelationKeySpaceId.String()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		id := doc.Value().GetString(idKey)
		spaceId := doc.Value().GetString(spaceIdKey)
		ids = append(ids, domain.FullID{ObjectID: id, SpaceID: spaceId})
	}
	err = iter.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}
	return ids, nil
}
