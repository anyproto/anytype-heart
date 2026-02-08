package indexer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var (
	ftIndexInterval                     = 1 * time.Second
	ftMaxIndexInterval                  = time.Second * 32
	ftIndexForceMinInterval             = time.Second * 10
	ftInconsistencyCheckStartDelay      = time.Second * 10
	ftBatchLimit                   uint = 1000
	ftBlockMaxSize                      = 1024 * 1024
	maxErrSent                     atomic.Int32
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
func (i *indexer) ftLoopRoutine(ctx context.Context) {
	tickerDuration := ftIndexInterval
	ticker := time.NewTicker(tickerDuration)

	var err error
	i.ftsearchLastIndexSeq, err = i.ftsearch.LastDbState()
	if err != nil {
		log.Errorf("get last db state: %v", err)
	} else {
		err = i.store.FtQueueReconcileWithSeq(ctx, i.ftsearchLastIndexSeq)
		if err != nil {
			log.Errorf("readd after ft seq: %v", err)
		}
	}
	prevError := i.runFullTextIndexer(ctx)
	defer close(i.ftQueueFinished)
	var lastForceIndex time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := i.runFullTextIndexer(ctx)
			if err != nil {
				if prevError != nil {
					// we have an error in the previous run
					// double the ticker duration, but not more than ftMaxIndexInterval
					if tickerDuration*2 <= ftMaxIndexInterval {
						tickerDuration *= 2
						ticker.Reset(tickerDuration)
					}
				}
			} else if tickerDuration != ftIndexInterval {
				// reset ticker to the initial value
				tickerDuration = ftIndexInterval
				ticker.Reset(tickerDuration)
			}
			prevError = err
		case <-i.forceFt:
			if time.Since(lastForceIndex) > ftIndexForceMinInterval {
				prevError = i.runFullTextIndexer(ctx)
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
	return lo.Keys(i.spaces)
}

func (i *indexer) runFullTextIndexer(ctx context.Context) error {
	batcher := i.ftsearch.NewAutoBatcher()

	var processQueuedObjects = func(objects []domain.FullTextQueuedObject) (succeedIds []domain.FullID, ftIndexSeq uint64, err error) {
		if len(objects) == 0 {
			return nil, 0, nil
		}

		for _, object := range objects {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			default:
			}
			objDocs, isChat, err := i.prepareSearchDocs(ctx, object)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil, 0, err
				}
				if !errors.Is(err, domain.ErrObjectNotFound) &&
					!errors.Is(err, space.ErrSpaceNotExists) &&
					!errors.Is(err, treestorage.ErrUnknownTreeId) &&
					!errors.Is(err, sourceimpl.ErrSpaceWithoutTreeBuilder) && // rare error because of marketplace
					!errors.Is(err, editor.ErrUnexpectedSmartblockType) && // this version doesn't support some new smartblocktype
					!errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
					// some error that doesn't mean object is no longer exists
					log.With("id", object.FullId()).Errorf("prepare document for full-text indexing: %s", err)
					continue
				}
			}

			removedDocIds := make([]string, len(object.DeletedMsgIds))
			if !isChat {
				objDocs, removedDocIds, err = i.filterOutNotChangedDocuments(object.ObjectId, objDocs)
			}

			// Add deleted message IDs from the queue to the removal list
			if len(object.DeletedMsgIds) > 0 {
				for _, msgId := range object.DeletedMsgIds {
					docId := domain.NewObjectPathWithMessage(object.ObjectId, msgId).String()
					removedDocIds = append(removedDocIds, docId)
				}
			}
			if err != nil {
				log.With("id", object.FullId()).Errorf("filter not changed error:: %s", err)
				// try to process the other returned values.
				continue
			}
			for _, removeId := range removedDocIds {
				err = batcher.DeleteDoc(removeId)
				if err != nil {
					return nil, 0, fmt.Errorf("batcher delete: %w", err)
				}
			}

			for _, doc := range objDocs {
				err = batcher.UpsertDoc(doc)
				if err != nil {
					if strings.Contains(err.Error(), "invalid utf-8 sequence") {
						log.With("id", object.ObjectId).Warnf(err.Error())
						continue // skip this document
					}
					return nil, 0, fmt.Errorf("batcher add: %w", err)
				}
			}

			succeedIds = append(succeedIds, object.FullId())
		}

		ftIndexSeq, err = batcher.Finish()
		if err != nil {
			return nil, 0, fmt.Errorf("finish batch failed: %w", err)
		}
		if ftIndexSeq > 0 {
			// we can have 0 ftIndexSeq if all documents were filtered-out as not changed, so the batch were empty
			// as a result of this filter-out workaround we can return newer Seq for some objects and persist them in the queue
			// but it's not a big problem, in case of db corruption we may just try to reindex more objects than needed
			i.ftsearchLastIndexSeq = ftIndexSeq
		}
		return succeedIds, i.ftsearchLastIndexSeq, nil
	}

	err := i.store.BatchProcessFullTextQueue(i.activeSpaces, ftBatchLimit, processQueuedObjects)
	if err != nil && maxErrSent.Load() < maxErrorsPerSession {
		maxErrSent.Add(1)
		log.Errorf("list ids from full-text queue: %v", err)
	}

	// When queue processing succeeds (queue is empty or fully processed),
	// check if we need to run the FT consistency check
	if err == nil {
		i.maybeRunFTConsistencyCheck(ctx)
	}

	return err
}

