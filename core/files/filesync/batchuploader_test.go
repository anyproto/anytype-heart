package filesync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

func TestAddToLimitedQueue(t *testing.T) {
	t.Run("object deleted error sets pending deletion", func(t *testing.T) {
		fx := newFixtureNotStarted(t, 1024*1024*1024)

		fx.OnStatusUpdated(func(string, domain.FullFileId, filesyncstatus.Status) error {
			return domain.ErrObjectIsDeleted
		})

		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		objectId := "obj1"
		fileId := domain.FileId("bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi")

		err := fx.queue.Upsert(objectId, func(exists bool, prev FileInfo) FileInfo {
			return FileInfo{
				ObjectId:    objectId,
				FileId:      fileId,
				SpaceId:     "space1",
				State:       FileStateUploading,
				ScheduledAt: time.Now(),
			}
		})
		require.NoError(t, err)

		err = fx.addToLimitedQueue(objectId)
		require.NoError(t, err)

		getCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		it, err := fx.queue.GetNext(getCtx, filequeue.GetNextRequest[FileInfo]{
			Subscribe:   true,
			StoreFilter: query.Or{filterByState(FileStatePendingDeletion), filterByState(FileStateDeleted)},
			Filter: func(info FileInfo) bool {
				return info.ObjectId == objectId && (info.State == FileStatePendingDeletion || info.State == FileStateDeleted)
			},
		})
		require.NoError(t, err)
		assert.True(t, it.State == FileStatePendingDeletion || it.State == FileStateDeleted)
		require.NoError(t, fx.queue.ReleaseAndUpdate(it.ObjectId, it))
	})

	t.Run("success sets limited state", func(t *testing.T) {
		fx := newFixtureNotStarted(t, 1024*1024*1024)

		fx.OnStatusUpdated(func(string, domain.FullFileId, filesyncstatus.Status) error {
			return nil
		})

		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		objectId := "obj1"
		fileId := domain.FileId("bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi")

		err := fx.queue.Upsert(objectId, func(exists bool, prev FileInfo) FileInfo {
			return FileInfo{
				ObjectId:    objectId,
				FileId:      fileId,
				SpaceId:     "space1",
				State:       FileStateUploading,
				ScheduledAt: time.Now(),
			}
		})
		require.NoError(t, err)

		err = fx.addToLimitedQueue(objectId)
		require.NoError(t, err)

		getCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		it, err := fx.queue.GetNext(getCtx, filequeue.GetNextRequest[FileInfo]{
			Subscribe:   true,
			StoreFilter: query.Or{filterByState(FileStateLimited), filterByState(FileStatePendingUpload)},
			Filter: func(info FileInfo) bool {
				return info.ObjectId == objectId && (info.State == FileStateLimited || info.State == FileStatePendingUpload)
			},
		})
		require.NoError(t, err)
		assert.True(t, it.State == FileStateLimited || it.State == FileStatePendingUpload)
		require.NoError(t, fx.queue.ReleaseAndUpdate(it.ObjectId, it))
	})

	t.Run("non-deleted error reschedules as pending upload", func(t *testing.T) {
		fx := newFixtureNotStarted(t, 1024*1024*1024)

		fx.OnStatusUpdated(func(string, domain.FullFileId, filesyncstatus.Status) error {
			return errors.New("some transient error")
		})

		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		objectId := "obj1"
		fileId := domain.FileId("bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi")

		err := fx.queue.Upsert(objectId, func(exists bool, prev FileInfo) FileInfo {
			return FileInfo{
				ObjectId:    objectId,
				FileId:      fileId,
				SpaceId:     "space1",
				State:       FileStateUploading,
				ScheduledAt: time.Now(),
			}
		})
		require.NoError(t, err)

		err = fx.addToLimitedQueue(objectId)
		require.Error(t, err)

		getCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		it, err := fx.queue.GetNext(getCtx, filequeue.GetNextRequest[FileInfo]{
			Subscribe:   true,
			StoreFilter: filterByState(FileStatePendingUpload),
			Filter: func(info FileInfo) bool {
				return info.ObjectId == objectId && info.State == FileStatePendingUpload
			},
		})
		require.NoError(t, err)
		assert.Equal(t, FileStatePendingUpload, it.State)
		require.NoError(t, fx.queue.ReleaseAndUpdate(it.ObjectId, it))
	})
}
