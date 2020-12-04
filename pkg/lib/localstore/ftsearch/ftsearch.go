package ftsearch

import (
	"strings"
	"unicode/utf8"

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
	var q query.Query
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= 3 {
		q = bleve.NewPrefixQuery(text)
	} else {
		fq := bleve.NewFuzzyQuery(text)
		fq.SetFuzziness(2)
		q = fq
	}
	sr := bleve.NewSearchRequest(q)
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
