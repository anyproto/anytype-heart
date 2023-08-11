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
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch/analyzers"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName  = "fts"
	ftsDir = "fts"
	ftsVer = "2"

	fieldTitle        = "Title"
	fieldText         = "Text"
	fieldTitleNoTerms = "TitleNoTerms"
	fieldTextNoTerms  = "TextNoTerms"
	fieldID           = "Id"
)

var log = logging.Logger("ftsearch")

type SearchDoc struct {
	//nolint:all
	Id           string
	Title        string
	TitleNoTerms string
	Text         string
	TextNoTerms  string
}

func New() FTSearch {
	return &ftSearch{}
}

type FTSearch interface {
	app.ComponentRunnable
	Index(d SearchDoc) (err error)
	BatchIndex(docs []SearchDoc) (err error)
	Search(query string) (results []string, err error)
	Has(id string) (exists bool, err error)
	Delete(id string) error
	DocCount() (uint64, error)
}

type ftSearch struct {
	rootPath       string
	ftsPath        string
	index          bleve.Index
	enStopWordsMap map[string]bool
}

func (f *ftSearch) Init(a *app.App) (err error) {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	f.rootPath = filepath.Join(repoPath, ftsDir)
	f.ftsPath = filepath.Join(repoPath, ftsDir, ftsVer)
	f.enStopWordsMap, err = en.TokenMapConstructor(nil, nil)
	return err
}

func (f *ftSearch) Name() (name string) {
	return CName
}

func (f *ftSearch) Run(context.Context) (err error) {
	f.index, err = bleve.Open(f.ftsPath)
	if err == bleve.ErrorIndexPathDoesNotExist || err == bleve.ErrorIndexMetaMissing {
		if f.index, err = bleve.New(f.ftsPath, makeMapping()); err != nil {
			return
		}
		f.cleanUpOldIndexes()
	} else if err != nil {
		return
	}
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

func (f *ftSearch) Index(doc SearchDoc) (err error) {
	metrics.ObjectFTUpdatedCounter.Inc()
	doc.TitleNoTerms = doc.Title
	doc.TextNoTerms = doc.Text
	return f.index.Index(doc.Id, doc)
}

func (f *ftSearch) BatchIndex(docs []SearchDoc) (err error) {
	if len(docs) == 0 {
		return nil
	}
	metrics.ObjectFTUpdatedCounter.Add(float64(len(docs)))
	b := f.index.NewBatch()
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
		doc.TitleNoTerms = doc.Title
		doc.TextNoTerms = doc.Text
		if err := b.Index(doc.Id, doc); err != nil {
			return fmt.Errorf("failed to index document %s: %w", doc.Id, err)
		}
	}
	return f.index.Batch(b)
}

func (f *ftSearch) Search(qry string) (results []string, err error) {
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

	return f.doSearch(queries)
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

func (f *ftSearch) doSearch(queries []query.Query) (results []string, err error) {
	searchRequest := bleve.NewSearchRequest(bleve.NewDisjunctionQuery(queries...))
	searchRequest.Size = 100
	searchRequest.Explain = true
	searchResult, err := f.index.Search(searchRequest)

	if err != nil {
		return
	}
	for _, result := range searchResult.Hits {
		results = append(results, result.ID)
	}
	return
}

func (f *ftSearch) Has(id string) (exists bool, err error) {
	d, err := f.index.Document(id)
	if err != nil {
		return false, err
	}
	return d != nil, nil
}

func (f *ftSearch) Delete(id string) (err error) {
	if f.index == nil {
		fmt.Println("index is not initialized")
	}
	return f.index.Delete(id)
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
		fieldID,
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
