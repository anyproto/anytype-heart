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
	fieldID           = "Id"
)

var analyzerName = standard.Name

var log = logging.Logger("ftsearch")

type SearchDoc struct {
	//nolint:all
	Id           string
	SpaceID      string
	Title        string
	TitleNoTerms string
	Text         string
}

func New() FTSearch {
	return &ftSearch{}
}

type FTSearch interface {
	app.ComponentRunnable
	Index(d SearchDoc) (err error)
	BatchIndex(docs []SearchDoc) (err error)
	Search(spaceID, query string) (results []string, err error)
	Has(id string) (exists bool, err error)
	Delete(id string) error
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
	f.index, err = bleve.Open(f.ftsPath)
	if err == bleve.ErrorIndexPathDoesNotExist || err == bleve.ErrorIndexMetaMissing {
		if f.index, err = bleve.New(f.ftsPath, makeMapping(analyzerName)); err != nil {
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
	//doc.TextNoTerms = doc.Text
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
		//doc.TextNoTerms = doc.Text
		if err := b.Index(doc.Id, doc); err != nil {
			return fmt.Errorf("failed to index document %s: %w", doc.Id, err)
		}
	}
	return f.index.Batch(b)
}

func (f *ftSearch) Search(spaceID, qry string) (results []string, err error) {
	qry = strings.ToLower(qry)
	qry = strings.TrimSpace(qry)
	terms := f.getTerms(qry)

	prefixQueriesTitle, prefixQueriesText, queries := f.exactQueries(qry, terms)

	results, err = f.doSearch(spaceID, queries)

	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		looseQuery := f.looseQueries(qry, prefixQueriesTitle, prefixQueriesText)
		results, err = f.doSearch(spaceID, looseQuery)
	}
	return results, err
}

func (f *ftSearch) looseQueries(qry string, prefixQueriesTitle []query.Query, prefixQueriesText []query.Query) []query.Query {
	var looseQuery []query.Query
	orMatchTitle := bleve.NewMatchQuery(qry)
	orMatchTitle.SetField(fieldTitle)
	orMatchTitle.SetFuzziness(0)
	orMatchTitle.SetOperator(query.MatchQueryOperatorOr)
	orMatchTitle.SetBoost(3)
	orMatchText := bleve.NewMatchQuery(qry)
	orMatchText.SetField(fieldText)
	orMatchText.SetFuzziness(0)
	orMatchText.SetOperator(query.MatchQueryOperatorOr)
	orMatchText.SetBoost(2)

	prefixQuerySomeTitle := bleve.NewDisjunctionQuery(prefixQueriesTitle...)
	prefixQuerySomeTitle.SetBoost(1)

	prefixQuerySomeText := bleve.NewDisjunctionQuery(prefixQueriesText...)
	prefixQuerySomeText.SetBoost(1)
	looseQuery = append(
		looseQuery,
		orMatchTitle,
		orMatchText,
		prefixQuerySomeTitle,
		prefixQuerySomeText,
	)
	return looseQuery
}

func (f *ftSearch) exactQueries(qry string, terms []string) ([]query.Query, []query.Query, []query.Query) {
	exactQueries := []query.Query{getIDMatchQuery(qry)}

	if len(terms) > 0 {
		exactQueries = append(
			exactQueries,
			getAllWordsFromQueryConsequently(terms, fieldTitleNoTerms),
		)
	}

	titleMatchPhrase := bleve.NewMatchPhraseQuery(qry)
	titleMatchPhrase.SetField(fieldTitle)
	titleMatchPhrase.SetFuzziness(0)
	titleMatchPhrase.SetBoost(100)

	textMatchPhrase := bleve.NewMatchPhraseQuery(qry)
	textMatchPhrase.SetField(fieldText)
	textMatchPhrase.SetFuzziness(0)
	textMatchPhrase.SetBoost(99)

	titleMatch := bleve.NewMatchQuery(qry)
	titleMatch.SetField(fieldTitle)
	titleMatch.SetFuzziness(0)
	titleMatch.SetOperator(query.MatchQueryOperatorAnd)
	titleMatch.SetBoost(20)
	textMatch := bleve.NewMatchQuery(qry)
	textMatch.SetField(fieldText)
	textMatch.SetFuzziness(0)
	textMatch.SetOperator(query.MatchQueryOperatorAnd)
	textMatch.SetBoost(19)

	var prefixQueriesTitle []query.Query

	for _, term := range terms {
		newQry := bleve.NewPrefixQuery(term)
		newQry.SetField(fieldTitle)
		prefixQueriesTitle = append(prefixQueriesTitle, newQry)
	}
	prefixQueryAllTitle := bleve.NewConjunctionQuery(prefixQueriesTitle...)

	prefixQueriesText := make([]query.Query, 0, 2)
	for _, term := range terms {
		newQry := bleve.NewPrefixQuery(term)
		newQry.SetField(fieldText)
		prefixQueriesText = append(prefixQueriesText, newQry)
	}
	prefixQueryAllText := bleve.NewConjunctionQuery(prefixQueriesText...)
	prefixQueryAllText.SetBoost(50)

	exactQueries = append(
		exactQueries,
		titleMatchPhrase,
		textMatchPhrase,
		titleMatch,
		textMatch,
		prefixQueryAllTitle,
		prefixQueryAllText,
	)
	return prefixQueriesTitle, prefixQueriesText, exactQueries
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

func (f *ftSearch) doSearch(spaceID string, queries []query.Query) (results []string, err error) {
	var rootQuery query.Query = bleve.NewDisjunctionQuery(queries...)
	if spaceID != "" {
		spaceQuery := bleve.NewMatchQuery(spaceID)
		spaceQuery.SetField("SpaceID")
		rootQuery = bleve.NewConjunctionQuery(rootQuery, spaceQuery)
	}

	searchRequest := bleve.NewSearchRequest(rootQuery)
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

func makeMapping(mapping string) mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()

	addNoTermsMapping(indexMapping)
	addDefaultMapping(indexMapping, mapping)

	return indexMapping
}

func addDefaultMapping(indexMapping *mapping.IndexMappingImpl, mapping string) {
	fields := []string{
		fieldTitle,
		fieldText,
	}

	addMappings(indexMapping, fields, getStandardMapping(mapping))
}

func addNoTermsMapping(indexMapping *mapping.IndexMappingImpl) {
	err := analyzers.AddNoTermsAnalyzer(indexMapping)
	if err != nil {
		log.Warnf("Failed to add no terms analyzer")
	}

	keywordMapping := analyzers.GetNoTermsFieldMapping()

	fields := []string{
		fieldTitleNoTerms,
		fieldID,
	}
	addMappings(indexMapping, fields, keywordMapping)
}

func addMappings(indexMapping *mapping.IndexMappingImpl, fields []string, mappings ...*mapping.FieldMapping) {
	for _, m := range fields {
		indexMapping.DefaultMapping.AddFieldMappingsAt(m, mappings...)
	}
}

func getStandardMapping(mapping string) *mapping.FieldMapping {
	standardMapping := bleve.NewTextFieldMapping()
	standardMapping.Analyzer = mapping
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

func getIDMatchQuery(qry string) *query.DocIDQuery {
	docIDQuery := bleve.NewDocIDQuery([]string{qry})
	docIDQuery.SetBoost(100)
	return docIDQuery
}
