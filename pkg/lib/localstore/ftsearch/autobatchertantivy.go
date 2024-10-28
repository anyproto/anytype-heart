package ftsearch

import (
	"fmt"
	"sync"

	tantivy "github.com/anyproto/tantivy-go"
	"github.com/blevesearch/bleve/v2/search"
)

const docLimit = 10000

func (f *ftSearchTantivy) NewAutoBatcher(maxDocs int, maxSizeBytes uint64) AutoBatcher {
	return &ftIndexBatcherTantivy{
		index: f.index,
		mu:    &f.mu,
	}
}

func (f *ftSearchTantivy) Iterate(objectId string, fields []string, shouldContinue func(doc *SearchDoc) bool) (err error) {
	result, err := f.index.Search(fmt.Sprintf("%s:%s", fieldId, objectId), docLimit, false, fieldId)
	if err != nil {
		return err
	}

	var parser = f.parserPool.Get()
	defer f.parserPool.Put(parser)
	searchResult, err := tantivy.GetSearchResults(
		result,
		f.schema,
		func(json string) (*search.DocumentMatch, error) {
			value, err := parser.Parse(json)
			if err != nil {
				return nil, err
			}
			dm := &search.DocumentMatch{
				ID: string(value.GetStringBytes(fieldId)),
			}
			dm.Fields = make(map[string]any)
			dm.Fields[fieldSpace] = string(value.GetStringBytes(fieldSpace))
			dm.Fields[fieldText] = string(value.GetStringBytes(fieldText))
			dm.Fields[fieldTitle] = string(value.GetStringBytes(fieldTitle))
			return dm, nil
		},
		fieldId, fieldSpace, fieldTitle, fieldText,
	)
	if err != nil {
		return err
	}

	var text, title, spaceId string
	for _, hit := range searchResult {
		if hit.Fields != nil {
			if hit.Fields[fieldText] != nil {
				text, _ = hit.Fields[fieldText].(string)
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
			SpaceID: spaceId,
		}) {
			break
		}
	}
	return nil
}

type ftIndexBatcherTantivy struct {
	index      *tantivy.TantivyContext
	deleteIds  []string
	updateDocs []*tantivy.Document
	mu         *sync.Mutex // original mutex, temporary solution
}

// Add adds a update operation to the batcher. If the batch is reaching the size limit, it will be indexed and reset.
func (f *ftIndexBatcherTantivy) UpdateDoc(searchDoc SearchDoc) error {
	err := f.DeleteDoc(searchDoc.Id)
	if err != nil {
		return err
	}
	doc := tantivy.NewDocument()
	if doc == nil {
		return fmt.Errorf("failed to create document")
	}

	err = doc.AddField(fieldId, searchDoc.Id, f.index)
	if err != nil {
		return err
	}

	err = doc.AddField(fieldIdRaw, searchDoc.Id, f.index)
	if err != nil {
		return err
	}

	err = doc.AddField(fieldSpace, searchDoc.SpaceID, f.index)
	if err != nil {
		return err
	}

	err = doc.AddField(fieldTitle, searchDoc.Title, f.index)
	if err != nil {
		return err
	}

	err = doc.AddField(fieldText, searchDoc.Text, f.index)
	if err != nil {
		return err
	}

	f.updateDocs = append(f.updateDocs, doc)

	if len(f.updateDocs) >= docLimit {
		return f.Finish()
	}
	return nil
}

// Finish indexes the remaining documents in the batch.
func (f *ftIndexBatcherTantivy) Finish() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := f.index.DeleteDocuments(fieldIdRaw, f.deleteIds...)
	if err != nil {
		return err
	}
	err = f.index.AddAndConsumeDocuments(f.updateDocs...)
	if err != nil {
		return err
	}
	f.deleteIds = f.deleteIds[:0]
	f.updateDocs = f.updateDocs[:0]
	return nil
}

// Delete adds a delete operation to the batcher
func (f *ftIndexBatcherTantivy) DeleteDoc(id string) error {
	f.deleteIds = append(f.deleteIds, id)
	return nil
}
