package ftsearch

import "C"
import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/JanFalkin/tantivy-jpc/go-client/tantivy"
	"github.com/anyproto/any-sync/app"
	"github.com/blevesearch/bleve/v2/search"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
)

func TantivyNew() FTSearch {
	return &ftSearch2{}
}

type ftSearch2 struct {
	rootPath             string
	ftsPath              string
	builderId            string
	index                *tantivy.TIndex
	doc                  *tantivy.TDocument
	idxFieldId           int
	idxFieldSpaceId      int
	idxFieldTitle        int
	idxFieldTitleNoTerms int
	idxFieldText         int
	idxFieldTextNoTerms  int
}

var ftsDir2 = "fts_tantivy"

func (f *ftSearch2) Init(a *app.App) error {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	f.rootPath = filepath.Join(repoPath, ftsDir2)
	f.ftsPath = filepath.Join(repoPath, ftsDir2, ftsVer)
	_ = os.MkdirAll(f.ftsPath, 0755) // nolint:errcheck
	tantivy.LibInit("release")
	return nil
}

func (f *ftSearch2) Name() (name string) {
	return CName
}

func (f *ftSearch2) Run(context.Context) error {
	builder, err := tantivy.NewBuilder(f.ftsPath)
	if err != nil {
		return err
	}
	f.builderId = builder.ID()

	idxFieldId, err := builder.AddTextField(fieldId, tantivy.STRING, true, true, "", false)
	if err != nil {
		return err
	}

	idxFieldSpaceId, err := builder.AddTextField(fieldSpace, tantivy.STRING, true, true, "", false)
	if err != nil {
		return err
	}

	idxFieldTitle, err := builder.AddTextField(fieldTitle, tantivy.TEXT, true, true, "", false)
	if err != nil {
		return err
	}

	idxFieldTitleNoTerms, err := builder.AddTextField(fieldTitleNoTerms, tantivy.STRING, true, true, "", false)
	if err != nil {
		return err
	}

	idxFieldText, err := builder.AddTextField(fieldText, tantivy.TEXT, true, true, "", false)
	if err != nil {
		return err
	}

	idxFieldTextNoTerms, err := builder.AddTextField(fieldTextNoTerms, tantivy.STRING, true, true, "", false)
	if err != nil {
		return err
	}

	doc, err := builder.Build()
	if err != nil {
		return err
	}

	idx, err := doc.CreateIndex()
	if err != nil {
		return err
	}

	_, err = idx.SetMultiThreadExecutor(int32(runtime.NumCPU()))
	if err != nil {
		return err
	}

	f.index = idx
	f.doc = doc
	f.idxFieldId = idxFieldId
	f.idxFieldSpaceId = idxFieldSpaceId
	f.idxFieldTitle = idxFieldTitle
	f.idxFieldTitleNoTerms = idxFieldTitleNoTerms
	f.idxFieldText = idxFieldText
	f.idxFieldTextNoTerms = idxFieldTextNoTerms
	return nil
}

func (f *ftSearch2) Index(doc SearchDoc) error {
	metrics.ObjectFTUpdatedCounter.Inc()
	toAdd, err := f.addDoc(doc)
	if err != nil {
		return err
	}
	err = f.commit(func(writer *tantivy.TIndexWriter) error {
		_, err := addDocument(writer, doc.Id, toAdd)
		return err
	})

	if err != nil {
		return err
	}

	return nil
}

