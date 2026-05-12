package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/pb"
)

const (
	testApiBaseUrl = "http://127.0.0.1:31009"
	testTechSpace  = "techspace"
)

func newDownloadSvc(t *testing.T) (*service.Service, *mock_apicore.MockFileObjectService) {
	mwMock := mock_apicore.NewMockClientCommands(t)
	crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	fileObjectMock := mock_apicore.NewMockFileObjectService(t)
	svc := service.NewService(mwMock, fileObjectMock, testApiBaseUrl, testTechSpace, crossSpaceSubService)
	return svc, fileObjectMock
}

func TestDownloadFileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("streams non-image file bytes", func(t *testing.T) {
		svc, fileObjectMock := newDownloadSvc(t)

		// Service tries image pipeline first; non-image returns error and we
		// fall through to GetFileData.
		fileObjectMock.EXPECT().GetImageData(mock.Anything, "obj-1").Return(nil, errors.New("not an image")).Once()

		fileMock := mock_files.NewMockFile(t)
		fileMock.EXPECT().Meta().Return(&files.FileMeta{
			Media:            "text/plain",
			Name:             "hello.txt",
			LastModifiedDate: time.Now().Unix(),
		})
		fileMock.EXPECT().Reader(mock.Anything).Return(readSeekerOf("hello world"), nil)
		fileObjectMock.EXPECT().GetFileData(mock.Anything, "obj-1").Return(fileMock, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/v1/spaces/space-1/files/obj-1", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.GET("/v1/spaces/:space_id/files/:file_id", DownloadFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
		assert.Equal(t, "hello world", w.Body.String())
	})

	t.Run("HEAD returns headers without body", func(t *testing.T) {
		svc, fileObjectMock := newDownloadSvc(t)

		fileObjectMock.EXPECT().GetImageData(mock.Anything, "obj-1").Return(nil, errors.New("not an image")).Once()

		fileMock := mock_files.NewMockFile(t)
		fileMock.EXPECT().Meta().Return(&files.FileMeta{
			Media:            "text/plain",
			Name:             "hello.txt",
			LastModifiedDate: time.Now().Unix(),
		})
		fileMock.EXPECT().Reader(mock.Anything).Return(readSeekerOf("hello world"), nil)
		fileObjectMock.EXPECT().GetFileData(mock.Anything, "obj-1").Return(fileMock, nil).Once()

		req := httptest.NewRequest(http.MethodHead, "/v1/spaces/space-1/files/obj-1", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.HEAD("/v1/spaces/:space_id/files/:file_id", DownloadFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
		assert.Empty(t, w.Body.String(), "HEAD response must have empty body")
	})

	t.Run("rejects non-numeric width", func(t *testing.T) {
		svc, _ := newDownloadSvc(t)

		req := httptest.NewRequest(http.MethodGet, "/v1/spaces/space-1/files/obj-1?width=oops", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.GET("/v1/spaces/:space_id/files/:file_id", DownloadFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid width")
	})

	t.Run("rejects negative width", func(t *testing.T) {
		svc, _ := newDownloadSvc(t)

		req := httptest.NewRequest(http.MethodGet, "/v1/spaces/space-1/files/obj-1?width=-5", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.GET("/v1/spaces/:space_id/files/:file_id", DownloadFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 when file is not found", func(t *testing.T) {
		svc, fileObjectMock := newDownloadSvc(t)
		fileObjectMock.EXPECT().GetImageData(mock.Anything, "missing").Return(nil, errors.New("not an image")).Once()
		fileObjectMock.EXPECT().GetFileData(mock.Anything, "missing").Return(nil, errors.New("not found")).Once()

		req := httptest.NewRequest(http.MethodGet, "/v1/spaces/space-1/files/missing", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.GET("/v1/spaces/:space_id/files/:file_id", DownloadFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestDeleteFileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("default soft delete archives the object", func(t *testing.T) {
		mwMock := mock_apicore.NewMockClientCommands(t)
		crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
		fileObjectMock := mock_apicore.NewMockFileObjectService(t)
		svc := service.NewService(mwMock, fileObjectMock, testApiBaseUrl, testTechSpace, crossSpaceSubService)

		mwMock.EXPECT().ObjectSetIsArchived(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectSetIsArchivedRequest) bool {
			return req.ContextId == "obj-1" && req.IsArchived
		})).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL},
		}).Once()

		req := httptest.NewRequest(http.MethodDelete, "/v1/spaces/space-1/files/obj-1", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.DELETE("/v1/spaces/:space_id/files/:file_id", DeleteFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("skip_bin=true skips archive and permanently deletes", func(t *testing.T) {
		mwMock := mock_apicore.NewMockClientCommands(t)
		crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
		fileObjectMock := mock_apicore.NewMockFileObjectService(t)
		svc := service.NewService(mwMock, fileObjectMock, testApiBaseUrl, testTechSpace, crossSpaceSubService)

		mwMock.EXPECT().ObjectListDelete(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectListDeleteRequest) bool {
			return len(req.ObjectIds) == 1 && req.ObjectIds[0] == "obj-1"
		})).Return(&pb.RpcObjectListDeleteResponse{
			Error: &pb.RpcObjectListDeleteResponseError{Code: pb.RpcObjectListDeleteResponseError_NULL},
		}).Once()

		req := httptest.NewRequest(http.MethodDelete, "/v1/spaces/space-1/files/obj-1?skip_bin=true", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.DELETE("/v1/spaces/:space_id/files/:file_id", DeleteFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("rejects non-bool skip_bin", func(t *testing.T) {
		mwMock := mock_apicore.NewMockClientCommands(t)
		crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
		fileObjectMock := mock_apicore.NewMockFileObjectService(t)
		svc := service.NewService(mwMock, fileObjectMock, testApiBaseUrl, testTechSpace, crossSpaceSubService)

		req := httptest.NewRequest(http.MethodDelete, "/v1/spaces/space-1/files/obj-1?skip_bin=oops", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.DELETE("/v1/spaces/:space_id/files/:file_id", DeleteFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid skip_bin")
	})

	t.Run("middleware error surfaces as 500", func(t *testing.T) {
		mwMock := mock_apicore.NewMockClientCommands(t)
		crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
		fileObjectMock := mock_apicore.NewMockFileObjectService(t)
		svc := service.NewService(mwMock, fileObjectMock, testApiBaseUrl, testTechSpace, crossSpaceSubService)

		mwMock.EXPECT().ObjectSetIsArchived(mock.Anything, mock.Anything).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{
				Code:        pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR,
				Description: "boom",
			},
		}).Once()

		req := httptest.NewRequest(http.MethodDelete, "/v1/spaces/space-1/files/obj-1", nil)
		w := httptest.NewRecorder()

		router := gin.New()
		router.DELETE("/v1/spaces/:space_id/files/:file_id", DeleteFileHandler(svc))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// readSeekerOf adapts a string to an io.ReadSeeker for stubbed file readers.
func readSeekerOf(s string) io.ReadSeeker {
	return bytes.NewReader([]byte(s))
}
