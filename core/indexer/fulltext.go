package indexer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/cache"
	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
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
	ftBatchLimit            = 50
	ftBlockMaxSize          = 1024 * 1024
)

func (i *indexer) ForceFTIndex() {
	select {
	case i.forceFt <- struct{}{}:
	default:
	}
}

func (i *indexer) getSpaceIdsByPriority() []string {
	var ids = make([]string, 0, i.lastSpacesSubscription.Len())
	i.lastSpacesSubscription.Iterate(func(_ string, v *types.Struct) bool {
		id := pbtypes.GetString(v, bundle.RelationKeyTargetSpaceId.String())
		if id != "" {
			ids = append(ids, id)
		}
		return true
	})

	log.Warnf("ft space ids priority: %v", ids)
	return ids
}

func (i *indexer) updateSpacesPriority(priority []string) {
	techSpaceId := i.techSpaceId.TechSpaceId()

	priority = append([]string{techSpaceId}, slices.DeleteFunc(priority, func(s string) bool {
		return s == techSpaceId
	})...)
	log.Warnf("update spaces priority: %v", priority)

	i.spaceReindexQueue.UpdatePriority(priority)
}

func (i *indexer) subscribeToSpaces() error {
	objectReq := subscription.SubscribeRequest{
		SubId:             fmt.Sprintf("lastOpenedSpaces"),
		Internal:          true,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyTargetSpaceId.String(), bundle.RelationKeyLastOpenedDate.String(), bundle.RelationKeyLastModifiedDate.String()},
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey:    bundle.RelationKeyLastOpenedDate.String(),
				Type:           model.BlockContentDataviewSort_Desc,
				IncludeTime:    true,
				Format:         model.RelationFormat_date,
				EmptyPlacement: model.BlockContentDataviewSort_End,
			},
		},
	}
	i.lastSpacesSubscription = syncsubscriptions.NewSubscription(i.subscriptionService, objectReq)
	return i.lastSpacesSubscription.Run(i.lastSpacesSubscriptionUpdateChan)
}

func (i *indexer) getIterator() func(id string, data struct{}) bool {
	var ids []string
	return func(id string, _ struct{}) bool {
		ids = append(ids, id)
		return true
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

	log.Warnf("start ft queue processor")
	i.runFullTextIndexer(ctx, i.getSpaceIdsByPriority())
	defer close(i.ftQueueFinished)
	var lastForceIndex time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			i.runFullTextIndexer(ctx, i.getSpaceIdsByPriority())
		case <-i.forceFt:
			if time.Since(lastForceIndex) > ftIndexForceMinInterval {
				i.runFullTextIndexer(ctx, i.getSpaceIdsByPriority())
				lastForceIndex = time.Now()
			}
		}
	}
}

func (i *indexer) runFullTextIndexer(ctx context.Context, spaceIdsPriority []string) {
	batcher := i.ftsearch.NewAutoBatcher(ftsearch.AutoBatcherRecommendedMaxDocs, ftsearch.AutoBatcherRecommendedMaxSize)
	err := i.store.BatchProcessFullTextQueue(ctx, spaceIdsPriority, ftBatchLimit, func(objectIds []string) error {
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
			spaceIds, err := i.storageService.AllSpaceIds()
			if err != nil {
				return err
			}
			var fullIds []domain.FullID
			for _, spaceId := range spaceIds {
				ids, err := i.store.ListIdsBySpace(spaceId)
				if err != nil {
					return err
				}
				for _, id := range ids {
					fullIds = append(fullIds, domain.FullID{
						ObjectID: id,
						SpaceID:  spaceId,
					})
				}
			}
			err = i.store.AddToIndexQueue(fullIds...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
