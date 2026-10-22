package v2handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
)

func TestDownloadFile(t *testing.T) {
	fx := newV2HandlerFixture(t)
	const path = "/v2/spaces/space1/files/file1/content"
	fx.router.GET("/v2/spaces/:space_id/files/:file_id/content", DownloadFileHandler(fx.svc))
	fx.router.HEAD("/v2/spaces/:space_id/files/:file_id/content", HeadFileHandler(fx.svc))
	require.NoError(t, fx.store.BindSpaceId(context.Background(), "space1", "file1"))
	file := mock_files.NewMockFile(t)
	fx.fileMock.EXPECT().GetImageData(mock.Anything, "file1").Return(nil, errors.New("not an image"))
	fx.fileMock.EXPECT().GetFileData(mock.Anything, "file1").Return(file, nil)
	file.EXPECT().FileId().Return(domain.FileId("file-cid"))
	modified := time.Unix(1700000000, 0)
	file.EXPECT().Meta().Return(&files.FileMeta{Name: "hello.txt", Media: "text/plain", LastModifiedDate: modified.Unix()})
	file.EXPECT().Reader(mock.Anything).RunAndReturn(func(context.Context) (io.ReadSeeker, error) {
		return strings.NewReader("hello world"), nil
	})
	request := func(method, url string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, nil)
		req = req.WithContext(util.CtxWithApiGrant(req.Context(), &util.ApiGrant{Spaces: []string{"space1"}, Perms: util.GrantPermsRead}))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)
		return w
	}

	w := request(http.MethodGet, path, nil)
	require.Equal(t, 200, w.Code)
	assert.Equal(t, "hello world", w.Body.String())
	assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
	assert.Equal(t, "11", w.Header().Get("Content-Length"))
	assert.Equal(t, "private, no-cache", w.Header().Get("Cache-Control"))
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)

	for _, tc := range []struct {
		name, method, query string
		headers             map[string]string
		status              int
		body                string
	}{
		{"head", http.MethodHead, "", nil, 200, ""},
		{"range", http.MethodGet, "", map[string]string{"Range": "bytes=0-4"}, 206, "hello"},
		{"suffix range", http.MethodGet, "", map[string]string{"Range": "bytes=-5"}, 206, "world"},
		{"etag unchanged", http.MethodGet, "", map[string]string{"If-None-Match": etag}, 304, ""},
		{"date unchanged", http.MethodGet, "", map[string]string{"If-Modified-Since": modified.UTC().Format(http.TimeFormat)}, 304, ""},
		{"matching if-range", http.MethodGet, "", map[string]string{"Range": "bytes=0-4", "If-Range": etag}, 206, "hello"},
		{"stale if-range", http.MethodGet, "", map[string]string{"Range": "bytes=0-4", "If-Range": `"old"`}, 200, "hello world"},
		{"unsatisfiable range", http.MethodGet, "", map[string]string{"Range": "bytes=100-200"}, 416, ""},
		{"invalid range", http.MethodGet, "", map[string]string{"Range": "bytes=nope"}, 416, ""},
		{"failed precondition", http.MethodGet, "", map[string]string{"If-Match": `"old"`}, 412, ""},
		{"invalid width", http.MethodGet, "?width=nope", nil, 400, ""},
		{"negative width", http.MethodGet, "?width=-1", nil, 400, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(tc.method, path+tc.query, tc.headers)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status >= 400 {
				var apiErr v2model.Error
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &apiErr))
				assert.Equal(t, tc.status, apiErr.Status)
				assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
				assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
			} else {
				assert.Equal(t, tc.body, w.Body.String())
				assert.Equal(t, etag, w.Header().Get("ETag"))
			}
			if tc.name == "range" {
				assert.Equal(t, "bytes 0-4/11", w.Header().Get("Content-Range"))
			}
			if tc.name == "unsatisfiable range" {
				assert.Equal(t, "bytes */11", w.Header().Get("Content-Range"))
			}
		})
	}
}
