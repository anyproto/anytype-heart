package v2service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
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
		want := &v2model.FileUploadResult{Id: "file1", Name: "a.pdf", MimeType: "application/pdf", Size: 123}

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
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
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

	t.Run("an unclassified upload failure is a 500 carrying op and description", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.Anything).Return(&pb.RpcFileUploadResponse{
			Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_UNKNOWN_ERROR, Description: "boom"},
		})

		// when
		_, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/a.pdf", false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusInternalServerError, apiErr.Status)
		assert.Equal(t, v2model.CodeInternalError, apiErr.Code)
		assert.Contains(t, apiErr.Message, "upload file")
		assert.Contains(t, apiErr.Message, "boom")
	})

	t.Run("a non-2xx source URL is a 400 on /url, not a retry-looping 500", func(t *testing.T) {
		// given: the description the uploader really produces — built from
		// the fileuploader sentinel so a producer rewording cannot leave
		// this test green against a dead string (surface review M2c)
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.Anything).Return(&pb.RpcFileUploadResponse{
			Error: &pb.RpcFileUploadResponseError{
				Code:        pb.RpcFileUploadResponseError_UNKNOWN_ERROR,
				Description: fileuploader.ErrFailedToDownload.Error() + ", status: 404",
			},
		})

		// when
		_, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.org/gone.pdf", false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/url", apiErr.Issues[0].Path)
	})

	t.Run("an unreachable host is a 400 on /url", func(t *testing.T) {
		// given: the description shape net/http produces (url.Error's
		// rendering, with the URL masked the way anyerror.CleanupError does
		// at the RPC boundary) — built from the stdlib type so a format
		// change in Go would surface here, not in production
		desc := (&url.Error{Op: "Get", URL: "<masked url>", Err: errors.New("dial tcp: connection refused")}).Error()
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.Anything).Return(&pb.RpcFileUploadResponse{
			Error: &pb.RpcFileUploadResponseError{
				Code:        pb.RpcFileUploadResponseError_UNKNOWN_ERROR,
				Description: desc,
			},
		})

		// when
		_, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://nope.invalid/a.pdf", false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/url", apiErr.Issues[0].Path)
	})

	t.Run("a local-path failure stays a 500 — the url arms are url-mode only", func(t *testing.T) {
		// given: a multipart upload (localPath mode) whose description
		// happens to contain a Get-shaped fragment must not be blamed on a
		// /url the caller never sent
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().FileUpload(mock.Anything, mock.Anything).Return(&pb.RpcFileUploadResponse{
			Error: &pb.RpcFileUploadResponseError{
				Code:        pb.RpcFileUploadResponseError_UNKNOWN_ERROR,
				Description: `cannot read file: Get "x"`,
			},
		})

		// when
		_, err := fx.UploadFile(context.Background(), testSpaceId, "/tmp/staged/upload.bin", "", false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusInternalServerError, apiErr.Status)
	})
}
