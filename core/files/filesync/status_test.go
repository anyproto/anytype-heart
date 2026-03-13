package filesync

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore/mock_rpcstore"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

func TestUpdateStatus(t *testing.T) {
	t.Run("success when no callbacks", func(t *testing.T) {
		s := &fileSync{}
		it := FileInfo{ObjectId: "obj1", SpaceId: "space1", FileId: "file1"}

		err := s.updateStatus(it, filesyncstatus.Synced)
		require.NoError(t, err)
	})

	t.Run("success when callback succeeds", func(t *testing.T) {
		var called bool
		s := &fileSync{
			onStatusUpdated: []StatusCallback{
				func(objectId string, fileId domain.FullFileId, status filesyncstatus.Status) error {
					called = true
					assert.Equal(t, "obj1", objectId)
					assert.Equal(t, filesyncstatus.Synced, status)
					return nil
				},
			},
		}
		it := FileInfo{ObjectId: "obj1", SpaceId: "space1", FileId: "file1"}

		err := s.updateStatus(it, filesyncstatus.Synced)
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("callback error is returned", func(t *testing.T) {
		wantErr := errors.New("callback failed")
		s := &fileSync{
			onStatusUpdated: []StatusCallback{
				func(string, domain.FullFileId, filesyncstatus.Status) error {
					return wantErr
				},
			},
		}
		it := FileInfo{ObjectId: "obj1", SpaceId: "space1", FileId: "file1"}

		err := s.updateStatus(it, filesyncstatus.Synced)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("object deleted error is returned without special handling", func(t *testing.T) {
		s := &fileSync{
			onStatusUpdated: []StatusCallback{
				func(string, domain.FullFileId, filesyncstatus.Status) error {
					return domain.ErrObjectIsDeleted
				},
			},
		}
		it := FileInfo{ObjectId: "obj1", SpaceId: "space1", FileId: "file1"}

		err := s.updateStatus(it, filesyncstatus.Synced)
		require.ErrorIs(t, err, domain.ErrObjectIsDeleted)
		assert.True(t, isObjectDeletedError(err))
	})

	t.Run("stops at first failing callback", func(t *testing.T) {
		var secondCalled bool
		wantErr := errors.New("first fails")
		s := &fileSync{
			onStatusUpdated: []StatusCallback{
				func(string, domain.FullFileId, filesyncstatus.Status) error {
					return wantErr
				},
				func(string, domain.FullFileId, filesyncstatus.Status) error {
					secondCalled = true
					return nil
				},
			},
		}
		it := FileInfo{ObjectId: "obj1", SpaceId: "space1", FileId: "file1"}

		err := s.updateStatus(it, filesyncstatus.Synced)
		require.ErrorIs(t, err, wantErr)
		assert.False(t, secondCalled)
	})
}

func TestHandleLimitReached(t *testing.T) {
	t.Run("object deleted error is propagated", func(t *testing.T) {
		synctest.Run(func() {
			rpcStore := mock_rpcstore.NewMockRpcStore(t)
			rpcStore.EXPECT().DeleteFiles(mock.Anything, "space1", domain.FileId("file1")).Return(nil)

			s := &fileSync{
				rpcStore: rpcStore,
				onStatusUpdated: []StatusCallback{
					func(string, domain.FullFileId, filesyncstatus.Status) error {
						return domain.ErrObjectIsDeleted
					},
				},
			}
			it := FileInfo{
				ObjectId: "obj1",
				SpaceId:  "space1",
				FileId:   "file1",
				State:    FileStateLimited,
			}

			got, err := s.handleLimitReached(context.Background(), it)
			require.Error(t, err)
			assert.True(t, isObjectDeletedError(err))
			assert.Equal(t, FileStateLimited, got.State, "state unchanged, caller handles it")
		})
	})

	t.Run("success sends limit event for user-added file", func(t *testing.T) {
		synctest.Run(func() {
			rpcStore := mock_rpcstore.NewMockRpcStore(t)
			rpcStore.EXPECT().DeleteFiles(mock.Anything, "space1", domain.FileId("file1")).Return(nil)

			sender := mock_event.NewMockSender(t)
			sender.EXPECT().Broadcast(mock.Anything).Once()

			s := &fileSync{
				rpcStore:    rpcStore,
				eventSender: sender,
				onStatusUpdated: []StatusCallback{
					func(string, domain.FullFileId, filesyncstatus.Status) error {
						return nil
					},
				},
			}
			it := FileInfo{
				ObjectId:    "obj1",
				SpaceId:     "space1",
				FileId:      "file1",
				State:       FileStateLimited,
				AddedByUser: true,
			}

			got, err := s.handleLimitReached(context.Background(), it)
			require.NoError(t, err)
			assert.Equal(t, FileStateLimited, got.State)
		})
	})

	t.Run("success stores import event for imported file", func(t *testing.T) {
		synctest.Run(func() {
			rpcStore := mock_rpcstore.NewMockRpcStore(t)
			rpcStore.EXPECT().DeleteFiles(mock.Anything, "space1", domain.FileId("file1")).Return(nil)

			s := &fileSync{
				rpcStore: rpcStore,
				onStatusUpdated: []StatusCallback{
					func(string, domain.FullFileId, filesyncstatus.Status) error {
						return nil
					},
				},
			}
			it := FileInfo{
				ObjectId: "obj1",
				SpaceId:  "space1",
				FileId:   "file1",
				State:    FileStateLimited,
				Imported: true,
			}

			got, err := s.handleLimitReached(context.Background(), it)
			require.NoError(t, err)
			assert.Equal(t, FileStateLimited, got.State)
			require.Len(t, s.importEvents, 1)
		})
	})

	t.Run("updateStatus error is propagated", func(t *testing.T) {
		synctest.Run(func() {
			rpcStore := mock_rpcstore.NewMockRpcStore(t)
			rpcStore.EXPECT().DeleteFiles(mock.Anything, "space1", domain.FileId("file1")).Return(nil)

			wantErr := errors.New("status update failed")
			s := &fileSync{
				rpcStore: rpcStore,
				onStatusUpdated: []StatusCallback{
					func(string, domain.FullFileId, filesyncstatus.Status) error {
						return wantErr
					},
				},
			}
			it := FileInfo{
				ObjectId: "obj1",
				SpaceId:  "space1",
				FileId:   "file1",
				State:    FileStateLimited,
			}

			_, err := s.handleLimitReached(context.Background(), it)
			require.Error(t, err)
			assert.ErrorIs(t, err, wantErr)
		})
	})
}

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
			Subscribe:   false,
			StoreFilter: filterByState(FileStatePendingDeletion),
			Filter: func(info FileInfo) bool {
				return info.ObjectId == objectId && info.State == FileStatePendingDeletion
			},
		})
		require.NoError(t, err)
		assert.Equal(t, FileStatePendingDeletion, it.State)
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
			Subscribe:   false,
			StoreFilter: filterByState(FileStateLimited),
			Filter: func(info FileInfo) bool {
				return info.ObjectId == objectId && info.State == FileStateLimited
			},
		})
		require.NoError(t, err)
		assert.Equal(t, FileStateLimited, it.State)
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
			Subscribe:   false,
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
