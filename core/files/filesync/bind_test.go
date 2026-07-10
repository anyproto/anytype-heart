package filesync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	rpcstore2 "github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

// Reproducers for the "Notion-imported files stuck in Syncing forever" bug
// (docs/FileSyncStuckQueued.md): a file object whose blocks are already on the
// file node (uploaded earlier, e.g. from another space or device) but absent
// from the local flat store must still reach Synced — by binding the cids the
// node already has, without downloading or re-uploading file data.

type statusRecorder struct {
	mu       sync.Mutex
	statuses map[string][]filesyncstatus.Status
}

func newStatusRecorder(fx *fixture) *statusRecorder {
	rec := &statusRecorder{statuses: map[string][]filesyncstatus.Status{}}
	fx.OnStatusUpdated(func(objectId string, _ domain.FullFileId, status filesyncstatus.Status) error {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.statuses[objectId] = append(rec.statuses[objectId], status)
		return nil
	})
	return rec
}

func (r *statusRecorder) lastStatus(objectId string) (filesyncstatus.Status, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	statuses := r.statuses[objectId]
	if len(statuses) == 0 {
		return 0, false
	}
	return statuses[len(statuses)-1], true
}

// collectDAGBlocks returns all blocks of a file DAG in walk order, root first
func (fx *fixture) collectDAGBlocks(t *testing.T, fileNode ipld.Node) []blocks.Block {
	rootBlock, err := blocks.NewBlockWithCid(fileNode.RawData(), fileNode.Cid())
	require.NoError(t, err)
	result := []blocks.Block{rootBlock}
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(fileNode, fx.fileService.DAGService()))
	err = walker.Iterate(func(navNode ipld.NavigableNode) error {
		node := navNode.GetIPLDNode()
		if node.Cid() == fileNode.Cid() {
			return nil
		}
		b, err := blocks.NewBlockWithCid(node.RawData(), node.Cid())
		if err != nil {
			return err
		}
		result = append(result, b)
		return nil
	})
	if !errors.Is(err, ipld.EndOfDag) {
		require.NoError(t, err)
	}
	return result
}

func (fx *fixture) waitItemState(t *testing.T, objectId string, wantState FileState, timeout time.Duration) FileInfo {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	it, err := fx.queue.GetNext(waitCtx, filequeue.GetNextRequest[FileInfo]{
		Subscribe:   true,
		StoreFilter: filterByState(wantState),
		Filter: func(info FileInfo) bool {
			return info.ObjectId == objectId && info.State == wantState
		},
	})
	require.NoError(t, err, "waiting for object %s to reach state %d", objectId, wantState)
	require.NoError(t, fx.queue.ReleaseAndUpdate(it.ObjectId, it))
	return it
}

