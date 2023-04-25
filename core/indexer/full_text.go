package indexer

import (
	"context"
	"time"

	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func (i *indexer) ForceFTIndex() {
	select {
	case i.forceFt <- struct{}{}:
	default:
	}
}

func (i *indexer) ftLoop() {
	ticker := time.NewTicker(ftIndexInterval)
	i.ftIndex()
	var lastForceIndex time.Time
	i.mu.Lock()
	quit := i.quit
	i.mu.Unlock()
	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
			i.ftIndex()
		case <-i.forceFt:
			if time.Since(lastForceIndex) > ftIndexForceMinInterval {
				i.ftIndex()
				lastForceIndex = time.Now()
			}
		}
	}
}

func (i *indexer) ftIndex() {
	if err := i.store.IndexForEach(i.ftIndexDoc); err != nil {
		log.Errorf("store.IndexForEach error: %v", err)
	}
}

func (i *indexer) ftIndexDoc(id string, _ time.Time) (err error) {
	st := time.Now()
	// ctx := context.WithValue(context.Background(), ocache.CacheTimeout, cacheTimeout)
	ctx := context.WithValue(context.Background(), metrics.CtxKeyRequest, "index_fulltext")

	info, err := i.doc.GetDocInfo(ctx, id)
	if err != nil {
		return
	}

	sbType, err := i.typeProvider.Type(info.Id)
	if err != nil {
		sbType = smartblock.SmartBlockTypePage
	}
	indexDetails, _ := sbType.Indexable()
	if !indexDetails {
		return nil
	}

	if err = i.store.UpdateObjectSnippet(id, info.State.Snippet()); err != nil {
		return
	}

	if len(info.FileHashes) > 0 {
		// todo: move file indexing to the main indexer as we have  the full state there now
		existingIDs, err := i.store.HasIDs(info.FileHashes...)
		if err != nil {
			log.Errorf("failed to get existing file ids : %s", err.Error())
		}
		newIds := slice.Difference(info.FileHashes, existingIDs)
		for _, hash := range newIds {
			// file's hash is id
			err = i.reindexDoc(ctx, hash, false)
			if err != nil {
				log.With("id", hash).Errorf("failed to reindex file: %s", err.Error())
			}

			err = i.store.AddToIndexQueue(hash)
			if err != nil {
				log.With("id", hash).Error(err.Error())
			}
		}
	}

	if fts := i.store.FTSearch(); fts != nil {
		title := pbtypes.GetString(info.State.Details(), bundle.RelationKeyName.String())
		if info.State.ObjectType() == bundle.TypeKeyNote.String() || title == "" {
			title = info.State.Snippet()
		}
		ftDoc := ftsearch.SearchDoc{
			Id:    id,
			Title: title,
			Text:  info.State.SearchText(),
		}
		if err := fts.Index(ftDoc); err != nil {
			log.Errorf("can't ft index doc: %v", err)
		}
		log.Debugf("ft search indexed with title: '%s'", ftDoc.Title)
	}

	log.With("thread", id).Infof("ft index updated for a %v", time.Since(st))
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
