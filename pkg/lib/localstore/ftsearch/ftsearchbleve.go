package ftsearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch/analyzers"
	_ "github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch/jsonhighlighter"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName  = "fts"
	ftsDir = "fts"
	ftsVer = "7"

	fieldTitle        = "Title"
	fieldTitleCh      = "TitleCh"
	fieldText         = "Text"
	fieldTextCh       = "TextCh"
	fieldSpace        = "SpaceID"
	fieldTitleNoTerms = "TitleNoTerms"
	fieldTextNoTerms  = "TextNoTerms"
	fieldId           = "Id"
	fieldIdRaw        = "IdRaw"
	score             = "score"
	highlights        = "highlights"
	tokenizerId       = "SimpleIdTokenizer"
)

var log = logging.Logger("ftsearch")

type SearchDoc struct {
	//nolint:all
	Id           string
	SpaceID      string
	Title        string
	TitleNoTerms string
	Text         string
	TextNoTerms  string
}

func New() FTSearch {
	return new(ftSearch)
}

type FTSearch interface {
	app.ComponentRunnable
	Index(d SearchDoc) (err error)
	NewAutoBatcher(maxDocs int, maxDocsSize uint64) AutoBatcher
	BatchIndex(ctx context.Context, docs []SearchDoc, deletedDocs []string) (err error)
	BatchDeleteObjects(ids []string) (err error)
	Search(spaceIds []string, highlightFormatter HighlightFormatter, query string) (results search.DocumentMatchCollection, err error)
	Iterate(objectId string, fields []string, shouldContinue func(doc *SearchDoc) bool) (err error)
	DeleteObject(id string) error
	DocCount() (uint64, error)
}

type ftSearch struct {
	rootPath string
	ftsPath  string
	index    bleve.Index
}

func (f *ftSearch) Init(a *app.App) (err error) {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	f.rootPath = filepath.Join(repoPath, ftsDir)
	f.ftsPath = filepath.Join(repoPath, ftsDir, ftsVer)
	return err
}

func (f *ftSearch) Name() (name string) {
	return CName
}

func (f *ftSearch) Run(context.Context) (err error) {
	index, err := bleve.Open(f.ftsPath)
	if err == bleve.ErrorIndexPathDoesNotExist || err == bleve.ErrorIndexMetaMissing {
		if index, err = bleve.New(f.ftsPath, makeMapping()); err != nil {
			return
		}
		f.cleanUpOldIndexes()
	} else if err != nil {
		return
	}
	f.index = index
	f.cleanTantivy()
	return nil
}

func (f *ftSearch) cleanUpOldIndexes() {
	if strings.HasSuffix(f.rootPath, ftsDir) {
		dirs, err := os.ReadDir(f.rootPath)
		if err == nil {
			// cleanup old index versions
			for _, dir := range dirs {
				if dir.Name() != ftsVer {
					_ = os.RemoveAll(filepath.Join(f.rootPath, dir.Name()))
				}
			}
		}
	}
}

func (f *ftSearch) cleanTantivy() {
	_ = os.RemoveAll(filepath.Join(f.rootPath, ftsDir2))
}

func (f *ftSearch) Index(doc SearchDoc) (err error) {
	metrics.ObjectFTUpdatedCounter.Inc()
	doc.TitleNoTerms = doc.Title
	doc.TextNoTerms = doc.Text
	return f.index.Index(doc.Id, doc)
}

func (f *ftSearch) BatchDo(proc func(b *bleve.Batch) error) (err error) {
	batch := f.index.NewBatch()

	err = proc(batch)
	if err != nil {
		batch.Reset()
		return err
	}

	return f.index.Batch(batch)
}

func (f *ftSearch) BatchIndex(ctx context.Context, docs []SearchDoc, deletedDocs []string) (err error) {
	if len(docs) == 0 {
		return nil
	}
	metrics.ObjectFTUpdatedCounter.Add(float64(len(docs)))
	batch := f.index.NewBatch()
	start := time.Now()
	defer func() {
		spentMs := time.Since(start).Milliseconds()
		l := log.With("objects", len(docs)).With("total", time.Since(start).Milliseconds())
		if spentMs > 1000 {
			l.Warnf("ft index took too long")
		} else {
			l.Debugf("ft index done")
		}
	}()
	for _, doc := range docs {
		if ctx.Err() == context.Canceled {
			return ctx.Err()
		}
		doc.TitleNoTerms = doc.Title
		doc.TextNoTerms = doc.Text
		if err := batch.Index(doc.Id, doc); err != nil {
			return fmt.Errorf("failed to index document %s: %w", doc.Id, err)
		}
	}
	for _, docId := range deletedDocs {
		if ctx.Err() == context.Canceled {
			return ctx.Err()
		}
		batch.Delete(docId)
	}
	return f.index.Batch(batch)
}

