package spaceindex

import (
	"cmp"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

var nameId = domain.ObjectPath{
	ObjectId:    "",
	RelationKey: bundle.RelationKeyName.String(),
}.String()

var (
	ftHits                atomic.Int64
	ftMisses              atomic.Int64
	ftCandidatesTruncated atomic.Int64
)

const (
	// minFulltextScore trim fulltext results with score lower than this value in case there are no highlight ranges available
	minFulltextScore = 0.02
	// ftCandidatesMin is the docs budget of the first fulltext round. It doubles
	// as the noise gate: BM25 scores aren't comparable across queries, so "the
	// top N most promising matches" is the only workable relevance cutoff.
	ftCandidatesMin = 100
	// ftCandidatesMultiplier pads the first round over the requested page:
	// grouping and filters almost always trim some candidates, so an unpadded
	// budget would need an extra escalation round (and re-resolution of all
	// candidates) for nearly every full page.
	ftCandidatesMultiplier = 2
	// ftCandidatesHardLimit bounds budget escalation. Escalation only happens
	// when store-level filters starved the requested page, so the common query
	// never pays for it.
	ftCandidatesHardLimit = 2000
	// ftRerankPoolSize is the fixed number of top-BM25 objects whose order the
	// additive final-score boosts (recency, name match) may rearrange. It must
	// NOT depend on the requested page: re-ranking a budget-dependent pool
	// makes offset pagination return duplicates (see queryFromFulltextRecords).
	ftRerankPoolSize = ftCandidatesMin
)

// ftCandidatesLimit derives the INITIAL tantivy docs budget from the requested
// page. Without an explicit limit the request starts with one conservative
// 100-doc round ("everything" combined with fulltext means "the relevant
// matches", not the whole index); note the escalation loop can still grow any
// request's budget to materialize the full re-rank head, which must be the
// same object set for every request.
func ftCandidatesLimit(q database.Query) int {
	if q.Limit <= 0 {
		return ftCandidatesMin
	}
	limit := ftCandidatesMultiplier * (q.Offset + q.Limit)
	if limit < ftCandidatesMin {
		return ftCandidatesMin
	}
	if limit > ftCandidatesHardLimit {
		return ftCandidatesHardLimit
	}
	return limit
}

func (s *dsObjectStore) Query(q database.Query) ([]database.Record, error) {
	recs, err := s.performQuery(q)
	return recs, err
}

// QueryAndCount runs the query (respecting limit/offset) and additionally returns the total number
// of objects matching the filters, ignoring limit/offset. The filters are compiled only once and
// reused for both the limited query and the count, and both reads run in a single read transaction
// so the page and the total reflect a consistent snapshot. It applies the same implicit filters as
// Query (isArchived/isDeleted/objectType). Fulltext queries are not supported.
func (s *dsObjectStore) QueryAndCount(q database.Query) (records []database.Record, total int, err error) {
	arena := s.arenaPool.Get()
	defer s.arenaPool.Put(arena)

	collatorBuffer := s.collatorBufferPool.get()
	defer s.collatorBufferPool.put(collatorBuffer)

	q.TextQuery = strings.TrimSpace(q.TextQuery)
	if q.TextQuery != "" {
		return nil, 0, fmt.Errorf("QueryAndCount does not support fulltext queries")
	}

	filters, err := database.NewFilters(q, s, arena, collatorBuffer)
	if err != nil {
		return nil, 0, fmt.Errorf("new filters: %w", err)
	}

	tx, err := s.db.ReadTx(s.componentCtx)
	if err != nil {
		return nil, 0, fmt.Errorf("read tx: %w", err)
	}
	defer func() {
		if cmErr := tx.Commit(); cmErr != nil && err == nil {
			records, total, err = nil, 0, fmt.Errorf("commit read tx: %w", cmErr)
		}
	}()

	records, err = s.queryAnyStore(tx.Context(), filters.FilterObj, filters.Order, uint(q.Limit), uint(q.Offset))
	if err != nil {
		return nil, 0, fmt.Errorf("query any store: %w", err)
	}

	// When the page reached the end of the result set, the total is known without a separate count:
	// it's the offset plus the number of records on this final page. The page reaches the end when the
	// query is unbounded (limit 0) or returned fewer records than the limit. We additionally require
	// len > 0 (or a zero offset) to rule out an offset that overshot the result set, which is
	// indistinguishable from an empty match without counting.
	if (q.Limit == 0 || len(records) < q.Limit) && (len(records) > 0 || q.Offset == 0) {
		return records, q.Offset + len(records), nil
	}

	// Otherwise count the full result set, reusing the already-compiled filters and the same read
	// transaction, without materializing or sorting.
	total, err = s.objects.Find(filters.FilterObj.AnystoreFilter()).Count(tx.Context())
	if err != nil {
		return nil, 0, fmt.Errorf("count objects: %w", err)
	}
	return records, total, nil
}

func (s *dsObjectStore) queryAnyStore(ctx context.Context, filter database.Filter, order database.Order, limit uint, offset uint) ([]database.Record, error) {
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
				if exp, expErr := query.Explain(ctx); expErr == nil {
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
	iter, err := query.Iter(ctx)
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
	return s.queryAnyStore(s.componentCtx, filters.FilterObj, filters.Order, uint(limit), uint(offset))
}

func (s *dsObjectStore) CountRaw(filters *database.Filters) (int, error) {
	if filters == nil || filters.FilterObj == nil {
		return 0, fmt.Errorf("filter cannot be nil or unitialized")
	}
	count, err := s.objects.Find(filters.FilterObj.AnystoreFilter()).Count(s.componentCtx)
	if err != nil {
		return 0, fmt.Errorf("count objects: %w", err)
	}
	return count, nil
}

type injectionHit struct {
	id      string
	details *domain.Details
	score   float64
}

// NOTE: a candidate-drop telemetry (classification of FT candidates rejected
// by store filters: missing/deleted = stale-index anomalies, archived/hidden =
// expected drops) lives on the go-7316-ft-drop-stats branch for debugging
// index/store consistency; it is intentionally kept out of the production path.

func (s *dsObjectStore) QueryFromFulltext(results []database.FulltextResult, params database.Filters, limit int, offset int, ftsSearch string, withInjections bool) ([]database.Record, error) {
	needed := 0
	if limit > 0 {
		needed = offset + limit
	}
	records := s.queryFromFulltextRecordsOpts(results, params, ftsSearch, needed, withInjections)
	return paginateRecords(records, offset, limit), nil
}

// paginateRecords applies offset/limit to the final, sorted record list.
func paginateRecords(records []database.Record, offset int, limit int) []database.Record {
	if offset >= len(records) {
		return nil
	}
	if limit > 0 {
		upperBound := offset + limit
		if upperBound > len(records) {
			upperBound = len(records)
		}
		return records[offset:upperBound]
	}
	return records[offset:]
}

// queryFromFulltextRecords resolves fulltext candidates to filtered, sorted
// records WITHOUT applying offset/limit, so callers can tell whether the
// candidate set produced enough results to fill the requested page.
//
// The ordering is two-tier to keep offset pagination consistent: the
// final-score sort (BM25 + recency/name boosts) is applied only to the first
// ftRerankPoolSize candidates — a fixed pool that does not depend on the
// requested page — while everything beyond stays in BM25 order. The BM25
// object order is prefix-stable under candidate-budget growth (a bigger top-K
// only appends lower-scoring objects), so re-ranking a fixed head keeps the
// whole sequence stable across requests with different offsets; re-ranking
// the entire budget-dependent pool would let a boosted tail candidate jump
// into an earlier page and produce duplicates on the next one.
//
// Known residual edge: docs with EXACTLY equal BM25 scores straddling the
// budget boundary enter the set in tantivy's internal doc order, so a larger
// budget can insert a tied object mid-order (the id tiebreak only orders the
// objects it can see). Exact float ties at the boundary are rare; accepted.
func (s *dsObjectStore) queryFromFulltextRecords(results []database.FulltextResult, params database.Filters, ftsSearch string, needed int) []database.Record {
	return s.queryFromFulltextRecordsOpts(results, params, ftsSearch, needed, true)
}

// withInjections controls the related-object injections (objects linking to /
// typed by / tagged with a name-matched object). Each injection group costs an
// anystore query over the whole collection — the links and option keys have no
// index — so cross-space search disables them: at N-space scale they dominate
// the request (measured 1.3s of a 30s profile, 46% of all store-query CPU).
func (s *dsObjectStore) queryFromFulltextRecordsOpts(results []database.FulltextResult, params database.Filters, ftsSearch string, needed int, withInjections bool) []database.Record {
	pool := ftRerankPoolSize
	if pool > len(results) {
		pool = len(results)
	}
	seen := make(map[string]struct{})

	// head: resolve, collect related-object injections, re-rank by final score.
	// The head is always resolved in full — re-ranking can move any of its
	// objects into the requested page
	records, injectionGroups := s.resolveFulltextResults(results[:pool], params, ftsSearch, seen, withInjections, 0)
	if withInjections {
		// the injection budget is a request-independent constant: deriving it
		// from the requested page would make the injected set (and thus the
		// re-ranked head order) differ between offsets, breaking pagination
		// consistency
		records = s.injectRelatedObjects(injectionGroups, ftRerankPoolSize, params, seen, records)
	}
	if params.Order != nil {
		// stable: equal final scores keep their BM25 order, so the result is
		// deterministic across requests
		sort.SliceStable(records, func(i, j int) bool {
			return params.Order.Compare(records[i].Details, records[j].Details) == -1
		})
	}

	// tail: BM25 order as returned by the index, no re-ranking, no injections.
	// Already in final order, so resolve lazily: stop as soon as the requested
	// page is covered (needed == 0 means everything). Early exit yields a
	// prefix of the same sequence, so pagination consistency is preserved.
	if needed == 0 || len(records) < needed {
		tailMax := 0
		if needed > 0 {
			tailMax = needed - len(records)
		}
		tail, _ := s.resolveFulltextResults(results[pool:], params, ftsSearch, seen, false, tailMax)
		records = append(records, tail...)
	}
	return records
}

// resolveFulltextResults resolves fulltext candidates to filtered records,
// preserving the input order. seen is shared between calls to dedupe objects;
// related-object injection hits are collected only when collectInjections is
// set. maxRecords > 0 stops the resolution once that many records were
// produced, skipping the store reads for the remaining candidates.
func (s *dsObjectStore) resolveFulltextResults(results []database.FulltextResult, params database.Filters, ftsSearch string, seen map[string]struct{}, collectInjections bool, maxRecords int) ([]database.Record, map[domain.RelationKey][]injectionHit) {
	records := make([]database.Record, 0, len(results))
	resultObjectMap := seen
	injectionGroups := map[domain.RelationKey][]injectionHit{}

	for _, res := range results {
		if maxRecords > 0 && len(records) >= maxRecords {
			break
		}
		if sbt, err := typeprovider.SmartblockTypeFromID(res.Path.ObjectId); err == nil {
			if _, indexDetails, _ := sbt.Indexable(); !indexDetails && s.sourceService != nil {
				details, err := s.sourceService.DetailsFromIdBasedSource(domain.FullID{
					ObjectID: res.Path.ObjectId,
					SpaceID:  s.SpaceId(),
				})
				if err != nil {
					log.Errorf("QueryFromFulltext failed to GetDetailsFromIdBasedSource id: %s", res.Path.ObjectId)
					continue
				}
				details.SetString(bundle.RelationKeyId, res.Path.ObjectId)
				details.SetFloat64(bundle.RelationKey_score, res.Score)
				details.SetFloat64(bundle.RelationKey_final_score, database.ComputeFinalScore(res.Score, details, res.NameMatch))
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
			// a doc in the FT index without a store object is a stale-index
			// anomaly; the consistency check's orphan GC collects those
			log.With("id", res.Path.ObjectId).Debugf("fulltext candidate not found in store")
			continue
		}
		details, err := domain.NewDetailsFromAnyEnc(doc.Value())
		if err != nil {
			log.Errorf("QueryByIds failed to extract details: %s", res.Path.ObjectId)
			continue
		}
		details.SetFloat64(bundle.RelationKey_score, res.Score)
		details.SetFloat64(bundle.RelationKey_final_score, database.ComputeFinalScore(res.Score, details, res.NameMatch))

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

		if collectInjections {
			// gate on the budget-stable NameMatch, not on the representative
			// doc's relation key: the pluralName preference can switch the
			// representative when the budget grows, and a budget-dependent
			// injection set would perturb the re-ranked head between requests
			if relKey, ok := injectionRelationKey(details, res.NameMatch); ok {
				injectionGroups[relKey] = append(injectionGroups[relKey], injectionHit{
					id:      details.GetString(bundle.RelationKeyId),
					details: details,
					score:   res.Score,
				})
			}
		}
	}

	return records, injectionGroups
}

func injectionRelationKey(details *domain.Details, nameMatch bool) (domain.RelationKey, bool) {
	if !nameMatch {
		return "", false
	}
	if details.GetBool(bundle.RelationKeyIsDeleted) || details.GetBool(bundle.RelationKeyIsArchived) {
		return "", false
	}
	//nolint:gosec
	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
	switch layout {
	case model.ObjectType_basic, model.ObjectType_note, model.ObjectType_profile, model.ObjectType_todo, model.ObjectType_participant:
		return bundle.RelationKeyLinks, true
	case model.ObjectType_objectType:
		return bundle.RelationKeyType, true
	case model.ObjectType_relationOption:
		relKey := domain.RelationKey(details.GetString(bundle.RelationKeyRelationKey))
		if relKey == "" {
			return "", false
		}
		return relKey, true
	default:
		return "", false
	}
}

func (s *dsObjectStore) injectRelatedObjects(
	groups map[domain.RelationKey][]injectionHit,
	budget int,
	params database.Filters,
	seen map[string]struct{},
	records []database.Record,
) []database.Record {
	// process groups with the best-scoring hits first so that a limited budget
	// is spent deterministically (map iteration order is randomized)
	type scoredGroup struct {
		relKey domain.RelationKey
		hits   []injectionHit
		best   float64
	}
	sortedGroups := make([]scoredGroup, 0, len(groups))
	for relKey, hits := range groups {
		best := hits[0].score
		for _, hit := range hits[1:] {
			if hit.score > best {
				best = hit.score
			}
		}
		sortedGroups = append(sortedGroups, scoredGroup{relKey: relKey, hits: hits, best: best})
	}
	sort.Slice(sortedGroups, func(i, j int) bool {
		if sortedGroups[i].best != sortedGroups[j].best {
			return sortedGroups[i].best > sortedGroups[j].best
		}
		return sortedGroups[i].relKey < sortedGroups[j].relKey
	})

	for _, group := range sortedGroups {
		if budget <= 0 {
			break
		}
		relKey, hits := group.relKey, group.hits

		hitMap := make(map[string]injectionHit, len(hits))
		for i := range hits {
			existing, ok := hitMap[hits[i].id]
			if !ok || hits[i].score > existing.score {
				hitMap[hits[i].id] = hits[i]
			}
		}
		values := make([]domain.Value, 0, len(hitMap))
		for id := range hitMap {
			values = append(values, domain.String(id))
		}

		queryLimit := uint(budget) //nolint:gosec
		recs, err := s.queryAnyStore(s.componentCtx, database.FiltersAnd{database.FilterIn{Key: relKey, Value: values}, params.FilterObj}, params.Order, queryLimit, 0)
		if err != nil {
			log.Errorf("inject related objects by %s: %v", relKey, err)
			continue
		}

		for _, rec := range recs {
			if budget <= 0 {
				break
			}
			id := rec.Details.GetString(bundle.RelationKeyId)
			if _, ok := seen[id]; ok {
				continue
			}
			hit, ok := matchHit(rec.Details, relKey, hitMap)
			if !ok {
				continue
			}
			seen[id] = struct{}{}
			budget--
			records = append(records, makeInjectionRecord(rec, hit, string(relKey)))
		}
	}
	return records
}

func matchHit(details *domain.Details, relKey domain.RelationKey, hitMap map[string]injectionHit) (injectionHit, bool) {
	var (
		best  injectionHit
		found bool
	)
	for _, val := range details.WrapToStringList(relKey) {
		hit, ok := hitMap[val]
		if !ok {
			continue
		}
		if !found || hit.score > best.score || (hit.score == best.score && hit.id < best.id) {
			best = hit
			found = true
		}
	}
	return best, found
}

func makeInjectionRecord(source database.Record, hit injectionHit, relationKey string) database.Record {
	relDetails := pbtypes.StructFilterKeys(hit.details.ToProto(), []string{
		bundle.RelationKeyId.String(),
		bundle.RelationKeyName.String(),
		bundle.RelationKeyType.String(),
		bundle.RelationKeyResolvedLayout.String(),
		bundle.RelationKeyRelationOptionColor.String(),
	})
	detailsCopy := source.Details.Copy()
	detailsCopy.SetFloat64(bundle.RelationKey_score, hit.score)
	detailsCopy.SetFloat64(bundle.RelationKey_final_score, database.ComputeFinalScore(hit.score, detailsCopy, false))
	return database.Record{
		Details: detailsCopy,
		Meta: model.SearchMeta{
			RelationKey:     relationKey,
			RelationDetails: relDetails,
		},
	}
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
		return s.performFulltextQuery(q, filters)
	}
	return s.QueryRaw(filters, q.Limit, q.Offset)
}

// performFulltextQuery runs the fulltext pipeline, escalating the candidate
// budget until the requested page is filled. Store-level filters and the
// doc→object grouping run AFTER the fulltext search, so a fixed candidate cap
// can starve a page even though more matches exist; clients interpret a short
// page as "no more results", which must therefore be true. The common query is
// served by the first round; extra rounds only run for queries that
// demonstrably starved, and stop as soon as the index is exhausted (tantivy
// returned fewer docs than the budget) or the hard limit is reached.
func (s *dsObjectStore) performFulltextQuery(q database.Query, filters *database.Filters) ([]database.Record, error) {
	search := func(limit int) ([]*ftsearch.DocumentMatch, error) {
		if q.PrefixNameQuery {
			return s.fts.NamePrefixSearch(q.SpaceId, q.TextQuery, limit)
		}
		return s.fts.Search(q.SpaceId, q.TextQuery, limit, true)
	}

	needed := 0
	if q.Limit > 0 {
		needed = q.Offset + q.Limit
	}
	ftLimit := ftCandidatesLimit(q)
	for {
		fulltextResults, ftDocs, err := s.performFulltextSearch(!q.PrefixNameQuery, func() ([]*ftsearch.DocumentMatch, error) {
			return search(ftLimit)
		})
		if err != nil {
			return nil, fmt.Errorf("perform fulltext search: %w", err)
		}

		// Fallback to anystore search when Tantivy returns 0 results
		if len(fulltextResults) == 0 {
			return s.performFulltextFallback(q, filters)
		}

		records := s.queryFromFulltextRecords(fulltextResults, *filters, q.TextQuery, needed)

		pageFilled := needed == 0 || len(records) >= needed
		// the re-rank head must be the same object set for every request, or
		// offset pagination drifts: keep escalating until the full head pool
		// is materialized (or the index has no more docs to offer)
		headMaterialized := len(fulltextResults) >= ftRerankPoolSize
		indexExhausted := ftDocs < ftLimit
		if (pageFilled && headMaterialized) || indexExhausted || ftLimit >= ftCandidatesHardLimit {
			if !pageFilled && !indexExhausted {
				// the page stays underfilled although more matches exist:
				// the client will read it as the end of the results
				ftCandidatesTruncated.Add(1)
				log.With("limit", ftLimit).With("records", len(records)).With("needed", needed).
					Warn("fulltext page underfilled at the candidate hard limit")
			}
			ftHits.Add(1)
			return paginateRecords(records, q.Offset, q.Limit), nil
		}

		ftLimit *= 2
		if ftLimit > ftCandidatesHardLimit {
			ftLimit = ftCandidatesHardLimit
		}
	}
}

// performFulltextSearch groups raw doc matches per object and converts them to
// fulltext results; ftDocs is the raw doc count, used by the caller to detect
// whether the index was exhausted (ftDocs < requested budget). enforceMinScore
// drops near-zero-score results without highlights — it must be disabled for
// the prefix-name path, which runs without highlight generation and would lose
// legitimate low-scoring prefix matches.
func (s *dsObjectStore) performFulltextSearch(enforceMinScore bool, search func() (results []*ftsearch.DocumentMatch, err error)) ([]database.FulltextResult, int, error) {
	ftsResults, err := search()
	if err != nil {
		return nil, 0, fmt.Errorf("fullText search: %w", err)
	}
	results, err := GroupFulltextResults(ftsResults, enforceMinScore)
	if err != nil {
		return nil, 0, err
	}
	return results, len(ftsResults), nil
}

// GroupFulltextResults groups raw doc matches per object, selects each
// object's best (budget-stable) doc, orders objects deterministically and
// converts them to fulltext results. Exported for cross-space search, which
// runs one global tantivy query and groups per space before resolution; the
// caller must pass matches of a single space (object ids are only unique
// within one). enforceMinScore drops near-zero-score results without
// highlights — disable it whenever the matches were produced without
// highlight generation.
func GroupFulltextResults(ftsResults []*ftsearch.DocumentMatch, enforceMinScore bool) ([]database.FulltextResult, error) {
	var resultsByObjectId = make(map[string][]*ftsearch.DocumentMatch)
	for _, result := range ftsResults {
		path, err := domain.NewFromPath(result.ID)
		if err != nil {
			// a malformed doc id must not fail the whole search
			log.Errorf("fullText search: skip invalid doc id: %v", err)
			continue
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
			return cmp.Compare(b.Score, a.Score)
		})
	}

	// select only the best block/relation result for each object
	var objectResults = make([]*ftsearch.DocumentMatch, 0, len(resultsByObjectId))
	bestDocByChosenId := make(map[string]string, len(resultsByObjectId))
	for _, objectPerBlockResults := range resultsByObjectId {
		if len(objectPerBlockResults) == 0 {
			continue
		}
		chosen := preferPluralNameRelation(objectPerBlockResults)
		if chosen.Score != objectPerBlockResults[0].Score {
			// the object must be ORDERED by its best doc score: the preferred
			// pluralName doc can enter the candidate set at a lower BM25 rank
			// when the budget grows, and letting it lower the object's
			// representative score would reorder pages between requests
			clone := *chosen
			clone.Score = objectPerBlockResults[0].Score
			chosen = &clone
		}
		objectResults = append(objectResults, chosen)
		bestDocByChosenId[chosen.ID] = objectPerBlockResults[0].ID
	}

	sort.Slice(objectResults, func(i, j int) bool {
		// deterministic id tiebreak: the object order must be prefix-stable
		// across requests with different candidate budgets, or offset
		// pagination breaks at tie boundaries
		if objectResults[i].Score == objectResults[j].Score {
			return objectResults[i].ID < objectResults[j].ID
		}
		return objectResults[i].Score > objectResults[j].Score
	})

	var results = make([]database.FulltextResult, 0, len(objectResults))
	for _, docMatch := range objectResults {
		result, err := database.FTDocumentMatchToFulltextResult(docMatch)
		if err != nil {
			return nil, fmt.Errorf("fullText search: %w", err)
		}
		// the name boost must derive from the budget-stable BEST doc, not from
		// the representative doc: the pluralName preference can switch the
		// representative when the budget grows, and a budget-dependent boost
		// would reorder the re-ranked head between page requests
		result.NameMatch = isNameDocId(bestDocByChosenId[docMatch.ID])
		if enforceMinScore && result.Score < minFulltextScore && len(result.HighlightRanges) == 0 {
			continue
		}
		results = append(results, result)

	}

	return results, nil
}

// isNameDocId reports whether the doc id is the object's name or pluralName
// relation doc.
func isNameDocId(docId string) bool {
	return strings.HasSuffix(docId, nameId) || strings.HasSuffix(docId, pluralNameId)
}

func (s *dsObjectStore) performFulltextFallback(q database.Query, filters *database.Filters) ([]database.Record, error) {
	// Build text search filter using FiltersOr with FilterLike
	textFilter := database.FiltersOr{
		database.FilterLike{Key: bundle.RelationKeyName, Value: q.TextQuery},
		database.FilterLike{Key: bundle.RelationKeySnippet, Value: q.TextQuery},
		database.FilterLike{Key: bundle.RelationKeyPluralName, Value: q.TextQuery},
	}

	// Combine with original filters if present
	var combinedFilter database.Filter
	if filters != nil && filters.FilterObj != nil {
		combinedFilter = database.FiltersAnd{textFilter, filters.FilterObj}
	} else {
		combinedFilter = textFilter
	}

	// Get order from filters
	var order database.Order
	if filters != nil {
		order = filters.Order
	}

	// Query anystore
	records, err := s.queryAnyStore(s.componentCtx, combinedFilter, order, uint(q.Limit), uint(q.Offset))
	if err != nil {
		return nil, fmt.Errorf("fulltext fallback query: %w", err)
	}

	// Run diagnostic check
	s.logFallbackDiagnostics(q.SpaceId, q.TextQuery, records)

	return records, nil
}

func isFulltextIndexable(record database.Record) bool {
	if sbt, err := typeprovider.SmartblockTypeFromID(record.Details.GetString(bundle.RelationKeyId)); err == nil {
		ft, _, _ := sbt.Indexable()
		return ft
	}

	if record.Details.GetBool(bundle.RelationKeyIsDeleted) || record.Details.GetBool(bundle.RelationKeyIsArchived) || record.Details.GetBool(bundle.RelationKeyIsHidden) {
		return false
	}
	return false
}

func (s *dsObjectStore) logFallbackDiagnostics(spaceId string, textQuery string, records []database.Record) {
	// Sample up to 10 objects
	records = slices.DeleteFunc(records, func(r database.Record) bool {
		return !isFulltextIndexable(r)
	})

	sampleSize := len(records)
	if sampleSize > 10 {
		sampleSize = 10
	}

	if sampleSize == 0 {
		log.With("spaceId", spaceId).
			With("queryLen", len(textQuery)).
			With("tantivyResults", 0).
			With("fallbackResults", 0).
			Debug("fulltext fallback: no results from either source")
		return
	}

	// Check if specific relation documents exist in Tantivy
	// Document IDs in Tantivy: "$objectId/r/name", "$objectId/r/snippet"

	// Track last modified dates for diagnostic purposes
	var oldestLastModified, newestLastModified int64
	var nameMissing, noDocs int
	for i := 0; i < sampleSize; i++ {
		objectId := records[i].Details.GetString(bundle.RelationKeyId)
		lastModified := records[i].Details.GetInt64(bundle.RelationKeyLastModifiedDate)
		layout := model.ObjectTypeLayout(records[i].Details.GetInt64(bundle.RelationKeyResolvedLayout))

		// Track oldest and newest last modified dates
		if i == 0 || lastModified < oldestLastModified {
			oldestLastModified = lastModified
		}
		if i == 0 || lastModified > newestLastModified {
			newestLastModified = lastModified
		}

		// Check $objectId/r/name
		hasNameDoc, totalDocs := s.checkDocExistsInTantivy(objectId)
		if layout != model.ObjectType_note && !hasNameDoc {
			nameMissing++
		}
		if totalDocs == 0 {
			noDocs++
		}
	}

	ftMisses.Add(1)
	ftReport := s.fts.ConsistencyReport()
	// Log diagnostic info without exposing user data
	log.With("spaceId", spaceId).
		With("queryLen", len(textQuery)).
		With("tantivyResults", 0).
		With("fallbackResults", len(records)).
		With("sampleSize", sampleSize).
		With("nameDocMissing", nameMissing).
		With("noDocs", noDocs).
		With("oldestLastModified", oldestLastModified).
		With("newestLastModified", newestLastModified).
		With("ftHitsCnt", ftHits.Load()).
		With("ftTruncatedCnt", ftCandidatesTruncated.Load()).
		With("ftMissesCnt", ftMisses.Load()).
		With("ftOldestSegment", ftReport.OldestSegmentModTime.Unix()).
		With("ftNewestSegment", ftReport.NewestSegmentModTime.Unix()).
		With("ftMetaModTime", ftReport.MetaJsonModTime.Unix()).
		With("ftOk", ftReport.IsOk()).
		With("ftReportTime", ftReport.ReportTime.Unix()).
		Warn("fulltext search fallback triggered")
}

func (s *dsObjectStore) checkDocExistsInTantivy(objectId string) (hasName bool, total int) {
	ids, err := s.fts.ListByIdPrefix(objectId)
	if err != nil {
		log.With("error", err).Debug("fulltext fallback diagnostic: list error")
		return
	}
	for _, id := range ids {
		if strings.HasSuffix(id, "/r/name") {
			hasName = true
		}
		total++
	}
	return
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

// QueryIterateRaw streams every record matching the precompiled filters
// without materializing the result set (no implicit filters, no sorts). The
// callee owns each row's details (they are deep-copied off the iterator's
// buffers and may be retained); the iterator itself holds no row past its
// callback. proc returning an error stops the iteration.
func (s *dsObjectStore) QueryIterateRaw(f *database.Filters, proc func(details *domain.Details) error) error {
	if f == nil || f.FilterObj == nil {
		return fmt.Errorf("filter cannot be nil or uninitialized")
	}
	iter, err := s.objects.Find(f.FilterObj.AnystoreFilter()).Iter(s.componentCtx)
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return fmt.Errorf("get doc: %w", err)
		}
		details, err := domain.NewDetailsFromAnyEnc(doc.Value())
		if err != nil {
			return fmt.Errorf("json to proto: %w", err)
		}
		if err = proc(details); err != nil {
			return err
		}
	}
	if err = iter.Err(); err != nil {
		return fmt.Errorf("iterate: %w", err)
	}
	return nil
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
