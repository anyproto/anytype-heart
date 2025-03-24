package indexer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/samber/lo"
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
	ftIndexInterval              = 1 * time.Second
	ftIndexForceMinInterval      = time.Second * 10
	ftBatchLimit            uint = 1000
	ftBlockMaxSize               = 1024 * 1024
	maxErrSent              atomic.Int32
)

const maxErrorsPerSession = 100

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

func (i *indexer) OnSpaceLoad(spaceId string) {
	i.spacesLock.Lock()
	defer i.spacesLock.Unlock()
	if i.spaces == nil {
		i.spaces = map[string]struct{}{}
	}
	i.spaces[spaceId] = struct{}{}
	i.ForceFTIndex()
}

func (i *indexer) OnSpaceUnload(spaceId string) {
	i.spacesLock.Lock()
	defer i.spacesLock.Unlock()
	delete(i.spaces, spaceId)
}

func (i *indexer) activeSpaces() []string {
	i.spacesLock.RLock()
	defer i.spacesLock.RUnlock()
	return lo.MapToSlice(i.spaces, func(key string, _ struct{}) string { return key })
}

func (i *indexer) runFullTextIndexer(ctx context.Context) {
	batcher := i.ftsearch.NewAutoBatcher()
	err := i.store.BatchProcessFullTextQueue(ctx, i.activeSpaces, ftBatchLimit, func(objectIds []domain.FullID) ([]string, error) {
		toRemove := make([]string, 0, len(objectIds))
		for _, objectId := range objectIds {
			objDocs, err := i.prepareSearchDocument(ctx, objectId.ObjectID)
			if err != nil &&
				!errors.Is(err, domain.ErrObjectNotFound) &&
				!errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
				log.With("id", objectId).Errorf("prepare document for full-text indexing: %s", err)
				if errors.Is(err, context.Canceled) {
					return nil, err
				}
				continue
			}

			objDocs, objRemovedIds, err := i.filterOutNotChangedDocuments(objectId.ObjectID, objDocs)
			if err != nil {
				log.With("id", objectId).Errorf("filter not changed error:: %s", err)
				// try to process the other returned values.
				continue
			}
			for _, removeId := range objRemovedIds {
				err = batcher.DeleteDoc(removeId)
				if err != nil {
					return nil, fmt.Errorf("batcher delete: %w", err)
				}
			}

			for _, doc := range objDocs {
				err = batcher.UpdateDoc(doc)
				if err != nil {
					return nil, fmt.Errorf("batcher add: %w", err)
				}
			}
			toRemove = append(toRemove, objectId.ObjectID)
		}
		err := batcher.Finish()
		if err != nil {
			return nil, fmt.Errorf("finish batch: %w", err)
		}
		return toRemove, nil
	})
	if err != nil && maxErrSent.Load() < maxErrorsPerSession {
		maxErrSent.Add(1)
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
	var fulltextSkipped bool
	err = cache.DoContext(i.picker, ctx, id, func(sb smartblock2.SmartBlock) error {
		fulltext, _, _ := sb.Type().Indexable()
		if !fulltext {
			fulltextSkipped = true
			return nil
		}

		for _, rel := range sb.GetRelationLinks() {
			if rel.Format != model.RelationFormat_shorttext && rel.Format != model.RelationFormat_longtext {
				continue
			}
			val := sb.Details().GetString(domain.RelationKey(rel.Key))
			if val == "" {
				val = sb.LocalDetails().GetString(domain.RelationKey(rel.Key))
				if val == "" {
					continue
				}
			}
			// skip readonly and hidden system relations
			if bundledRel, err := bundle.PickRelation(domain.RelationKey(rel.Key)); err == nil {
				layout, _ := sb.Layout()
				skip := bundledRel.ReadOnly || bundledRel.Hidden
				if rel.Key == bundle.RelationKeyName.String() {
					skip = false
				}
				if layout == model.ObjectType_note && rel.Key == bundle.RelationKeySnippet.String() {
					// index snippet only for notes, so we will be able to do fast prefix queries
					skip = false
				}

				if skip {
					continue
				}
			}

			doc := ftsearch.SearchDoc{
				Id:      domain.NewObjectPathWithRelation(id, rel.Key).String(),
				SpaceId: sb.SpaceID(),
				Text:    val,
			}

			if rel.Key == bundle.RelationKeyName.String() {
				layout, layoutValid := sb.Layout()
				if layoutValid {
					if _, contains := filesLayouts[layout]; !contains {
						doc.Title = val
						doc.Text = ""
					}
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
	if fulltextSkipped {
		// todo: this should be removed. objects which is not supposed to be added to fulltext index should not be added to the queue
		// but now it happens in the ftInit that some objects still can be added to the queue
		// we need to avoid TryRemoveFromCache in this case
		return docs, nil
	}

	_, cacheErr := i.picker.TryRemoveFromCache(ctx, id)
	if cacheErr != nil &&
		!errors.Is(err, domain.ErrObjectNotFound) {
		log.With("objectId", id).Errorf("object cache remove: %v", err)
	}

	return docs, err
}

func (i *indexer) ftInit() error {
	if ft := i.ftsearch; ft != nil {
		docCount, err := ft.DocCount()
		if err != nil {
			return err
		}
		if docCount == 0 {
			// delete the remnants from the last run if any
			err := i.store.ClearFullTextQueue(nil)
			if err != nil {
				return err
			}
			// query objects that are existing in the store
			// if they are not existing in the object store, they will be indexed and added via reindexOutdatedObjects or on receiving via any-sync
			ids, err := i.store.ListIdsCrossSpaceWithoutTech()
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
