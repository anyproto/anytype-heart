package filesync

import (
	"testing"
	"time"

	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
)

func TestDeleteFile(t *testing.T) {
	t.Run("if all is ok, delete file", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)
		spaceId := "space1"

		fileId, _ := fx.givenFileAddedToDAG(t, 1024)
		fx.givenFileUploaded(t, spaceId, fileId)

		err := fx.DeleteFile("objectId", domain.FullFileId{SpaceId: spaceId, FileId: fileId})
		require.NoError(t, err)

		it, err := fx.queue.GetNext(ctx, filequeue.GetNextRequest[FileInfo]{
			Subscribe:   true,
			StoreFilter: filterByState(FileStateDeleted),
			Filter:      func(info FileInfo) bool { return info.State == FileStateDeleted },
		})

		require.NoError(t, err)
		assert.Equal(t, fileId, it.FileId)

		err = fx.queue.ReleaseAndUpdate(it.ObjectId, it)

		resp, err := fx.rpcStore.FilesInfo(ctx, spaceId, fileId)
		require.NoError(t, err)
		require.Empty(t, resp)
	})

	t.Run("with error while deleting, add to retry queue", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)
		spaceId := "space1"

		testFileId := domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")

		// Just try to delete missing file, in-memory RPC store will return error.
		// In case of a missing file the real file node returns OK
		err := fx.DeleteFile("objectId", domain.FullFileId{SpaceId: spaceId, FileId: testFileId})
		require.NoError(t, err)

		inFuture := time.Now().Add(30 * time.Second)
		it, err := fx.queue.GetNext(ctx, filequeue.GetNextRequest[FileInfo]{
			Subscribe: true,
			StoreFilter: query.And{
				filterByState(FileStatePendingDeletion),
				// Means it's scheduled for retry
				query.Key{
					Path:   []string{"scheduledAt"},
					Filter: query.NewComp(query.CompOpGte, inFuture.Unix()),
				},
			},
			Filter: func(info FileInfo) bool {
				return info.State == FileStatePendingDeletion && info.ScheduledAt.After(inFuture)
			},
		})

		require.NoError(t, err)
		assert.Equal(t, testFileId, it.FileId)

		err = fx.queue.ReleaseAndUpdate(it.ObjectId, it)
		require.NoError(t, err)
	})
}