func (i *indexer) filterOutNotChangedDocuments(id string, newDocs []ftsearch.SearchDoc) (changed []ftsearch.SearchDoc, removedIds []string, err error) {
	var (
		changedDocs []ftsearch.SearchDoc
		removeDocs  []string
		// todo: return new docs as a separate slice so we can avoid deletion operations in tantivy
	)

	var fields []string
	if len(newDocs) > 0 {
		// no need to query fields if we have no documents to compare (means the whole object is deleted)
		fields = []string{"Title", "Text"}
	}
	err = i.ftsearch.Iterate(id, fields, func(doc *ftsearch.SearchDoc) bool {
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

func (i *indexer) prepareSearchDocs(ctx context.Context, object domain.FullTextQueuedObject) (docs []ftsearch.SearchDoc, isChat bool, err error) {
	// shortcut for deleted objects via objectstore
	// otherwise we can have race condition when object is marked as deleted but the tree is not yet deleted
	details, err := i.store.SpaceIndex(object.SpaceId).GetDetails(object.ObjectId)
	if err != nil {
		log.With("id", object.FullId()).Errorf("prepareSearchDocs: get details: %v", err)
	} else if details.GetBool(bundle.RelationKeyIsDeleted) {
		// object is deleted, no need to index it
		return
	}

	ctx = context.WithValue(ctx, metrics.CtxKeyEntrypoint, "index_fulltext")

	if object.MsgOrderId != "" || len(object.DeletedMsgIds) > 0 {
		docs, err = i.prepareChatSearchDocs(ctx, object)
		if err != nil {
			return nil, true, err
		}
		return docs, true, nil
	}

	var fulltextSkipped bool

	err = cache.DoContextFullID(i.picker, ctx, domain.FullID{SpaceID: object.SpaceId, ObjectID: object.ObjectId}, func(sb smartblock.SmartBlock) error {
		sbType := sb.Type()
		isChat = sbType == coresb.SmartBlockTypeChatDerivedObject
		fulltext, _, _ := sbType.Indexable()
		if !fulltext {
			fulltextSkipped = true
			return nil
		}

		for _, key := range sb.AllRelationKeys() {
			format, err := i.formatFetcher.GetRelationFormatByKey(object.SpaceId, key)
			if err == nil && format != model.RelationFormat_shorttext && format != model.RelationFormat_longtext {
				continue
			}
			val := sb.Details().GetString(key)
			if val == "" {
				val = sb.LocalDetails().GetString(key)
				if val == "" {
					continue
				}
			}
			// skip readonly and hidden system relations
			if bundledRel, err := bundle.PickRelation(key); err == nil {
				layout, _ := sb.Layout()
				skip := bundledRel.ReadOnly || bundledRel.Hidden
				if isName(key) {
					skip = false
				}
				if layout == model.ObjectType_note && key == bundle.RelationKeySnippet {
					// index snippet only for notes, so we will be able to do fast prefix queries
					skip = false
				}

				if skip {
					continue
				}
			}

			doc := ftsearch.SearchDoc{
				Id:      domain.NewObjectPathWithRelation(object.ObjectId, key.String()).String(),
				SpaceId: sb.SpaceID(),
				Text:    val,
			}

			if isName(key) {
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
					Id:      domain.NewObjectPathWithBlock(object.ObjectId, b.Model().Id).String(),
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
		return docs, isChat, nil
	}
	if err != nil {
		return nil, isChat, err
	}
	_, cacheErr := i.picker.TryRemoveFromCache(ctx, object.ObjectId)
	if cacheErr != nil &&
		!errors.Is(err, domain.ErrObjectNotFound) {
		log.With("objectId", object.FullId()).Errorf("object cache remove: %v", err)
	}

	return docs, isChat, nil
}

func (i *indexer) prepareChatSearchDocs(ctx context.Context, object domain.FullTextQueuedObject) (docs []ftsearch.SearchDoc, err error) {
	repository, err := i.chatRepository.Repository(object.SpaceId, object.ObjectId)
	if err != nil {
		return nil, fmt.Errorf("prepareChatSearchDocs: failed to get chat repository: %w", err)
	}

	var msgs []*chatmodel.Message
	switch object.MsgOrderId {
	case objectstore.FtAllOrderId:
		// TODO: GO-6758 add batch messages fetch by limits
		msgs, err = repository.GetMessages(ctx, chatrepository.GetMessagesRequest{})
	case "":
		return nil, nil // no new search docs should be added
	default:
		msgs, err = repository.GetMessagesForIndexing(ctx, object.MsgOrderId)
	}

	if err != nil {
		return nil, fmt.Errorf("prepareChatSearchDocs: failed to get messages for indexing: %w", err)
	}

	for _, msg := range msgs {
		docs = append(docs, ftsearch.SearchDoc{
			Id:        domain.NewObjectPathWithMessage(object.ObjectId, msg.Id).String(),
			SpaceId:   object.SpaceId,
			Text:      msg.Message.Text,
			Author:    msg.Creator,
			OrderId:   msg.OrderId,
			MessageId: msg.Id,
			Timestamp: strconv.Itoa(int(msg.CreatedAt)),
		})
	}
	return docs, nil

}

func isName(key domain.RelationKey) bool {
	return key == bundle.RelationKeyName || key == bundle.RelationKeyPluralName
}

func (i *indexer) ftInit() error {
	if ft := i.ftsearch; ft != nil {
		docCount, err := ft.DocCount()
		if err != nil {
			return err
		}
		if docCount == 0 {
			// means db got removed, lets remove the queue and reindex all objects
			err = i.store.ClearFullTextQueue(nil)
			if err != nil {
				return err
			}
			// query objects that are existing in the store
			// if they are not existing in the object store, they will be indexed and added via reindexOutdatedObjects or on receiving via any-sync
			err = i.store.EnqueueAllForFulltextIndexing(i.runCtx)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// maybeRunFTConsistencyCheck runs a one-time consistency check when the FT queue becomes empty.
// It compares objects in the object store against the FT index and enqueues missing ones.
func (i *indexer) maybeRunFTConsistencyCheck(ctx context.Context) {
	// Only run once per session
	if i.ftConsistencyCheckDone.Load() {
		return
	}

	// Check if counter needs update
	currentCounter, err := i.store.GetFTRecheckCounter(ctx)
	if err != nil {
		log.Errorf("get ft recheck counter: %v", err)
		return
	}

	if currentCounter >= ForceFTRecheckCounter {
		i.ftConsistencyCheckDone.Store(true)
		return // already up to date
	}

	// Mark as started (prevent concurrent runs)
	if !i.ftConsistencyCheckDone.CompareAndSwap(false, true) {
		return
	}

	start := time.Now()
	checked, enqueued, err := i.store.RunFTConsistencyCheck(i.runCtx, i.ftsearch)
	if err != nil {
		log.Errorf("ft consistency check failed: %v", err)
		return
	}

	// Update counter only on success
	err = i.store.SetFTRecheckCounter(i.runCtx, ForceFTRecheckCounter)
	if err != nil {
		log.Errorf("save ft recheck counter: %v", err)
		return
	}

	var l = log.With("checked", checked, "enqueued", enqueued, "duration", time.Since(start))
	if enqueued > 0 {
		l.Warn("ft consistency check completed")
	} else {
		l.Info("ft consistency check completed")
	}
}
