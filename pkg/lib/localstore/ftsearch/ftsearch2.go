package ftsearch

import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/JanFalkin/tantivy-jpc/go-client/tantivy"
	"github.com/anyproto/any-sync/app"
	"github.com/blevesearch/bleve/v2/search"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
)

func New2() FTSearch {
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

func (f *ftSearch2) Init(a *app.App) (err error) {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	f.rootPath = filepath.Join(repoPath, ftsDir2)
	f.ftsPath = filepath.Join(repoPath, ftsDir2, ftsVer)
	os.MkdirAll(f.ftsPath, 0755)
	tantivy.LibInit("release")
	return err
}

func (f *ftSearch2) Name() (name string) {
	return CName
}

func (f *ftSearch2) Run(context.Context) (err error) {
	builder, err := tantivy.NewBuilder(f.ftsPath)
	if err != nil {
		panic(err)
	}
	f.builderId = builder.ID()

	idxFieldId, err := builder.AddTextField(fieldID, tantivy.STRING, true, true, "", false)
	if err != nil {
		panic(err)
	}

	idxFieldSpaceId, err := builder.AddTextField(fieldSpace, tantivy.STRING, true, true, "", false)
	if err != nil {
		panic(err)
	}

	idxFieldTitle, err := builder.AddTextField(fieldTitle, tantivy.TEXT, true, true, "", false)
	if err != nil {
		panic(err)
	}

	idxFieldTitleNoTerms, err := builder.AddTextField(fieldTitleNoTerms, tantivy.STRING, true, true, "", false)
	if err != nil {
		panic(err)
	}

	idxFieldText, err := builder.AddTextField(fieldText, tantivy.TEXT, true, true, "", false)
	if err != nil {
		panic(err)
	}

	idxFieldTextNoTerms, err := builder.AddTextField(fieldTextNoTerms, tantivy.STRING, true, true, "", false)
	if err != nil {
		panic(err)
	}

	doc, err := builder.Build()
	if err != nil {
		panic(err)
	}

	idx, err := doc.CreateIndex()
	if err != nil {
		panic(err)
	}

	_, err = idx.SetMultiThreadExecutor(8)
	if err != nil {
		panic(err)
	}

	_ = os.Mkdir(f.ftsPath, os.ModeDir)
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

func (f *ftSearch2) Index(doc SearchDoc) (err error) {
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
	writer, _ := f.index.CreateIndexWriter()
	defer writer.Commit()
	return txn(writer)
}

func (f *ftSearch2) addDoc(doc SearchDoc) (uint, error) {
	regex := regexp.MustCompile(`[\n\t\r]+`) // remove newlines, tabs, and carriage returns
	doc.TitleNoTerms = regex.ReplaceAllString(doc.Title, " ")
	doc.TextNoTerms = regex.ReplaceAllString(doc.Text, " ")
	toAdd, err := f.doc.Create()
	if err != nil {
		return 0, err
	}
	f.doc.AddText(f.idxFieldId, doc.Id, toAdd)
	f.doc.AddText(f.idxFieldSpaceId, doc.SpaceID, toAdd)
	f.doc.AddText(f.idxFieldTitle, doc.Title, toAdd)
	f.doc.AddText(f.idxFieldTitleNoTerms, doc.TitleNoTerms, toAdd)
	f.doc.AddText(f.idxFieldText, doc.Text, toAdd)
	f.doc.AddText(f.idxFieldTextNoTerms, doc.TextNoTerms, toAdd)
	return toAdd, nil
}

func (f *ftSearch2) BatchIndex(docs []SearchDoc) (err error) {
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
	err = f.commit(func(writer *tantivy.TIndexWriter) error {
		for _, doc := range docs {
			toAdd, _ := f.addDoc(doc)
			_, err := addDocument(writer, doc.Id, toAdd)
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
	deleteTerm(writer, docId)
	return writer.AddDocument(toAdd)
}

func deleteTerm(writer *tantivy.TIndexWriter, docId string) (uint, error) {
	return writer.DeleteTerm(fieldID, docId)
}

func (f *ftSearch2) BatchDelete(ids []string) (err error) {
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
	f.commit(func(writer *tantivy.TIndexWriter) error {
		for _, id := range ids {
			deleteTerm(writer, id)
		}
		return nil
	})

	return nil
}

func (f *ftSearch2) Search(spaceID, qry string) (results search.DocumentMatchCollection, err error) {
	start := time.Now().UnixMilli()
	err, searcher := f.prepareSearcher(spaceID, qry)

	var sr []map[string]interface{}
	fmt.Println("### Pre-Search took", time.Now().UnixMilli()-start, "ms")
	start = time.Now().UnixMilli()
	s, err := searcher.Search(true, 100, 0, true)
	fmt.Println("### Search took", time.Now().UnixMilli()-start, "ms")
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal([]byte(s), &sr)
	if err != nil {
		panic(err)
	}

	res := make([]*search.DocumentMatch, 0, len(sr))
	for _, value := range sr {
		docMap, ok := value["doc"].(map[string]interface{})
		if !ok {
			// Обработка ошибки или возврат значения по умолчанию
		}

		fieldValue, ok := docMap[fieldID]
		if !ok {
			// Обработка ошибки или возврат значения по умолчанию
		}

		fieldStringValue, ok := fieldValue.([]any)[0].(string)
		if !ok {
			// Обработка ошибки или возврат значения по умолчанию
		}
		res = append(res, &search.DocumentMatch{
			ID:    fieldStringValue,
			Score: value["score"].(float64),
		})
	}

	return res, nil
}

func (f *ftSearch2) prepareSearcher(spacedId string, qry string) (error, *tantivy.TSearcher) {
	rb, err := f.index.ReaderBuilder()
	if err != nil {
		panic(err)
	}

	qp, err := rb.Searcher()
	if err != nil {
		panic(err)
	}

	_, err = qp.ForIndex([]string{fieldID, fieldSpace, fieldTitle, fieldTitleNoTerms, fieldText, fieldTextNoTerms})
	if err != nil {
		panic(err)
	}

	// searcher, err := qp.ParseQuery(qry)
	searcher, err := qp.PrepareQuery(spacedId, qry)
	if err != nil {
		panic(err)
	}

	return err, searcher
}

func (f *ftSearch2) Has(id string) (exists bool, err error) {
	// d, err := f.index.Document(id)
	// if err != nil {
	// 	return false, err
	// }
	// return d != nil, nil
	return false, err
}

func (f *ftSearch2) Delete(id string) (err error) {
	f.commit(func(writer *tantivy.TIndexWriter) error {
		deleteTerm(writer, id)
		return nil
	})
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
