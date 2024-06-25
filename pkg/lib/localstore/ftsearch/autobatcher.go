package ftsearch

import (
	"fmt"

	"github.com/blevesearch/bleve/v2"
)

const (
	AutoBatcherRecommendedMaxDocs = 300
	AutoBatcherRecommendedMaxSize = 10 * 1024 * 1024 // 10MB
)

type AutoBatcher interface {
	// UpdateDoc adds a update operation to the batcher. If the batch is reaching the size limit, it will be indexed and reset.
	UpdateDoc(doc SearchDoc) error
	// DeleteDoc adds a delete operation to the batcher
	// maxSize limit check is not performed for this operation
	DeleteDoc(id string) error
	// Finish performs the
	Finish() error
}

func (f *ftSearch) NewAutoBatcher(maxDocs int, maxSizeBytes uint64) AutoBatcher {
	return &ftIndexBatcher{
		batch:        f.index.NewBatch(),
		index:        f.index,
		maxSizeBytes: maxSizeBytes,
		maxDocs:      maxDocs,
	}
}

type ftIndexBatcher struct {
	batch        *bleve.Batch
	index        bleve.Index
	docs         int
	maxSizeBytes uint64
	maxDocs      int
}

// Add adds a update operation to the batcher. If the batch is reaching the size limit, it will be indexed and reset.
func (f *ftIndexBatcher) UpdateDoc(doc SearchDoc) error {
	doc.TitleNoTerms = doc.Title
	doc.TextNoTerms = doc.Text
	if err := f.batch.Index(doc.Id, doc); err != nil {
		return fmt.Errorf("failed to index document %s: %w", doc.Id, err)
	}
	f.docs++
	var err error
	if (f.maxSizeBytes > 0 && f.batch.TotalDocsSize() >= f.maxSizeBytes) ||
		(f.docs > 0 && f.docs >= f.maxDocs) {
		err = f.index.Batch(f.batch)
		if err != nil {
			return err
		}
		f.batch.Reset()
		f.docs = 0
	}
	return nil
}

// Finish indexes the remaining documents in the batch.
func (f *ftIndexBatcher) Finish() error {
	if f.batch.Size() == 0 {
		return nil
	}
	err := f.index.Batch(f.batch)
	if err != nil {
		return err
	}
	f.batch.Reset()
	f.docs = 0
	// do not check batch size
	return nil
}

// Delete adds a delete operation to the batcher
func (f *ftIndexBatcher) DeleteDoc(id string) error {
	f.batch.Delete(id)
	// do not check batch size
	return nil
}
