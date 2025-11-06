package indexer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/migration"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

type spaceIndexer struct {
	runCtx          context.Context
	spaceIndex      spaceindex.Store
	objectStore     objectstore.ObjectStore
	contextMigration migration.ContextMigrationService
	batcher         *mb.MB[indexTask]
	fulltextEnabled bool
	isTechSpace      bool

	startedAt time.Time
	lastIndex atomic.Time
}

func newSpaceIndexer(runCtx context.Context, spaceIndex spaceindex.Store, objectStore objectstore.ObjectStore, fulltextEnabled bool, contextMigration migration.ContextMigrationService, isTechSpace bool)) *spaceIndexer {
	ind := &spaceIndexer{
		runCtx:          runCtx,
		spaceIndex:      spaceIndex,
		objectStore:     objectStore,
		batcher:         mb.New[indexTask](100),
		fulltextEnabled: fulltextEnabled,
		contextMigration: contextMigration,
		isTechSpace:      isTechSpace,
	}
	go ind.indexBatchLoop()
	go ind.indexFileCreationContext()
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

func (i *spaceIndexer) indexFileCreationContext() {
	if os.Getenv("ANYTYPE_CONTEXT_MIGRATION") != "1" {
		return
	}
	for {
		select {
		case <-time.After(time.Second * 10):
			lastIndex := i.lastIndex.Load()
			if time.Since(lastIndex) > time.Second*10 {
				err := i.contextMigration.MigrateSpace(i.runCtx, i.spaceIndex)
				if err != nil {
					log.With("spaceId", i.spaceIndex.SpaceId()).Errorf("failed to migrate context for space: %v", err)
				} else {
					log.With("spaceId", i.spaceIndex.SpaceId()).Warnf("context migration completed")
				}
				return
			}
		case <-i.runCtx.Done():
			return
		}
	}
}

func (i *spaceIndexer) indexBatch(tasks []indexTask) (err error) {
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

	for _, task := range tasks {
		if iErr := i.index(tx.Context(), task.info, task.options...); iErr != nil {
			task.done <- iErr
		}
	}
	if err = tx.Commit(); err != nil {
		closeTasks(err)
	} else {
		closeTasks(nil)
	}
	log.Infof("indexBatch: indexed %d docs for a %v: err: %v", len(tasks), time.Since(st), err)
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

func (i *spaceIndexer) index(ctx context.Context, info smartblock.DocInfo, options ...smartblock.IndexOption) error {
	// options are stored in smartblock pkg because of cyclic dependency :(
	opts := &smartblock.IndexOptions{}
	for _, o := range options {
		o(opts)
	}
	err := i.objectStore.BindSpaceId(info.Space.Id(), info.Id)
	if err != nil {
		log.Error("failed to bind space id", zap.Error(err), zap.String("id", info.Id))
		return err
	}
	headHashToIndex := headsHash(info.Heads)
	saveIndexedHash := func() {
		if headHashToIndex == "" {
			return
		}

		err = i.spaceIndex.SaveLastIndexedHeadsHash(ctx, info.Id, headHashToIndex)
		if err != nil {
			log.With("objectID", info.Id).Errorf("failed to save indexed heads hash: %v", err)
		}
	}

	_, indexDetails, indexLinks := info.SmartblockType.Indexable()
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
				fmt.Printf("Processing link: %s -> %s (block: %s, relation: %s)\n", info.Id, link.TargetID, link.SourceBlockID, link.RelationKey)
				if link.RelationKey == bundle.RelationKeyBacklinks.String() {
					fmt.Println()
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

		if !(opts.SkipFullTextIfHeadsNotChanged && lastIndexedHash == headHashToIndex) {
			// Use component's context because ctx from parameter contains transaction
			fulltext, _, _ := info.SmartblockType.Indexable()

			if fulltext && i.fulltextEnabled {
				if err := i.objectStore.AddToIndexQueue(i.runCtx, domain.FullID{ObjectID: info.Id, SpaceID: info.Space.Id()}); err != nil {
					log.With("objectID", info.Id).Errorf("can't add id to index queue: %v", err)
				}
			}
		}
	} else {
		_ = i.spaceIndex.DeleteDetails(ctx, []string{info.Id})
	}

	if !hasError {
		saveIndexedHash()
	}

	return nil
}

func headsHash(heads []string) string {
	if len(heads) == 0 {
		return ""
	}
	slices.Sort(heads)

	sum := sha256.Sum256([]byte(strings.Join(heads, ",")))
	return fmt.Sprintf("%x", sum)
}
