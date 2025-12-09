package service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestFileService_UploadFile(t *testing.T) {
	t.Run("successful upload", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("FileUpload", mock.Anything, &pb.RpcFileUploadRequest{
			SpaceId:   mockedSpaceId,
			LocalPath: "/tmp/test.txt",
			Type:      model.BlockContentFile_File,
		}).Return(&pb.RpcFileUploadResponse{
			ObjectId:      "obj123",
			PreloadFileId: "bafyreiabc123",
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					"name":     pbtypes.String("test.txt"),
					"sizeInBytes": pbtypes.Int64(1024),
				},
			},
			Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL},
		}).Once()

		// when
		result, err := fx.service.UploadFile(ctx, mockedSpaceId, "/tmp/test.txt")

		// then
		require.NoError(t, err)
		assert.Equal(t, "obj123", result.ObjectId)
		assert.Equal(t, "bafyreiabc123", result.FileId)
		assert.Equal(t, "test.txt", result.Details["name"])
		assert.Equal(t, float64(1024), result.Details["sizeInBytes"])
	})

	t.Run("upload error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("FileUpload", mock.Anything, mock.Anything).
			Return(&pb.RpcFileUploadResponse{
				Error: &pb.RpcFileUploadResponseError{
					Code:        pb.RpcFileUploadResponseError_UNKNOWN_ERROR,
					Description: "upload failed",
				},
			}).Once()

		// when
		result, err := fx.service.UploadFile(ctx, mockedSpaceId, "/tmp/test.txt")

		// then
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrFailedUploadFile)
		assert.Contains(t, err.Error(), "upload failed")
	})

	t.Run("upload with nil details", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("FileUpload", mock.Anything, mock.Anything).
			Return(&pb.RpcFileUploadResponse{
				ObjectId:      "obj456",
				PreloadFileId: "bafyreiabc456",
				Details:       nil,
				Error:         &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL},
			}).Once()

		// when
		result, err := fx.service.UploadFile(ctx, mockedSpaceId, "/tmp/test.txt")

		// then
		require.NoError(t, err)
		assert.Equal(t, "obj456", result.ObjectId)
		assert.Equal(t, "bafyreiabc456", result.FileId)
		assert.Nil(t, result.Details)
	})
}
