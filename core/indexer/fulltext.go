package indexer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/cache"
	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var (
	ftIndexInterval         = 10 * time.Second
	ftIndexForceMinInterval = time.Second * 10
	ftBatchLimit            = 100
	ftBlockMaxSize          = 1024 * 1024
)

func (i *indexer) ForceFTIndex() {
	select {
	case i.forceFt <- struct{}{}:
	default:
	}
}

// ftLoop runs full-text indexer
// MUST NOT be called more than once
func (i *indexer) ftLoopRoutine() {
	ticker := time.NewTicker(ftIndexInterval)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-i.quit:
			cancel()
		case <-ctx.Done():
		}
	}()

	i.runFullTextIndexer(ctx)
	defer close(i.ftQueueFinished)
	var lastForceIndex time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			i.runFullTextIndexer(ctx)
		case <-i.forceFt:
			if time.Since(lastForceIndex) > ftIndexForceMinInterval {
				i.runFullTextIndexer(ctx)
				lastForceIndex = time.Now()
			}
		}
	}
}

func (i *indexer) runFullTextIndexer(ctx context.Context) {
	batcher := i.ftsearch.NewAutoBatcher(ftsearch.AutoBatcherRecommendedMaxDocs, ftsearch.AutoBatcherRecommendedMaxSize)
	err := i.store.BatchProcessFullTextQueue(ctx, ftBatchLimit, func(objectIds []string) error {
		for _, objectId := range objectIds {
			objDocs, err := i.prepareSearchDocument(ctx, objectId)
			if err != nil {
				log.With("id", objectId).Errorf("prepare document for full-text indexing: %s", err)
				if errors.Is(err, context.Canceled) {
					return err
				}
				continue
			}

			objDocs, objRemovedIds, err := i.filterOutNotChangedDocuments(objectId, objDocs)
			for _, removeId := range objRemovedIds {
				err = batcher.DeleteDoc(removeId)
				if err != nil {
					return fmt.Errorf("batcher delete: %w", err)
				}
			}

			for _, doc := range objDocs {
				err = batcher.UpdateDoc(doc)
				if err != nil {
					return fmt.Errorf("batcher add: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		log.Errorf("list ids from full-text queue: %v", err)
		return
	}
	err = batcher.Finish()
	if err != nil {
		log.Errorf("finish batcher: %v", err)
		return
	}
}

func (i *indexer) filterOutNotChangedDocuments(id string, newDocs []ftsearch.SearchDoc) (changed []ftsearch.SearchDoc, removedIds []string, err error) {
	var (
		changedDocs []ftsearch.SearchDoc
		removeDocs  []string
	)
	err = i.ftsearch.Iterate(id, []string{"Title", "Text"}, func(doc *ftsearch.SearchDoc) bool {
		newDocIndex := slice.Find(newDocs, func(d ftsearch.SearchDoc) bool {
			return d.Id == doc.Id
		})

		if newDocIndex == -1 {
			// doc got removed
			removeDocs = append(removeDocs, doc.Id)
			return true
		} else {
			if newDocs[newDocIndex].Text != doc.Text || newDocs[newDocIndex].Title != doc.Title {
				changedDocs = append(changedDocs, newDocs[newDocIndex])
			}
		}
		return true
	})
	if err != nil {
		return nil, nil, fmt.Errorf("iterate over existing objects: %w", err)
	}

	for _, doc := range newDocs {
		if !slices.ContainsFunc(changedDocs, func(d ftsearch.SearchDoc) bool {
			return d.Id == doc.Id
		}) {
			// doc is new as it doesn't exist in the index
			changedDocs = append(changedDocs, doc)
		}
	}
	return changedDocs, removeDocs, nil
}

func (i *indexer) prepareSearchDocument(ctx context.Context, id string) (docs []ftsearch.SearchDoc, err error) {
	ctx = context.WithValue(ctx, metrics.CtxKeyEntrypoint, "index_fulltext")
	err = cache.DoContext(i.picker, ctx, id, func(sb smartblock2.SmartBlock) error {
		indexDetails, _ := sb.Type().Indexable()
		if !indexDetails {
			return nil
		}

		for _, rel := range sb.GetRelationLinks() {
			if rel.Format != model.RelationFormat_shorttext && rel.Format != model.RelationFormat_longtext {
				continue
			}
			val := pbtypes.GetString(sb.Details(), rel.Key)
			if val == "" {
				continue
			}
			// skip readonly and hidden system relations
			if bundledRel, err := bundle.PickRelation(domain.RelationKey(rel.Key)); err == nil {
				if bundledRel.ReadOnly || bundledRel.Hidden && rel.Key != bundle.RelationKeyName.String() {
					continue
				}
			}

			doc := ftsearch.SearchDoc{
				Id:      domain.NewObjectPathWithRelation(id, rel.Key).String(),
				SpaceID: sb.SpaceID(),
				Text:    val,
			}

			if rel.Key == bundle.RelationKeyName.String() {
				doc.Title = val
			}
			docs = append(docs, doc)
		}

		sb.Iterate(func(b simple.Block) (isContinue bool) {
			if ctx.Err() != nil {
				return false
			}
			if tb := b.Model().GetText(); tb != nil {
				if len(strings.TrimSpace(tb.Text)) == 0 {
					return true
				}

				if len(pbtypes.GetStringList(b.Model().GetFields(), text.DetailsKeyFieldName)) > 0 {
					// block doesn't store the value itself, but it's a reference to relation
					return true
				}
				doc := ftsearch.SearchDoc{
					Id:      domain.NewObjectPathWithBlock(id, b.Model().Id).String(),
					SpaceID: sb.SpaceID(),
				}
				if len(tb.Text) > ftBlockMaxSize {
					doc.Text = tb.Text[:ftBlockMaxSize]
				} else {
					doc.Text = tb.Text
				}
				docs = append(docs, doc)

			}
			return true
		})

		return nil
	})

	return docs, err
}

func (i *indexer) ftInit() error {
	if ft := i.store.FTSearch(); ft != nil {
		docCount, err := ft.DocCount()
		if err != nil {
			return err
		}
		if docCount == 0 {
			ids, err := i.store.ListIds()
			if err != nil {
				return err
			}
			for _, id := range ids {
				if err := i.store.AddToIndexQueue(id); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