func (f *ftSearch) batchDeleteDocs(docIds []string) (err error) {
	if len(docIds) == 0 {
		return nil
	}
	batch := f.index.NewBatch()
	start := time.Now()
	defer func() {
		spentMs := time.Since(start).Milliseconds()
		l := log.With("objects", len(docIds)).With("total", time.Since(start).Milliseconds())
		if spentMs > 1000 {
			l.Warnf("ft delete took too long")
		} else {
			l.Debugf("ft delete done")
		}
	}()

	for _, docId := range docIds {
		batch.Delete(docId)
	}
	return f.index.Batch(batch)
}

func (f *ftSearch) BatchDeleteObjects(objectIds []string) (err error) {
	if len(objectIds) == 0 {
		return nil
	}

	var docIds []string
	for _, id := range objectIds {
		ids, err := f.listIndexedIds(id)
		if err != nil {
			log.With("id", id).Errorf("failed to get doc ids for object id: %s", err)
		}
		docIds = append(docIds, ids...)

	}
	return f.batchDeleteDocs(docIds)
}

type HighlightFormatter string

const (
	HtmlHighlightFormatter    HighlightFormatter = "html"
	JSONHighlightFormatter    HighlightFormatter = "json"
	DefaultHighlightFormatter                    = JSONHighlightFormatter
)

type IndexedDoc struct {
	FullDocId string
	Text      string
	Title     string
}

func (f *ftSearch) Iterate(objectId string, fields []string, shouldContinue func(doc *SearchDoc) bool) (err error) {
	prefixQuery := bleve.NewPrefixQuery(objectId + "/")
	prefixQuery.SetField("_id")

	searchRequest := bleve.NewSearchRequest(prefixQuery)

	searchRequest.Size = 10000
	searchRequest.Explain = false
	searchRequest.Fields = fields
	searchResult, err := f.index.Search(searchRequest)
	if err != nil {
		return
	}

	var text, title, spaceId string
	for _, hit := range searchResult.Hits {
		text, title, spaceId = "", "", ""
		if hit.Fields != nil {
			if hit.Fields["Text"] != nil {
				text, _ = hit.Fields["Text"].(string)
			}
			if hit.Fields["Title"] != nil {
				title, _ = hit.Fields["Title"].(string)
			}
			if hit.Fields["SpaceID"] != nil {
				spaceId, _ = hit.Fields["SpaceID"].(string)
			}
		}

		if !shouldContinue(&SearchDoc{
			Id:      hit.ID,
			Text:    text,
			Title:   title,
			SpaceID: spaceId,
		}) {
			break
		}
	}
	return nil
}

func (f *ftSearch) listIndexedIds(objectId string) (ids []string, err error) {
	prefixQuery := bleve.NewPrefixQuery(objectId + "/")
	prefixQuery.SetField("_id")

	searchRequest := bleve.NewSearchRequest(prefixQuery)

	searchRequest.Size = 10000
	searchRequest.Explain = false
	searchRequest.Fields = []string{"_id", "Text"}

	searchResult, err := f.index.Search(searchRequest)
	if err != nil {
		return
	}
	for _, hit := range searchResult.Hits {
		ids = append(ids, hit.ID)
	}
	return ids, nil
}

func (f *ftSearch) Search(spaceIds []string, highlightFormatter HighlightFormatter, qry string) (results search.DocumentMatchCollection, err error) {
	qry = strings.ToLower(qry)
	qry = strings.TrimSpace(qry)
	terms := f.getTerms(qry)

	queries := append(
		getFullQueries(qry),
		bleve.NewMatchQuery(qry),
	)

	if len(terms) > 0 {
		queries = append(
			queries,
			getAllWordsFromQueryConsequently(terms, fieldTitleNoTerms),
			getAllWordsFromQueryConsequently(terms, fieldTextNoTerms),
		)
	}

	return f.doSearch(spaceIds, highlightFormatter, queries)
}