func TestFileSync_BindFileAlreadyOnNode(t *testing.T) {
	t.Run("no local blocks at all: synced via bind, nothing uploaded or downloaded", func(t *testing.T) {
		// given
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		statuses := newStatusRecorder(fx)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		// A multi-chunk file: root file node + 3 leaf chunks
		fileId, fileNode := fx.givenFileAddedToDAG(t, 3*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)
		require.Greater(t, len(dagBlocks), 3, "expect a multi-block DAG")

		// Another client uploaded the same file to the node, under another space
		require.NoError(t, fx.rpcStore.AddToFile(ctx, "spaceOld", fileId, dagBlocks))
		blocksUploadedBefore := fx.rpcStore.Stats().BlocksAdded()

		// This device never had the blocks (dedup on import reuses the fileId
		// without writing blocks locally)
		for _, b := range dagBlocks {
			require.NoError(t, fx.localFileStorage.Delete(ctx, b.Cid()))
		}

		// when
		objectId := "fileObjectId1"
		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
			Imported:     true,
		}))

		// then
		fx.waitItemState(t, objectId, FileStateDone, 5*time.Second)

		gotStatus, ok := statuses.lastStatus(objectId)
		require.True(t, ok, "status update must be emitted")
		assert.Equal(t, filesyncstatus.Synced, gotStatus)

		// Nothing was uploaded: sync was confirmed by binding existing cids
		assert.Equal(t, blocksUploadedBefore, fx.rpcStore.Stats().BlocksAdded())

		// Every cid of the DAG is bound to the requesting space
		spaceInfo, err := fx.rpcStore.SpaceInfo(ctx, "space1")
		require.NoError(t, err)
		assert.Equal(t, uint64(len(dagBlocks)), spaceInfo.CidsCount)

		// Leaf data was not downloaded: only structural nodes may be fetched
		leafCids := blockCids(dagBlocks[1:])
		exists, err := fx.localFileStorage.ExistsCids(ctx, leafCids)
		require.NoError(t, err)
		assert.Empty(t, exists, "leaf chunks must not be downloaded to confirm sync")
	})

	t.Run("partial local blocks: synced via bind, nothing uploaded", func(t *testing.T) {
		// The state left by viewing a file: root and content chunks cached
		// locally, but some blocks (e.g. exif, thumbnail) never fetched
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		statuses := newStatusRecorder(fx)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 3*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)

		require.NoError(t, fx.rpcStore.AddToFile(ctx, "spaceOld", fileId, dagBlocks))
		blocksUploadedBefore := fx.rpcStore.Stats().BlocksAdded()

		// Remove one leaf locally, keep the rest
		require.NoError(t, fx.localFileStorage.Delete(ctx, dagBlocks[len(dagBlocks)-1].Cid()))

		objectId := "fileObjectId1"
		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
		}))

		fx.waitItemState(t, objectId, FileStateDone, 5*time.Second)

		gotStatus, ok := statuses.lastStatus(objectId)
		require.True(t, ok)
		assert.Equal(t, filesyncstatus.Synced, gotStatus)
		assert.Equal(t, blocksUploadedBefore, fx.rpcStore.Stats().BlocksAdded())
	})

	t.Run("more blocks than one availability/bind batch: every cid is checked and bound", func(t *testing.T) {
		// 66 MiB → 66 leaf chunks + root = 67 cids, crossing the 64-cid
		// batch boundary of both CheckAvailability and BindCids
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		statuses := newStatusRecorder(fx)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 66*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)
		require.Greater(t, len(dagBlocks), checkAvailabilityBatchSize)
		require.Greater(t, len(dagBlocks), bindBatchSize)

		require.NoError(t, fx.rpcStore.AddToFile(ctx, "spaceOld", fileId, dagBlocks))
		blocksUploadedBefore := fx.rpcStore.Stats().BlocksAdded()
		for _, b := range dagBlocks {
			require.NoError(t, fx.localFileStorage.Delete(ctx, b.Cid()))
		}

		objectId := "fileObjectId1"
		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
		}))

		fx.waitItemState(t, objectId, FileStateDone, 10*time.Second)

		gotStatus, ok := statuses.lastStatus(objectId)
		require.True(t, ok)
		assert.Equal(t, filesyncstatus.Synced, gotStatus)
		assert.Equal(t, blocksUploadedBefore, fx.rpcStore.Stats().BlocksAdded())

		spaceInfo, err := fx.rpcStore.SpaceInfo(ctx, "space1")
		require.NoError(t, err)
		assert.Equal(t, uint64(len(dagBlocks)), spaceInfo.CidsCount)
	})

	t.Run("already bound to the space: synced without binding or uploading anything", func(t *testing.T) {
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		statuses := newStatusRecorder(fx)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 3*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)

		// The file is already fully bound to the requesting space
		require.NoError(t, fx.rpcStore.AddToFile(ctx, "space1", fileId, dagBlocks))
		blocksUploadedBefore := fx.rpcStore.Stats().BlocksAdded()
		cidsBoundBefore := fx.rpcStore.Stats().CidsBinded()

		objectId := "fileObjectId1"
		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
		}))

		fx.waitItemState(t, objectId, FileStateDone, 5*time.Second)

		gotStatus, ok := statuses.lastStatus(objectId)
		require.True(t, ok)
		assert.Equal(t, filesyncstatus.Synced, gotStatus)
		assert.Equal(t, blocksUploadedBefore, fx.rpcStore.Stats().BlocksAdded())
		assert.Equal(t, cidsBoundBefore, fx.rpcStore.Stats().CidsBinded())
	})

	t.Run("cid omitted from the availability response is treated as missing and uploaded", func(t *testing.T) {
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 3*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)

		// Node has every block, but doesn't answer for one of them
		require.NoError(t, fx.rpcStore.AddToFile(ctx, "spaceOld", fileId, dagBlocks))
		blocksUploadedBefore := fx.rpcStore.Stats().BlocksAdded()
		fx.rpcStore.SetOmitFromAvailability(dagBlocks[len(dagBlocks)-1].Cid())

		objectId := "fileObjectId1"
		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
		}))

		fx.waitItemState(t, objectId, FileStateDone, 5*time.Second)

		// The unanswered cid was uploaded rather than silently counted as synced
		assert.Equal(t, blocksUploadedBefore+1, fx.rpcStore.Stats().BlocksAdded())
	})

	t.Run("mixed: node misses some blocks, they are uploaded from local store", func(t *testing.T) {
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		statuses := newStatusRecorder(fx)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 3*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)

		// Node has all blocks except the last leaf; all blocks are local
		require.NoError(t, fx.rpcStore.AddToFile(ctx, "spaceOld", fileId, dagBlocks[:len(dagBlocks)-1]))
		blocksUploadedBefore := fx.rpcStore.Stats().BlocksAdded()

		objectId := "fileObjectId1"
		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
		}))

		fx.waitItemState(t, objectId, FileStateDone, 5*time.Second)

		gotStatus, ok := statuses.lastStatus(objectId)
		require.True(t, ok)
		assert.Equal(t, filesyncstatus.Synced, gotStatus)

		// Only the block missing on the node was uploaded
		assert.Equal(t, blocksUploadedBefore+1, fx.rpcStore.Stats().BlocksAdded())

		spaceInfo, err := fx.rpcStore.SpaceInfo(ctx, "space1")
		require.NoError(t, err)
		assert.Equal(t, uint64(len(dagBlocks)), spaceInfo.CidsCount)
	})
}

