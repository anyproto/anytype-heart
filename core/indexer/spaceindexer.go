package indexer

import (
	"context"
	"time"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/util/hash"
)

type spaceIndexer struct {
	runCtx         context.Context
	spaceIndex     spaceindex.Store
	objectStore    objectstore.ObjectStore
	storageService storage.ClientStorage
	batcher        *mb.MB[indexTask]
}

func newSpaceIndexer(runCtx context.Context, spaceIndex spaceindex.Store, objectStore objectstore.ObjectStore, storageService storage.ClientStorage) *spaceIndexer {
	ind := &spaceIndexer{
		runCtx:         runCtx,
		spaceIndex:     spaceIndex,
		objectStore:    objectStore,
		storageService: storageService,
		batcher:        mb.New[indexTask](100),
	}
	go ind.indexBatchLoop()
	return ind
}

func (i *spaceIndexer) close() error {
	return i.batcher.Close()
}

type indexTask struct {
	info    smartblock.DocInfo
	options []smartblock.IndexOption
	done    chan error
}

func (i *spaceIndexer) indexBatchLoop() {
	for {
		tasks, err := i.batcher.Wait(i.runCtx)
		if err != nil {
			return
		}
		if iErr := i.indexBatch(tasks); iErr != nil {
			log.Warnf("indexBatch error: %v", iErr)
		}
	}
}

func (i *spaceIndexer) indexBatch(tasks []indexTask) (err error) {
	tx, err := i.spaceIndex.WriteTx(i.runCtx)
	if err != nil {
		return err
	}
	st := time.Now()

	closeTasks := func(closeErr error) {
		for _, t := range tasks {
			if closeErr != nil {
				select {
				case t.done <- closeErr:
				default:
				}
			} else {
				close(t.done)
			}
		}
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			if err = tx.Commit(); err != nil {
				closeTasks(err)
			} else {
				closeTasks(nil)
			}
			log.Infof("indexBatch: indexed %d docs for a %v: err: %v", len(tasks), time.Since(st), err)
		}
	}()

	for _, task := range tasks {
		if iErr := i.index(tx.Context(), task.info, task.options...); iErr != nil {
			task.done <- iErr
		}
	}
	return
}

func (i *spaceIndexer) Index(info smartblock.DocInfo, options ...smartblock.IndexOption) error {
	done := make(chan error)
	if err := i.batcher.Add(i.runCtx, indexTask{
		info:    info,
		options: options,
		done:    done,
	}); err != nil {
		return err
	}
	select {
	case <-i.runCtx.Done():
		return i.runCtx.Err()
	case err := <-done:
		return err
	}
}

func (i *spaceIndexer) index(ctx context.Context, info smartblock.DocInfo, options ...smartblock.IndexOption) error {
	// options are stored in smartblock pkg because of cyclic dependency :(
	startTime := time.Now()
	opts := &smartblock.IndexOptions{}
	for _, o := range options {
		o(opts)
	}
	err := i.storageService.BindSpaceID(info.Space.Id(), info.Id)
	if err != nil {
		log.Error("failed to bind space id", zap.Error(err), zap.String("id", info.Id))
		return err
	}
	headHashToIndex := hash.HeadsHash(info.Heads)
	saveIndexedHash := func() {
		if headHashToIndex == "" {
			return
		}

		err = i.spaceIndex.SaveLastIndexedHeadsHash(ctx, info.Id, headHashToIndex)
		if err != nil {
			log.With("objectID", info.Id).Errorf("failed to save indexed heads hash: %v", err)
		}
	}

	indexDetails, indexLinks := info.SmartblockType.Indexable()
	if !indexDetails && !indexLinks {
		return nil
	}

	lastIndexedHash, err := i.spaceIndex.GetLastIndexedHeadsHash(ctx, info.Id)
	if err != nil {
		log.With("object", info.Id).Errorf("failed to get last indexed heads hash: %v", err)
	}

	if opts.SkipIfHeadsNotChanged {
		if headHashToIndex == "" {
			log.With("objectID", info.Id).Errorf("heads hash is empty")
		} else if lastIndexedHash == headHashToIndex {
			log.With("objectID", info.Id).Debugf("heads not changed, skipping indexing")
			return nil
		}
	}

	details := info.Details

	indexSetTime := time.Now()
	var hasError bool
	if indexLinks {
		if err = i.spaceIndex.UpdateObjectLinks(ctx, info.Id, info.Links); err != nil {
			hasError = true
			log.With("objectID", info.Id).Errorf("failed to save object links: %v", err)
		}
	}

	indexLinksTime := time.Now()
	if indexDetails {
		if err := i.spaceIndex.UpdateObjectDetails(ctx, info.Id, details); err != nil {
			hasError = true
			log.With("objectID", info.Id).Errorf("can't update object store: %v", err)
		} else if lastIndexedHash == headHashToIndex {
			l := log.With("objectID", info.Id).
				With("hashesAreEqual", lastIndexedHash == headHashToIndex).
				With("lastHashIsEmpty", lastIndexedHash == "").
				With("skipFlagSet", opts.SkipIfHeadsNotChanged)

			if opts.SkipIfHeadsNotChanged {
				l.Warnf("details have changed, but heads are equal")
			} else {
				l.Debugf("details have changed, but heads are equal")
			}
		}

		if !(opts.SkipFullTextIfHeadsNotChanged && lastIndexedHash == headHashToIndex) {
			// Use component's context because ctx from parameter contains transaction
			if err := i.objectStore.AddToIndexQueue(i.runCtx, info.Id); err != nil {
				log.With("objectID", info.Id).Errorf("can't add id to index queue: %v", err)
			}
		}
	} else {
		_ = i.spaceIndex.DeleteDetails(ctx, []string{info.Id})
	}
	indexDetailsTime := time.Now()
	detailsCount := details.Len()

	if !hasError {
		saveIndexedHash()
	}

	metrics.Service.Send(&metrics.IndexEvent{
		ObjectId:                info.Id,
		IndexLinksTimeMs:        indexLinksTime.Sub(indexSetTime).Milliseconds(),
		IndexDetailsTimeMs:      indexDetailsTime.Sub(indexLinksTime).Milliseconds(),
		IndexSetRelationsTimeMs: indexSetTime.Sub(startTime).Milliseconds(),
		DetailsCount:            detailsCount,
	})

	return nil
}
