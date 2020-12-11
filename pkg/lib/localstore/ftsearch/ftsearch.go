package ftsearch

import (
	"strings"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/search/query"
)

type SearchDoc struct {
	Id    string
	Title string
	Text  string
}

func NewFTSearch(path string) (FTSearch, error) {
	fts := &ftSearch{path: path}
	if err := fts.init(); err != nil {
		return nil, err
	}
	return fts, nil
}

type FTSearch interface {
	Index(d SearchDoc) (err error)
	Search(query string) (results []string, err error)
	Delete(id string) error
	DocCount() (uint64, error)
	Close()
}

type ftSearch struct {
	path  string
	index bleve.Index
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
	text = strings.TrimSpace(text)
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

func (f *ftSearch) Close() {
	if f.index != nil {
		f.index.Close()
	}
}
