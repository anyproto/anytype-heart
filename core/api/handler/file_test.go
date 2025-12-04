package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestUploadFileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("successful upload", func(t *testing.T) {
		// given
		mwMock := mock_apicore.NewMockClientCommands(t)
		svc := service.NewService(mwMock, "http://localhost:31006", "techspace")

		mwMock.On("FileUpload", mock.Anything, mock.MatchedBy(func(req *pb.RpcFileUploadRequest) bool {
			return req.SpaceId == "space1" && req.Type == model.BlockContentFile_File
		})).Return(&pb.RpcFileUploadResponse{
			ObjectId:      "obj123",
			PreloadFileId: "bafyreiabc123",
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					"name":        pbtypes.String("test.txt"),
					"sizeInBytes": pbtypes.Int64(12),
				},
			},
			Error: &pb.RpcFileUploadResponseError{Code: pb.RpcFileUploadResponseError_NULL},
		}).Once()

		// Create multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		_, err = part.Write([]byte("test content"))
		require.NoError(t, err)
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/spaces/space1/files", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/v1/spaces/:space_id/files", UploadFileHandler(svc))

		// when
		router.ServeHTTP(w, req)

		// then
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "obj123")
		assert.Contains(t, w.Body.String(), "bafyreiabc123")
	})

	t.Run("missing file", func(t *testing.T) {
		// given
		mwMock := mock_apicore.NewMockClientCommands(t)
		svc := service.NewService(mwMock, "http://localhost:31006", "techspace")

		req := httptest.NewRequest(http.MethodPost, "/v1/spaces/space1/files", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/v1/spaces/:space_id/files", UploadFileHandler(svc))

		// when
		router.ServeHTTP(w, req)

		// then
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "missing file in request")
	})

	t.Run("service upload error", func(t *testing.T) {
		// given
		mwMock := mock_apicore.NewMockClientCommands(t)
		svc := service.NewService(mwMock, "http://localhost:31006", "techspace")

		mwMock.On("FileUpload", mock.Anything, mock.Anything).
			Return(&pb.RpcFileUploadResponse{
				Error: &pb.RpcFileUploadResponseError{
					Code:        pb.RpcFileUploadResponseError_UNKNOWN_ERROR,
					Description: "upload failed",
				},
			}).Once()

		// Create multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		_, err = part.Write([]byte("test content"))
		require.NoError(t, err)
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/spaces/space1/files", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router := gin.New()
		router.POST("/v1/spaces/:space_id/files", UploadFileHandler(svc))

		// when
		router.ServeHTTP(w, req)

		// then
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
