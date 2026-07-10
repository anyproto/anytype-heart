package filesync

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
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
