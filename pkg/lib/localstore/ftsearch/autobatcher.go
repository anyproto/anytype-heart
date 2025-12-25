package ftsearch

import (
	"fmt"
	"sync"

	tantivy "github.com/anyproto/tantivy-go"
)

type AutoBatcher interface {
	// UpsertDoc adds an update operation to the batcher. If the batch is reaching the size limit, it will be indexed and reset.
	UpsertDoc(doc SearchDoc) error
	// DeleteDoc adds a delete operation to the batcher
	// maxSize limit check is not performed for this operation
	DeleteDoc(id string) error
	// Finish performs the operations
	Finish() (ftIndexSeq uint64, err error)
}

func (f *ftSearch) NewAutoBatcher() AutoBatcher {
	return &ftIndexBatcherTantivy{
		index: f.index,
		mu:    &f.mu,
	}
}

func (f *ftSearch) Iterate(objectId string, fields []string, shouldContinue func(doc *SearchDoc) bool) (err error) {
	sCtx := tantivy.NewSearchContextBuilder().
		SetQuery(fmt.Sprintf("%s:%s", fieldId, objectId)).
		SetDocsLimit(docLimit).
		SetWithHighlights(false).
		AddFieldDefaultWeight(fieldId).
		Build()

	result, err := f.index.Search(sCtx)
	if err != nil {
		return err
	}

	var parser = f.parserPool.Get()
	defer f.parserPool.Put(parser)
	searchResult, err := tantivy.GetSearchResults(
		result,
		f.index,
		func(json string) (*DocumentMatch, error) {
			value, err := parser.Parse(json)
			if err != nil {
				return nil, err
			}
			dm := &DocumentMatch{
				ID: string(value.GetStringBytes(fieldId)),
			}
			dm.Fields = make(map[string]any)
			dm.Fields[fieldSpace] = string(value.GetStringBytes(fieldSpace))
			dm.Fields[fieldText] = string(value.GetStringBytes(fieldText))
			dm.Fields[fieldTextZh] = string(value.GetStringBytes(fieldTextZh))
			dm.Fields[fieldTitle] = string(value.GetStringBytes(fieldTitle))
			dm.Fields[fieldTitleZh] = string(value.GetStringBytes(fieldTitleZh))
			return dm, nil
		},
		fieldId, fieldSpace, fieldTitle, fieldTitleZh, fieldText, fieldTextZh,
	)
	if err != nil {
		return err
	}

	var text, title, spaceId string
	for _, hit := range searchResult {
		if hit.Fields != nil {
			if hit.Fields[fieldTextZh] != nil {
				text, _ = hit.Fields[fieldTextZh].(string)
			}
			if hit.Fields[fieldText] != nil {
				text, _ = hit.Fields[fieldText].(string)
			}
			if hit.Fields[fieldTitleZh] != nil {
				title, _ = hit.Fields[fieldTitleZh].(string)
			}
			if hit.Fields[fieldTitle] != nil {
				title, _ = hit.Fields[fieldTitle].(string)
			}
			if hit.Fields[fieldSpace] != nil {
				spaceId, _ = hit.Fields[fieldSpace].(string)
			}
		}

		if !shouldContinue(&SearchDoc{
			Id:      hit.ID,
			Text:    text,
			Title:   title,
			SpaceId: spaceId,
		}) {
			break
		}
	}
	return nil
}

type ftIndexBatcherTantivy struct {
	index          *tantivy.TantivyContext
	deleteIds      []string
	updateDocs     []*tantivy.Document
	tantivyOpstamp uint64
	mu             *sync.Mutex // original mutex, temporary solution
}

// UpsertDoc adds an update operation to the batcher. If the batch is reaching the size limit, it will be indexed and reset.
func (f *ftIndexBatcherTantivy) UpsertDoc(searchDoc SearchDoc) error {
	err := f.DeleteDoc(searchDoc.Id)
	if err != nil {
		return err
	}
	doc := tantivy.NewDocument()
	if doc == nil {
		return fmt.Errorf("failed to create document")
	}

	for _, field := range []struct {
		value      string
		fieldNames []string
	}{
		{searchDoc.Id, []string{fieldId, fieldIdRaw}},
		{searchDoc.SpaceId, []string{fieldSpace}},
		{searchDoc.Title, []string{fieldTitle, fieldTitleZh}},
		{searchDoc.Text, []string{fieldText, fieldTextZh}},
		{searchDoc.Author, []string{fieldAuthor}},
		{searchDoc.OrderId, []string{fieldOrderId}},
		{searchDoc.MessageId, []string{fieldMessageId}},
		{searchDoc.Timestamp, []string{fieldTimestamp}},
	} {
		if field.value == "" {
			continue
		}
		if len(field.fieldNames) == 1 {
			if err = doc.AddField(field.value, f.index, field.fieldNames[0]); err != nil {
				return err
			}
			continue
		}
		if err = doc.AddFields(field.value, f.index, field.fieldNames...); err != nil {
			return err
		}
	}

	f.updateDocs = append(f.updateDocs, doc)

	if len(f.updateDocs) >= docLimit {
		f.tantivyOpstamp, err = f.Finish()
		if err != nil {
			return fmt.Errorf("finish batch failed: %w", err)
		}
	}
	return nil
}

// Finish indexes the remaining documents in the batch.
func (f *ftIndexBatcherTantivy) Finish() (ftIndexSeq uint64, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	opstamp, err := f.index.BatchAddAndDeleteDocumentsWithOpstamp(f.updateDocs, fieldIdRaw, f.deleteIds)
	if err != nil {
		if f.tantivyOpstamp > 0 {
			log.Warnf("batch was partially commited with opstamp %d, but failed to finish: %v", f.tantivyOpstamp, err)
		}
		return 0, err
	}
	f.deleteIds = f.deleteIds[:0]
	f.updateDocs = f.updateDocs[:0]

	return opstamp, nil
}

// Delete adds a delete operation to the batcher
func (f *ftIndexBatcherTantivy) DeleteDoc(id string) error {
	f.deleteIds = append(f.deleteIds, id)
	return nil
}
