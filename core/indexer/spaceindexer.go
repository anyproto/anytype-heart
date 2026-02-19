package indexer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

type spaceIndexer struct {
	runCtx          context.Context
	spaceIndex      spaceindex.Store
	objectStore     objectstore.ObjectStore
	batcher         *mb.MB[indexTask]
	fulltextEnabled bool

	lastIndex atomic.Time
}

func newSpaceIndexer(runCtx context.Context, spaceIndex spaceindex.Store, objectStore objectstore.ObjectStore, fulltextEnabled bool) *spaceIndexer {
	ind := &spaceIndexer{
		runCtx:          runCtx,
		spaceIndex:      spaceIndex,
		objectStore:     objectStore,
		batcher:         mb.New[indexTask](1000),
		fulltextEnabled: fulltextEnabled,
	}
	go ind.indexBatchLoop()
	return ind
}

// LastIndex returns the time of the last indexing operation
func (i *spaceIndexer) LastIndex() time.Time {
	return i.lastIndex.Load()
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
		// todo: add some delay in case batcher returns too many in minute to avoid high cpu/disk usage
	}
}

func (i *spaceIndexer) indexBatch(tasks []indexTask) (err error) {
	if len(tasks) == 0 {
		return nil
	}
	tx, err := i.spaceIndex.WriteTx(i.runCtx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
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

	fulltextIds := make([]domain.FullID, 0, len(tasks))

	ftQueueCtr := objectstore.GenerateFTQueueCounter()
	for _, task := range tasks {
		if addToFtQueue, iErr := i.index(tx.Context(), ftQueueCtr, task.info, task.options...); iErr != nil {
			task.done <- iErr
		} else if addToFtQueue {
			fulltextIds = append(fulltextIds, domain.FullID{ObjectID: task.info.Id, SpaceID: task.info.Space.Id()})
		}
	}

	commonTx, err := i.objectStore.WriteTx(i.runCtx)
	if err != nil {
		return err
	}
	defer func() {
		_ = commonTx.Rollback()
	}()

	for _, task := range tasks {
		err = i.objectStore.BindSpaceId(commonTx.Context(), task.info.Space.Id(), task.info.Id)
		if err != nil {
			log.Error("failed to bind space id", zap.Error(err), zap.String("id", task.info.Id))
			return err
		}
	}

	if len(fulltextIds) > 0 {
		_, err = i.objectStore.AddToIndexQueueWithCounter(commonTx.Context(), ftQueueCtr, fulltextIds...)
		if err != nil {
			log.Errorf("can't add ids to index queue: %v", err)
		}
	}
	if err = tx.Commit(); err != nil {
		closeTasks(err)
	} else if commonTx.Commit() != nil {
		closeTasks(fmt.Errorf("failed to commit commondb tx: %w", err))
	} else {
		closeTasks(nil)
	}
	log.With("spaceId", i.spaceIndex.SpaceId()).Infof("indexBatch: indexed %d docs for a %v: err: %v", len(tasks), time.Since(st), err)
	i.lastIndex.Store(time.Now())
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

func (i *spaceIndexer) index(ctx context.Context, ftQueueCounter uint64, info smartblock.DocInfo, options ...smartblock.IndexOption) (addToFulltextQueue bool, err error) {
	// options are stored in smartblock pkg because of cyclic dependency :(
	opts := &smartblock.IndexOptions{}
	for _, o := range options {
		o(opts)
	}

	headHashToIndex := headsHash(info.Heads)

	saveIndexedHash := func() {
		if headHashToIndex == "" {
			return
		}

		// If we have ftQueueCtr, use the method that saves it for crash recovery
		if ftQueueCounter > 0 {
			err = i.spaceIndex.SaveLastIndexedHeadsHashWithFtQueueCtr(ctx, info.Id, headHashToIndex, ftQueueCounter)
		} else {
			err = i.spaceIndex.SaveLastIndexedHeadsHash(ctx, info.Id, headHashToIndex)
		}
		if err != nil {
			log.With("objectID", info.Id).Errorf("failed to save indexed heads hash: %v", err)
		}
	}

	fulltext, indexDetails, indexLinks := info.SmartblockType.Indexable()
	if !indexDetails && !indexLinks {
		return false, nil
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
			return false, nil
		}
	}

	details := info.Details

	var hasError bool
	if indexLinks {
		// Convert smartblock.OutgoingLink to spaceindex.OutgoingLink
		var spaceIndexLinks []spaceindex.OutgoingLink
		if len(info.OutgoingLinks) > 0 {
			spaceIndexLinks = make([]spaceindex.OutgoingLink, len(info.OutgoingLinks))
			for i, link := range info.OutgoingLinks {
				spaceIndexLinks[i] = spaceindex.OutgoingLink{
					TargetID:    link.TargetID,
					BlockID:     link.SourceBlockID,
					RelationKey: link.RelationKey,
				}
			}
			if err = i.spaceIndex.UpdateObjectLinksDetailed(ctx, info.Id, spaceIndexLinks); err != nil {
				hasError = true
				log.With("objectID", info.Id).Errorf("failed to save detailed object links: %v", err)
			}
		} else {
			// Fallback to simple links for backward compatibility
			if err = i.spaceIndex.UpdateObjectLinks(ctx, info.Id, info.Links); err != nil {
				hasError = true
				log.With("objectID", info.Id).Errorf("failed to save object links: %v", err)
			}
		}
	}

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

		if (!opts.SkipFullTextIfHeadsNotChanged || lastIndexedHash != headHashToIndex) && fulltext && i.fulltextEnabled {
			// Use component's context because ctx from parameter contains transaction
			// Get counter from AddToIndexQueueWithCounter for crash recovery consistency
			addToFulltextQueue = true
		}
	} else {
		_ = i.spaceIndex.DeleteDetails(ctx, []string{info.Id})
	}

	if !hasError {
		saveIndexedHash()
	}

	return addToFulltextQueue, nil
}

func headsHash(heads []string) string {
	if len(heads) == 0 {
		return ""
	}
	slices.Sort(heads)

	sum := sha256.Sum256([]byte(strings.Join(heads, ",")))
	return fmt.Sprintf("%x", sum)
}