func TestFileSync_MissingBlocksIsRetried(t *testing.T) {
	t.Run("blocks available nowhere: parked with future retry, then recovers when node gets the file", func(t *testing.T) {
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		statuses := newStatusRecorder(fx)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 3*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)

		// Blocks exist neither locally nor on the node
		for _, b := range dagBlocks {
			require.NoError(t, fx.localFileStorage.Delete(ctx, b.Cid()))
		}

		objectId := "fileObjectId1"
		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
		}))

		it := fx.waitItemState(t, objectId, FileStateMissingBlocks, 5*time.Second)
		assert.True(t, it.ScheduledAt.After(time.Now()), "missing-blocks item must be scheduled for a future retry")

		_, ok := statuses.lastStatus(objectId)
		assert.False(t, ok, "no premature status update for a file we can't act on")

		// The file appears on the node (e.g. the original uploader came online)
		require.NoError(t, fx.rpcStore.AddToFile(ctx, "spaceOld", fileId, dagBlocks))

		// Make the retry due now
		queueIt, err := fx.queue.GetById(objectId)
		require.NoError(t, err)
		queueIt.ScheduledAt = time.Now()
		require.NoError(t, fx.queue.ReleaseAndUpdate(objectId, queueIt))

		fx.waitItemState(t, objectId, FileStateDone, 5*time.Second)

		gotStatus, ok := statuses.lastStatus(objectId)
		require.True(t, ok)
		assert.Equal(t, filesyncstatus.Synced, gotStatus)
	})

	t.Run("parked from upload phase: stale plan is dropped, retry re-checks the node", func(t *testing.T) {
		// The item parks AFTER a successful availability check (all blocks
		// were locally enumerable, one leaf's data was missing everywhere),
		// so a to-upload plan was computed. The retry must re-check the node
		// instead of replaying the stale plan.
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		statuses := newStatusRecorder(fx)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 3*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)

		// Node has nothing; one leaf's data is missing locally too
		require.NoError(t, fx.localFileStorage.Delete(ctx, dagBlocks[len(dagBlocks)-1].Cid()))

		objectId := "fileObjectId1"
		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
		}))

		it := fx.waitItemState(t, objectId, FileStateMissingBlocks, 5*time.Second)
		assert.Empty(t, it.CidsToUpload, "parked item must not keep a stale plan")
		assert.Empty(t, it.CidsToBind, "parked item must not keep a stale plan")

		// The whole file appears on the node
		require.NoError(t, fx.rpcStore.AddToFile(ctx, "spaceOld", fileId, dagBlocks))

		queueIt, err := fx.queue.GetById(objectId)
		require.NoError(t, err)
		queueIt.ScheduledAt = time.Now()
		require.NoError(t, fx.queue.ReleaseAndUpdate(objectId, queueIt))

		fx.waitItemState(t, objectId, FileStateDone, 5*time.Second)

		gotStatus, ok := statuses.lastStatus(objectId)
		require.True(t, ok)
		assert.Equal(t, filesyncstatus.Synced, gotStatus)
	})

	t.Run("AddFile wakes a parked item when blocks become available", func(t *testing.T) {
		fx := newFixtureNotStarted(t, 1024*1024*1024)
		statuses := newStatusRecorder(fx)
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 3*1024*1024)
		dagBlocks := fx.collectDAGBlocks(t, fileNode)

		// Blocks exist neither locally nor on the node
		for _, b := range dagBlocks {
			require.NoError(t, fx.localFileStorage.Delete(ctx, b.Cid()))
		}

		objectId := "fileObjectId1"
		req := AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: "space1", FileId: fileId},
		}
		require.NoError(t, fx.AddFile(req))

		it := fx.waitItemState(t, objectId, FileStateMissingBlocks, 5*time.Second)
		assert.Positive(t, it.MissingBlocksRetries)

		// The file is re-added by the user: blocks are local again
		require.NoError(t, fx.localFileStorage.Add(ctx, dagBlocks))

		// The explicit AddFile must wake the parked item immediately
		require.NoError(t, fx.AddFile(req))

		fx.waitItemState(t, objectId, FileStateDone, 5*time.Second)

		gotStatus, ok := statuses.lastStatus(objectId)
		require.True(t, ok)
		assert.Equal(t, filesyncstatus.Synced, gotStatus)
	})
}

