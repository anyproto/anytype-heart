package indexer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/block"
	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

func (i *indexer) ftLoop() {
	ticker := time.NewTicker(ftIndexInterval)
	i.runFullTextIndexer()
	var lastForceIndex time.Time
	for {
		select {
		case <-i.quit:
			return
		case <-ticker.C:
			i.runFullTextIndexer()
		case <-i.forceFt:
			if time.Since(lastForceIndex) > ftIndexForceMinInterval {
				i.runFullTextIndexer()
				lastForceIndex = time.Now()
			}
		}
	}
}

// TODO maybe use two queues? One for objects, one for files
func (i *indexer) runFullTextIndexer() {
	docs := make([]ftsearch.SearchDoc, 0, ftBatchLimit)
	err := i.store.BatchProcessFullTextQueue(ftBatchLimit, func(ids []string) error {
		for _, id := range ids {
			err := i.prepareSearchDocument(id, func(doc ftsearch.SearchDoc) error {
				docs = append(docs, doc)
				if len(docs) >= ftBatchLimit {
					err := i.ftsearch.BatchIndex(docs)
					docs = docs[:0]
					if err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		if len(docs) > 0 {
			return i.ftsearch.BatchIndex(docs)
		}
		return nil
	})

	if err != nil {
		log.Errorf("list ids from full-text queue: %v", err)
		return
	}
}

func (i *indexer) prepareSearchDocument(id string, processor func(doc ftsearch.SearchDoc) error) (err error) {
	// ctx := context.WithValue(context.Background(), ocache.CacheTimeout, cacheTimeout)
	ctx := context.WithValue(context.Background(), metrics.CtxKeyEntrypoint, "index_fulltext")
	objectPath := domain.NewFromPath(id)

	err = block.DoContext(i.picker, ctx, objectPath.ObjectId, func(sb smartblock2.SmartBlock) error {
		indexDetails, _ := sb.Type().Indexable()
		if !indexDetails {
			return nil
		}

		if err = i.store.UpdateObjectSnippet(id, sb.Snippet()); err != nil {
			return fmt.Errorf("update object snippet: %w", err)
		}

		title := pbtypes.GetString(sb.Details(), bundle.RelationKeyName.String())
		if sb.ObjectTypeKey() == bundle.TypeKeyNote || title == "" {
			title = sb.Snippet()
		}

		for _, rel := range sb.GetRelationLinks() {
			if objectPath.HasRelation() && rel.Key != objectPath.RelationKey {
				continue
			}
			if rel.Format != model.RelationFormat_shorttext && rel.Format != model.RelationFormat_longtext {
				continue
			}
			val := pbtypes.GetString(sb.Details(), rel.Key)
			if val == "" {
				continue
			}

			f := ftsearch.SearchDoc{
				Id:      domain.NewObjectPathWithRelation(id, rel.Key).String(),
				SpaceID: sb.SpaceID(),
				Text:    val,
			}
			if rel.Key == bundle.RelationKeyName.String() {
				f.Title = val
			}
			err = processor(f)
			if err != nil {
				return fmt.Errorf("process relation: %w", err)
			}
		}

		sb.Iterate(func(b simple.Block) (isContinue bool) {
			if objectPath.HasBlock() && b.Model().Id != objectPath.BlockId {
				return true
			}

			if tb := b.Model().GetText(); tb != nil {
				if len(strings.TrimSpace(tb.Text)) == 0 {
					return true
				}

				doc := ftsearch.SearchDoc{
					Id:      domain.NewObjectPathWithBlock(id, b.Model().Id).String(),
					SpaceID: sb.SpaceID(),
				}
				if len(doc.Text) > ftBlockMaxSize {
					doc.Text = doc.Text[0:ftBlockMaxSize]
				} else {
					doc.Text = tb.Text
				}
				err = processor(doc)
				if err != nil {
					log.Errorf("process block: %v", err)
					return false
				}
			}
			return true
		})

		return nil
	})
	return
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
