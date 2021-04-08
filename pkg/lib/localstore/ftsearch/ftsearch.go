package ftsearch

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/search/query"
)

const (
	CName  = "fts"
	ftsDir = "fts"
)

type SearchDoc struct {
	Id    string
	Title string
	Text  string
}

func New() FTSearch {
	return &ftSearch{}
}

type FTSearch interface {
	app.Component
	Index(d SearchDoc) (err error)
	Search(query string) (results []string, err error)
	Delete(id string) error
	DocCount() (uint64, error)
}

type ftSearch struct {
	path  string
	index bleve.Index
}

func (f *ftSearch) Init(a *app.App) (err error) {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	f.path = filepath.Join(repoPath, ftsDir)
	if err = f.init(); err != nil {
		return err
	}

	return nil
}

func (f *ftSearch) Name() (name string) {
	return CName
}

func (f *ftSearch) init() (err error) {
	f.index, err = bleve.Open(f.path)
	if err == bleve.ErrorIndexPathDoesNotExist || err == bleve.ErrorIndexMetaMissing {
		mapping := bleve.NewIndexMapping()
		if f.index, err = bleve.New(f.path, mapping); err != nil {
			return
		}
	} else if err != nil {
		return
	}
	return
}

func (f *ftSearch) Index(d SearchDoc) (err error) {
	return f.index.Index(d.Id, d)
}

func (f *ftSearch) Search(text string) (results []string, err error) {
	text = strings.ToLower(strings.TrimSpace(text))
	var queries = make([]query.Query, 0, 4)

	// id match
	if len(text) > 10 {
		im := bleve.NewMatchQuery(text)
		im.SetField("Id")
		im.SetBoost(30)
		queries = append(queries, im)
	}

	// title prefix
	tp := bleve.NewPrefixQuery(text)
	tp.SetField("Title")
	tp.SetBoost(40)
	queries = append(queries, tp)
	// title substr
	tss := bleve.NewWildcardQuery("*" + strings.ReplaceAll(text, "*", `\*`) + "*")
	tss.SetField("Title")
	tss.SetBoost(8)
	queries = append(queries, tss)
	// title match
	tm := bleve.NewMatchQuery(text)
	tm.SetFuzziness(2)
	tm.SetField("Title")
	tm.SetBoost(7)
	queries = append(queries, tm)
	// text match
	txtm := bleve.NewMatchQuery(text)
	txtm.SetFuzziness(1)
	txtm.SetField("Text")
	queries = append(queries, txtm)

	sr := bleve.NewSearchRequest(bleve.NewDisjunctionQuery(queries...))
	res, err := f.index.Search(sr)
	if err != nil {
		return
	}
	for _, r := range res.Hits {
		results = append(results, r.ID)
	}
	return
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
