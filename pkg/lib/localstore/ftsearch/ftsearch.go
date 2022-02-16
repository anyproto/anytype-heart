package ftsearch

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/single"
	"os"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
)

const (
	CName  = "fts"
	ftsDir = "fts"
	ftsVer = "1"
)

type SearchDoc struct {
	Id           string
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
	f.enStopWordsMap, _ = en.TokenMapConstructor(nil, nil)
	return nil
}

func (f *ftSearch) Name() (name string) {
	return CName
}

func (f *ftSearch) Run() (err error) {
	de, e := os.ReadDir(f.rootPath)
	if e == nil {
		// cleanup old index versions
		for _, d := range de {
			if d.Name() != ftsVer {
				os.RemoveAll(filepath.Join(f.rootPath, d.Name()))
			}
		}
	}

	f.index, err = bleve.Open(f.ftsPath)
	if err == bleve.ErrorIndexPathDoesNotExist || err == bleve.ErrorIndexMetaMissing {
		if f.index, err = bleve.New(f.ftsPath, f.makeMapping()); err != nil {
			return
		}
	} else if err != nil {
		return
	}
	return nil
}

func (f *ftSearch) makeMapping() mapping.IndexMapping {
	mapping := bleve.NewIndexMapping()

	keywordMapping := bleve.NewTextFieldMapping()
	keywordMapping.Analyzer = "noTerms"

	mapping.DefaultMapping.AddFieldMappingsAt("TitleNoTerms", keywordMapping)
	mapping.DefaultMapping.AddFieldMappingsAt("Id", keywordMapping)

	standardMapping := bleve.NewTextFieldMapping()
	standardMapping.Analyzer = standard.Name
	mapping.DefaultMapping.AddFieldMappingsAt("Title", standardMapping)
	mapping.DefaultMapping.AddFieldMappingsAt("Text", standardMapping)

	mapping.AddCustomAnalyzer("noTerms",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": single.Name,
			"token_filters": []string{
				lowercase.Name,
			},
		})
	return mapping
}

func (f *ftSearch) Index(d SearchDoc) (err error) {
	metrics.ObjectFTUpdatedCounter.Inc()
	d.TitleNoTerms = d.Title
	return f.index.Index(d.Id, d)
}

func (f *ftSearch) Search(text string) (results []string, err error) {
	text = strings.ToLower(strings.TrimSpace(text))
	terms := append([]string{text}, strings.Split(text, " ")...)
	termsFiltered := terms[:0]

	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t != "" && !f.enStopWordsMap[t] {
			termsFiltered = append(termsFiltered, t)
		}
	}
	terms = termsFiltered

	var exactQueries = make([]query.Query, 0, 4)
	// id match
	if len(text) > 5 {
		im := bleve.NewDocIDQuery([]string{text})
		im.SetBoost(30)
		exactQueries = append(exactQueries, im)
	}
	// title prefix
	tp := bleve.NewPrefixQuery(text)
	tp.SetField("TitleNoTerms")
	tp.SetBoost(40)
	exactQueries = append(exactQueries, tp)

	// title substr
	tss := bleve.NewWildcardQuery("*" + strings.ReplaceAll(text, "*", `\*`) + "*")
	tss.SetField("TitleNoTerms")
	tss.SetBoost(8)
	exactQueries = append(exactQueries, tss)

	var notExactQueriesGroup = make([]query.Query, 0, 5)
	for i, t := range terms {
		// fulltext queries
		var notExactQueries = make([]query.Query, 0, 3)
		tp = bleve.NewPrefixQuery(t)
		tp.SetField("Title")
		if i == 0 {
			tp.SetBoost(8)
		}
		notExactQueries = append(notExactQueries, tp)

		// title match
		tm := bleve.NewMatchQuery(t)
		tm.SetFuzziness(1)
		tm.SetField("Title")
		if i == 0 {
			tm.SetBoost(7)
		}
		notExactQueries = append(notExactQueries, tm)

		// text match
		txtm := bleve.NewMatchQuery(t)
		txtm.SetFuzziness(0)
		txtm.SetField("Text")
		if i == 0 {
			txtm.SetBoost(2)
		}
		notExactQueries = append(notExactQueries, txtm)
		notExactQueriesGroup = append(notExactQueriesGroup, bleve.NewDisjunctionQuery(notExactQueries...))
	}

	//exactQueries = []query.Query{bleve.NewDisjunctionQuery(notExactQueriesGroup...)}
	exactQueries = append(exactQueries, bleve.NewConjunctionQuery(notExactQueriesGroup...))

	sr := bleve.NewSearchRequest(bleve.NewDisjunctionQuery(exactQueries...))
	sr.Size = 100
	sr.Explain = true
	res, err := f.index.Search(sr)
	//fmt.Println(res.String())
	if err != nil {
		return
	}
	for _, r := range res.Hits {
		results = append(results, r.ID)
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

func (f *ftSearch) Close() error {
	if f.index != nil {
		f.index.Close()
	}
	return nil
}
