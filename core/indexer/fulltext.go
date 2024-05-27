package indexer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

// TODO maybe use two queues? One for objects, one for files
func (i *indexer) runFullTextIndexer(ctx context.Context) {
	docs := make([]ftsearch.SearchDoc, 0, ftBatchLimit)
	err := i.store.BatchProcessFullTextQueue(ctx, ftBatchLimit, func(ids []string) error {
		for _, id := range ids {
			err := i.ftsearch.Delete(id)
			if err != nil {
				log.With("id", id).Errorf("delete document for full-text indexing: %s", err)
			}
			err = i.prepareSearchDocument(ctx, id, func(doc ftsearch.SearchDoc) error {
				docs = append(docs, doc)
				if len(docs) >= ftBatchLimit {
					err := i.ftsearch.BatchIndex(ctx, docs)
					docs = docs[:0]
					if err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				// in the most cases it's about the files that were deleted
				// should be fixed with files as objects project
				// todo: research errors
				log.With("id", id).Errorf("prepare document for full-text indexing: %s", err)
				if errors.Is(err, context.Canceled) {
					return err
				}
				continue
			}
		}
		if len(docs) > 0 {
			return i.ftsearch.BatchIndex(ctx, docs)
		}
		return nil
	})

	if err != nil {
		log.Errorf("list ids from full-text queue: %v", err)
		return
	}
}

func (i *indexer) prepareSearchDocument(ctx context.Context, id string, processor func(doc ftsearch.SearchDoc) error) (err error) {
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

			f := ftsearch.SearchDoc{
				Id:      domain.NewObjectPathWithRelation(id, rel.Key).String(),
				DocId:   id,
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
					DocId:   id,
					Id:      domain.NewObjectPathWithBlock(id, b.Model().Id).String(),
					SpaceID: sb.SpaceID(),
				}
				if len(tb.Text) > ftBlockMaxSize {
					doc.Text = tb.Text[:ftBlockMaxSize]
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
