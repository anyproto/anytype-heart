package v2service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestV2UploadFile(t *testing.T) {
	t.Run("url upload returns the file object id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.MatchedBy(func(req *pb.RpcFileUploadRequest) bool {
			return req.SpaceId == testSpaceId && req.Url == "https://example.org/a.pdf" && req.LocalPath == ""
		})).Return(&pb.RpcFileUploadResponse{
			ObjectId: "file1",
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():         pbtypes.String("a.pdf"),
				bundle.RelationKeyFileMimeType.String(): pbtypes.String("application/pdf"),
				bundle.RelationKeySizeInBytes.String():  pbtypes.Int64(123),
			}},
			Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL},
		})
		want := &v2model.V2FileUploadResult{Id: "file1", Name: "a.pdf", MimeType: "application/pdf", Size: 123}

		// when
		got, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/a.pdf", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("neither path nor url is a 400", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.UploadFile(context.Background(), testSpaceId, "", "", false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.V2CodeValidationFailed, apiErr.Code)
	})

	t.Run("dry run uploads nothing", func(t *testing.T) {
		// given: no FileUpload expectation
		fx := newV2Fixture(t)

		// when
		got, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/a.pdf", true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
		assert.Empty(t, got.Id)
	})

	t.Run("upload failure is wrapped with the operation", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.Anything).Return(&pb.RpcFileUploadResponse{
			Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_UNKNOWN_ERROR, Description: "boom"},
		})

		// when
		_, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/a.pdf", false)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upload file")
		assert.Contains(t, err.Error(), "boom")
	})
}
