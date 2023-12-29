package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/block"
	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ftIndexInterval         = 10 * time.Second
	ftIndexForceMinInterval = time.Second * 10
	ftBatchLimit            = 100
	ftsTitleMaxSize         = 1024
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
			doc, err := i.prepareSearchDocument(id)
			if err != nil {
				log.With("id", id).Errorf("prepare document for full-text indexing: %s", err)
				continue
			}
			docs = append(docs, doc)
		}

		err := i.ftsearch.BatchIndex(docs)
		docs = docs[:0]
		if err != nil {
			log.Errorf("full-text indexing: %v", err)
		}
		return err
	})

	if err != nil {
		log.Errorf("list ids from full-text queue: %v", err)
		return
	}
}

func (i *indexer) prepareSearchDocument(id string) (ftDoc ftsearch.SearchDoc, err error) {
	// ctx := context.WithValue(context.Background(), ocache.CacheTimeout, cacheTimeout)
	ctx := context.WithValue(context.Background(), metrics.CtxKeyEntrypoint, "index_fulltext")
	err = block.DoContext(i.picker, ctx, id, func(sb smartblock2.SmartBlock) error {
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
		runes := []rune(title)
		if len([]rune(title)) > ftsTitleMaxSize {
			title = string(runes[:ftsTitleMaxSize])
		}

		ftDoc = ftsearch.SearchDoc{
			Id:      id,
			SpaceID: sb.SpaceID(),
			Title:   title,
			Text:    sb.SearchText(),
		}
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
