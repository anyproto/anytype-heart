package ftsearch

import "github.com/blevesearch/bleve"

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

func (f *ftSearch) Search(query string) (results []string, err error) {
	q := bleve.NewSearchRequest(bleve.NewQueryStringQuery(query))
	res, err := f.index.Search(q)
	if err != nil {
		return
	}
	for _, r := range res.Hits {
		results = append(results, r.ID)
	}
	return
}

func (f *ftSearch) Close() {
	if f.index != nil {
		f.index.Close()
	}
}
