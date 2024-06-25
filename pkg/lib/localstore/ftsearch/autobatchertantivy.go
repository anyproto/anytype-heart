package ftsearch

import (
	"fmt"

	"github.com/anyproto/tantivy-go/go/tantivy"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/valyala/fastjson"
)

const docLimit = 10000

func (f *ftSearch2) NewAutoBatcher(maxDocs int, maxSizeBytes uint64) AutoBatcher {
	return &ftIndexBatcher2{
		index: f.index,
	}
}

func (f *ftSearch2) Iterate(objectId string, fields []string, shouldContinue func(doc *SearchDoc) bool) (err error) {
	result, err := f.index.Search(fmt.Sprintf("%s:%s", fieldId, objectId), docLimit, false, fieldId)
	if err != nil {
		return err
	}

	var p fastjson.Parser
	searchResult, err := tantivy.GetSearchResults(
		result,
		f.schema,
		func(json string) (*search.DocumentMatch, error) {
			value, err := p.Parse(json)
			if err != nil {
				return nil, err
			}
			dm := &search.DocumentMatch{
				ID: string(value.GetStringBytes(fieldId)),
			}
			dm.Fields = make(map[string]any)
			dm.Fields[fieldSpace] = value.GetStringBytes(fieldSpace)
			dm.Fields[fieldText] = value.GetStringBytes(fieldText)
			dm.Fields[fieldTitle] = value.GetStringBytes(fieldTitle)
			return dm, nil
		},
		fieldId, fieldSpace, fieldTitle, fieldText,
	)
	if err != nil {
		return err
	}

	var text, title, spaceId string
	for _, hit := range searchResult {
		text, title, spaceId = "", "", ""
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

type ftIndexBatcher2 struct {
	index      *tantivy.Index
	deleteIds  []string
	updateDocs []*tantivy.Document
}

// Add adds a update operation to the batcher. If the batch is reaching the size limit, it will be indexed and reset.
func (f *ftIndexBatcher2) UpdateDoc(searchDoc SearchDoc) error {
	doc := tantivy.NewDocument()
	if doc == nil {
		return fmt.Errorf("failed to create document")
	}

	err := doc.AddField(fieldId, searchDoc.Id, f.index)
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
	return nil
}

// Finish indexes the remaining documents in the batch.
func (f *ftIndexBatcher2) Finish() error {
	err := f.index.DeleteDocuments(fieldId, f.deleteIds...)
	if err != nil {
		return err
	}
	return f.index.AddAndConsumeDocuments(f.updateDocs...)
}

// Delete adds a delete operation to the batcher
func (f *ftIndexBatcher2) DeleteDoc(id string) error {
	f.deleteIds = append(f.deleteIds, id)
	return nil
}
