package indexer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
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
	ftIndexInterval         = 1 * time.Second
	ftIndexForceMinInterval = time.Second * 10
	ftBatchLimit            = 1000
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
	ctx := i.runCtx

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
	batcher := i.ftsearch.NewAutoBatcher()
	err := i.store.BatchProcessFullTextQueue(ctx, ftBatchLimit, func(objectIds []string) error {
		for _, objectId := range objectIds {
			objDocs, err := i.prepareSearchDocument(ctx, objectId)
			if err != nil && !errors.Is(err, domain.ErrObjectNotFound) && !errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
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
				if err != nil {
					return fmt.Errorf("batcher delete: %w", err)
				}
				err = batcher.UpdateDoc(doc)
				if err != nil {
					return fmt.Errorf("batcher add: %w", err)
				}
			}
		}
		err := batcher.Finish()
		if err != nil {
			return fmt.Errorf("finish batch: %w", err)
		}
		return nil
	})
	if err != nil {
		log.Errorf("list ids from full-text queue: %v", err)
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

var filesLayouts = map[model.ObjectTypeLayout]struct{}{
	model.ObjectType_file:  {},
	model.ObjectType_image: {},
	model.ObjectType_audio: {},
	model.ObjectType_video: {},
	model.ObjectType_pdf:   {},
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
			val := sb.Details().GetString(domain.RelationKey(rel.Key))
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
				SpaceId: sb.SpaceID(),
				Text:    val,
			}

			layout, layoutValid := sb.Layout()
			if layoutValid {
				if _, contains := filesLayouts[layout]; !contains {
					doc.Title = val
					doc.Text = ""
				}
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
					SpaceId: sb.SpaceID(),
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
	if ft := i.ftsearch; ft != nil {
		docCount, err := ft.DocCount()
		if err != nil {
			return err
		}
		if docCount == 0 {
			// query objects that are existing in the store
			// if they are not existing in the object store, they will be indexed and added via reindexOutdatedObjects or on receiving via any-sync
			ids, err := i.store.ListIdsCrossSpace()
			if err != nil {
				return err
			}
			if err := i.store.AddToIndexQueue(i.runCtx, ids...); err != nil {
				return err
			}

		}
	}
	return nil
}