func (f *ftSearch2) commit(txn func(writer *tantivy.TIndexWriter) error) error {
	writer, err := f.index.CreateIndexWriter()
	if err != nil {
		return err
	}

	err = txn(writer)
	if err != nil {
		return err
	}

	_, err = writer.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (f *ftSearch2) addDoc(doc SearchDoc) (uint, error) {
	regex := regexp.MustCompile(`[\n\t\r]+`) // remove newlines, tabs, and carriage returns
	doc.TitleNoTerms = regex.ReplaceAllString(doc.Title, " ")
	doc.TextNoTerms = regex.ReplaceAllString(doc.Text, " ")
	addedDocIndex, err := f.doc.Create()
	if err != nil {
		return 0, err
	}
	_, err = f.doc.AddText(f.idxFieldId, doc.Id, addedDocIndex)
	if err != nil {
		return 0, err
	}
	_, err = f.doc.AddText(f.idxFieldSpaceId, doc.SpaceID, addedDocIndex)
	if err != nil {
		return 0, err
	}
	_, err = f.doc.AddText(f.idxFieldTitle, doc.Title, addedDocIndex)
	if err != nil {
		return 0, err
	}
	_, err = f.doc.AddText(f.idxFieldTitleNoTerms, doc.TitleNoTerms, addedDocIndex)
	if err != nil {
		return 0, err
	}
	_, err = f.doc.AddText(f.idxFieldText, doc.Text, addedDocIndex)
	if err != nil {
		return 0, err
	}
	_, err = f.doc.AddText(f.idxFieldTextNoTerms, doc.TextNoTerms, addedDocIndex)
	if err != nil {
		return 0, err
	}
	return addedDocIndex, nil
}

func (f *ftSearch2) BatchIndex(ctx context.Context, docs []SearchDoc) error {
	if len(docs) == 0 {
		return nil
	}
	metrics.ObjectFTUpdatedCounter.Add(float64(len(docs)))
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
	err := f.commit(func(writer *tantivy.TIndexWriter) error {
		for _, doc := range docs {
			toAdd, err := f.addDoc(doc)
			if err != nil {
				return err
			}

			_, err = addDocument(writer, doc.Id, toAdd)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}
	return nil
}

func addDocument(writer *tantivy.TIndexWriter, docId string, toAdd uint) (uint, error) {
	term, err := deleteTerm(writer, docId)
	if err != nil {
		return term, err
	}
	return writer.AddDocument(toAdd)
}

func deleteTerm(writer *tantivy.TIndexWriter, docId string) (uint, error) {
	return writer.DeleteTerm(fieldId, docId)
}

func (f *ftSearch2) BatchDelete(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	start := time.Now()
	defer func() {
		spentMs := time.Since(start).Milliseconds()
		l := log.With("objects", len(ids)).With("total", time.Since(start).Milliseconds())
		if spentMs > 1000 {
			l.Warnf("ft delete took too long")
		} else {
			l.Debugf("ft delete done")
		}
	}()
	err := f.commit(func(writer *tantivy.TIndexWriter) error {
		for _, id := range ids {
			_, err := deleteTerm(writer, id)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (f *ftSearch2) Search(spaceID, qry string) (search.DocumentMatchCollection, error) {
	err, searcher := f.prepareSearcher(spaceID, qry)

	var sr []map[string]interface{}
	s, err := searcher.Search(true, 100, 0, true)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(s), &sr)
	if err != nil {
		return nil, err
	}

	res := make([]*search.DocumentMatch, 0, len(sr))
	for _, value := range sr {
		docMap, ok := value["doc"].(map[string]interface{})
		if !ok {
			return nil, errors.New("doc not found")
		}

		fieldValue, ok := docMap[fieldId]
		if !ok {
			return nil, errors.New("id not found")
		}

		arrayValue, ok := fieldValue.([]any)
		if !ok || len(arrayValue) == 0 {
			return nil, errors.New("fieldValue is not a valid array of interfaces")
		}

		fieldStringValue, ok := arrayValue[0].(string)
		if !ok {
			return nil, errors.New("fieldValue[0] is not a string")
		}

		scoreValue, ok := value["score"]
		if !ok {
			return nil, errors.New("key not found in the map")
		}

		scoreFloat, ok := scoreValue.(float64)
		if !ok {
			return nil, errors.New("value is not a float64")
		}

		res = append(res, &search.DocumentMatch{
			ID:    fieldStringValue,
			Score: scoreFloat,
		})
	}

	return res, nil
}

func (f *ftSearch2) prepareSearcher(spacedId string, qry string) (error, *tantivy.TSearcher) {
	rb, err := f.index.ReaderBuilder()
	if err != nil {
		return err, nil
	}

	qp, err := rb.Searcher()
	if err != nil {
		return err, nil
	}

	_, err = qp.ForIndex([]string{fieldId, fieldSpace, fieldTitle, fieldTitleNoTerms, fieldText, fieldTextNoTerms})
	if err != nil {
		return err, nil
	}

	// searcher, err := qp.ParseQuery(qry)
	searcher, err := qp.PrepareQuery(spacedId, qry)
	if err != nil {
		return err, nil
	}

	return nil, searcher
}

func (f *ftSearch2) Delete(id string) error {
	err := f.commit(func(writer *tantivy.TIndexWriter) error {
		_, err := deleteTerm(writer, id)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (f *ftSearch2) DocCount() (uint64, error) {
	err, searcher := f.prepareSearcher("", "")
	if err != nil {
		return 0, err
	}

	docs, err := searcher.NumDocs()
	if err != nil {
		return 0, err
	}

	result, err := strconv.Atoi(docs)
	return uint64(result), err
}

func (f *ftSearch2) Close(ctx context.Context) error {
	tantivy.ClearSession(f.builderId)
	return nil
}