func blockCids(bs []blocks.Block) []cid.Cid {
	out := make([]cid.Cid, 0, len(bs))
	for _, b := range bs {
		out = append(out, b.Cid())
	}
	return out
}

func TestFileSync_RecoveredParkedItemGetsShortTransientRetry(t *testing.T) {
	// A parked item whose availability check finally succeeds must leave the
	// MissingBlocks state immediately: a transient failure in a later step
	// (here: SpaceInfo) must produce the normal ~1-minute retry, not the
	// multi-hour missing-blocks backoff earned by its parking history.
	transientErr := errors.New("transient space info failure")

	fx := newFixtureNotStarted(t, 1024*1024*1024)
	fx.rpcStore.SetSpaceInfoError(transientErr)
	require.NoError(t, fx.a.Start(ctx))
	defer fx.Finish(t)

	spaceId := "space1"
	fx.waitCondition(t, 2*time.Second, func() bool {
		_, err := fx.limitManager.getSpace(spaceId)
		return err == nil
	})

	// Blocks are fully available locally, so the availability check succeeds
	fileId, _ := fx.givenFileAddedToDAG(t, 1024)

	it := FileInfo{
		FileId:               fileId,
		SpaceId:              spaceId,
		ObjectId:             "fileObjectId1",
		State:                FileStateMissingBlocks,
		MissingBlocksRetries: 5, // long backoff earned before recovery
		ScheduledAt:          time.Now(),
		CidsToUpload:         map[cid.Cid]struct{}{},
		CidsToBind:           map[cid.Cid]struct{}{},
	}

	result, err := fx.processFilePendingUpload(ctx, it)

	require.Error(t, err)
	assert.ErrorIs(t, err, transientErr)
	assert.Equal(t, FileStatePendingUpload, result.State, "recovered item must leave MissingBlocks once the check succeeds")
	assert.Zero(t, result.MissingBlocksRetries)
	assert.WithinDuration(t, time.Now().Add(time.Minute), result.ScheduledAt, 15*time.Second,
		"transient failure after a successful check must get the short retry")
}