func (f *ftSearch) getTerms(qry string) []string {
	terms := strings.Split(qry, " ")
	termsFiltered := terms[:0]

	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term != "" {
			termsFiltered = append(termsFiltered, term)
		}
	}
	terms = termsFiltered
	return terms
}

func (f *ftSearch) doSearch(spaceIds []string, highlightFormatter HighlightFormatter, queries []query.Query) (results search.DocumentMatchCollection, err error) {
	var rootQuery query.Query = bleve.NewDisjunctionQuery(queries...)
	if len(spaceIds) != 0 {
		var spaceQueries []query.Query
		for _, spaceId := range spaceIds {
			if spaceId == "" {
				continue
			}
			spaceQuery := bleve.NewMatchQuery(spaceId)
			spaceQuery.SetField(fieldSpace)
			spaceQueries = append(spaceQueries, spaceQuery)
		}
		spaceIdsQuery := bleve.NewDisjunctionQuery(spaceQueries...)
		rootQuery = bleve.NewConjunctionQuery(rootQuery, spaceIdsQuery)
	}

	searchRequest := bleve.NewSearchRequest(rootQuery)
	searchRequest.Highlight = bleve.NewHighlightWithStyle(string(highlightFormatter))
	searchRequest.Highlight.Fields = []string{fieldText}

	searchRequest.Size = 100
	searchRequest.Explain = true
	searchResult, err := f.index.Search(searchRequest)
	if err != nil {
		return
	}
	return searchResult.Hits, nil
}

func (f *ftSearch) Has(id string) (exists bool, err error) {
	d, err := f.index.Document(id)
	if err != nil {
		return false, err
	}
	return d != nil, nil
}

func (f *ftSearch) DeleteObject(objectId string) (err error) {
	return f.BatchDeleteObjects([]string{objectId})
}

func (f *ftSearch) DocCount() (uint64, error) {
	return f.index.DocCount()
}

func (f *ftSearch) Close(ctx context.Context) error {
	if f.index != nil {
		return f.index.Close()
	}
	return nil
}

func makeMapping() mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()

	addNoTermsMapping(indexMapping)
	addDefaultMapping(indexMapping)

	return indexMapping
}

func addDefaultMapping(indexMapping *mapping.IndexMappingImpl) {
	fields := []string{
		fieldTitle,
		fieldText,
	}

	addMappings(indexMapping, fields, getStandardMapping())
}

func addNoTermsMapping(indexMapping *mapping.IndexMappingImpl) {
	err := analyzers.AddNoTermsAnalyzer(indexMapping)
	if err != nil {
		log.Warnf("Failed to add no terms analyzer")
	}

	keywordMapping := analyzers.GetNoTermsFieldMapping()

	fields := []string{
		fieldTitleNoTerms,
		fieldTextNoTerms,
		fieldId,
	}
	addMappings(indexMapping, fields, keywordMapping)
}

func addMappings(indexMapping *mapping.IndexMappingImpl, fields []string, mappings ...*mapping.FieldMapping) {
	for _, m := range fields {
		indexMapping.DefaultMapping.AddFieldMappingsAt(m, mappings...)
	}
}

func getStandardMapping() *mapping.FieldMapping {
	standardMapping := bleve.NewTextFieldMapping()
	standardMapping.Analyzer = standard.Name
	return standardMapping
}

func getAllWordsFromQueryConsequently(terms []string, field string) query.Query {
	terms = lo.Map(
		terms,
		func(item string, index int) string { return regexp.QuoteMeta(item) },
	)
	qry := strings.Join(terms, ".*")
	regexpQuery := bleve.NewRegexpQuery(".*" + qry + ".*")
	regexpQuery.SetField(field)
	return regexpQuery
}

func getFullQueries(qry string) []query.Query {
	var fullQueries = make([]query.Query, 0, 2)

	if len(qry) > 5 {
		fullQueries = append(fullQueries, getIDMatchQuery(qry))
	}
	fullQueries = append(fullQueries, bleve.NewPrefixQuery(qry))

	return fullQueries
}

func getIDMatchQuery(qry string) *query.DocIDQuery {
	docIDQuery := bleve.NewDocIDQuery([]string{qry})
	docIDQuery.SetBoost(30)
	return docIDQuery
}
