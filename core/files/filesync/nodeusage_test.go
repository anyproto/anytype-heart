package filesync

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore/mock_rpcstore"
)

func TestSpaceLimit(t *testing.T) {
	t.Run("concurrent file upload", func(t *testing.T) {
		for _, tc := range []struct {
			name          string
			limit         int
			usage         int
			filesToUpload int
			fileSize      int

			errorsCount int
		}{
			{
				name:          "3 concurrent files, 2 ok, 1 out of limits",
				limit:         100_000_000,
				usage:         0,
				filesToUpload: 3,
				fileSize:      40_000_000,
				errorsCount:   1,
			},
			{
				name:          "3 concurrent files, some used space, 1 ok, 2 out of limits",
				limit:         100_000_000,
				usage:         40_000_000,
				filesToUpload: 3,
				fileSize:      40_000_000,
				errorsCount:   2,
			},
			{
				name:          "10 concurrent files, 1 ok, 9 out of limits",
				limit:         100_000_000,
				usage:         0,
				filesToUpload: 10,
				fileSize:      90_000_000,
				errorsCount:   9,
			},
			{
				name:          "10 concurrent files, 10 ok",
				limit:         100_000_000,
				usage:         0,
				filesToUpload: 10,
				fileSize:      1_000,
				errorsCount:   0,
			},
			{
				name:          "10 concurrent files, no free space, 10 out of limits",
				limit:         100_000_000,
				usage:         99_999_999,
				filesToUpload: 10,
				fileSize:      1_000,
				errorsCount:   10,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				rpcStore := mock_rpcstore.NewMockRpcStore(t)
				rpcStore.EXPECT().SpaceInfo(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
					return &fileproto.SpaceInfoResponse{
						SpaceId:         spaceId,
						LimitBytes:      uint64(tc.limit),
						SpaceUsageBytes: uint64(tc.usage),
					}, nil
				})

				ctx := context.Background()
				spaceId := "space1"
				updateCh := make(chan updateMessage, 1)

				usage := newSpaceUsage(ctx, spaceId, rpcStore, updateCh)

				var wg sync.WaitGroup
				errorsCh := make(chan error, tc.filesToUpload)
				for i := 0; i < tc.filesToUpload; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						fileKey := fmt.Sprintf("file%d", i)
						errorsCh <- usage.allocateFile(ctx, fileKey, tc.fileSize)
					}()
				}

				wg.Wait()
				close(errorsCh)

				var errorsCount int
				for err := range errorsCh {
					if err != nil {
						errorsCount++
					}
				}

				assert.Equal(t, tc.errorsCount, errorsCount)
			})
		}
	})

	t.Run("test allocate then remove", func(t *testing.T) {
		rpcStore := mock_rpcstore.NewMockRpcStore(t)
		rpcStore.EXPECT().SpaceInfo(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
			return &fileproto.SpaceInfoResponse{
				SpaceId:         spaceId,
				LimitBytes:      uint64(100_000_000),
				SpaceUsageBytes: uint64(0),
			}, nil
		})

		ctx := context.Background()
		spaceId := "space1"
		updateCh := make(chan updateMessage, 1)

		usage := newSpaceUsage(ctx, spaceId, rpcStore, updateCh)

		err := usage.allocateFile(ctx, "file1", 90_000_000)
		require.NoError(t, err)

		err = usage.allocateFile(ctx, "file2", 80_000_000)
		require.Error(t, err)

		usage.deallocateFile("file1")

		err = usage.allocateFile(ctx, "file2", 80_000_000)
		require.NoError(t, err)
	})

	t.Run("test allocate then mark as uploaded", func(t *testing.T) {
		rpcStore := mock_rpcstore.NewMockRpcStore(t)
		rpcStore.EXPECT().SpaceInfo(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
			return &fileproto.SpaceInfoResponse{
				SpaceId:         spaceId,
				LimitBytes:      uint64(100_000_000),
				SpaceUsageBytes: uint64(0),
			}, nil
		})

		ctx := context.Background()
		spaceId := "space1"

		updateCh := make(chan updateMessage, 1)

		usage := newSpaceUsage(ctx, spaceId, rpcStore, updateCh)

		err := usage.allocateFile(ctx, "file1", 90_000_000)
		require.NoError(t, err)

		err = usage.allocateFile(ctx, "file2", 80_000_000)
		require.Error(t, err)

		usage.markFileUploaded("file1")

		err = usage.allocateFile(ctx, "file2", 80_000_000)
		require.Error(t, err)
	})
}