func TestFileSync_BindNotFoundDropsCachedPlan(t *testing.T) {
	// The node lost a block between BlocksCheck and BlocksBind (GC race): the
	// cached plan must be dropped so the next attempt re-enumerates instead of
	// replaying the stale plan forever
	fx := newFixture(t, 1024*1024*1024)
	defer fx.Finish(t)

	spaceId := "space1"
	fx.waitCondition(t, 2*time.Second, func() bool {
		_, err := fx.limitManager.getSpace(spaceId)
		return err == nil
	})

	fileId, _ := fx.givenFileAddedToDAG(t, 1024)
	goneCid := cid.MustParse("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")

	it := FileInfo{
		FileId:              fileId,
		SpaceId:             spaceId,
		ObjectId:            "fileObjectId1",
		State:               FileStatePendingUpload,
		ScheduledAt:         time.Now(),
		BytesToUploadOrBind: 1024,
		CidsToBind:          map[cid.Cid]struct{}{goneCid: {}},
		CidsToUpload:        map[cid.Cid]struct{}{},
	}

	result, err := fx.processFilePendingUpload(ctx, it)

	require.Error(t, err)
	assert.Empty(t, result.CidsToBind, "stale plan must be dropped after a bind not-found error")
	assert.Empty(t, result.CidsToUpload)
	assert.Zero(t, result.BytesToUploadOrBind)
	assert.Equal(t, FileStatePendingUpload, result.State)
}

func TestFileSync_DeleteResetsParkedSchedule(t *testing.T) {
	// Deleting a file parked with a long backoff must not wait the backoff out
	fx := newFixture(t, 1024*1024*1024)
	defer fx.Finish(t)

	fileId, _ := fx.givenFileAddedToDAG(t, 1024)
	objectId := "fileObjectId1"

	err := fx.queue.Upsert(objectId, func(exists bool, prev FileInfo) FileInfo {
		return FileInfo{
			FileId:      fileId,
			SpaceId:     "space1",
			ObjectId:    objectId,
			State:       FileStateMissingBlocks,
			ScheduledAt: time.Now().Add(20 * time.Hour),
		}
	})
	require.NoError(t, err)

	require.NoError(t, fx.DeleteFile(objectId, domain.FullFileId{SpaceId: "space1", FileId: fileId}))

	it, err := fx.queue.GetById(objectId)
	require.NoError(t, err)
	assert.Equal(t, FileStatePendingDeletion, it.State)
	assert.True(t, it.ScheduledAt.Before(time.Now().Add(5*time.Minute)),
		"deletion must be scheduled promptly, not after the parked backoff")
	require.NoError(t, fx.queue.ReleaseAndUpdate(objectId, it))
}

func TestMissingBlocksRetryDelay(t *testing.T) {
	want := []time.Duration{
		10 * time.Minute,
		40 * time.Minute,
		160 * time.Minute,
		640 * time.Minute,
		24 * time.Hour, // 2560min capped
		24 * time.Hour,
	}
	for retries, wantDelay := range want {
		assert.Equal(t, wantDelay, missingBlocksRetryDelay(retries), "retries=%d", retries)
	}
}

func TestRescheduleTransient(t *testing.T) {
	t.Run("parked item keeps its long backoff", func(t *testing.T) {
		it := FileInfo{State: FileStateMissingBlocks, MissingBlocksRetries: 2}
		got := rescheduleTransient(it, errors.New("any transient error"))
		assert.WithinDuration(t, time.Now().Add(160*time.Minute), got.ScheduledAt, 15*time.Second)
	})

	t.Run("connectivity error gets the longer delay", func(t *testing.T) {
		it := FileInfo{State: FileStatePendingUpload}
		got := rescheduleTransient(it, fmt.Errorf("get node: %w", rpcstore2.ErrNoConnectionToAnyFileClient))
		assert.WithinDuration(t, time.Now().Add(connectivityRetryDelay), got.ScheduledAt, 15*time.Second)
	})

	t.Run("other transient errors get the short retry", func(t *testing.T) {
		it := FileInfo{State: FileStatePendingUpload}
		got := rescheduleTransient(it, errors.New("some rpc error"))
		assert.WithinDuration(t, time.Now().Add(time.Minute), got.ScheduledAt, 15*time.Second)
	})
}
