package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestV2FileDownloadRoutes(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, tc := range []struct {
			name, grantedSpace, fileSpace string
			status                        int
		}{
			{"read-only key", "space1", "space1", 200},
			{"ungranted space", "space2", "space1", 403},
			{"wrong file space", "space1", "space2", 404},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				fx := newV2ServerFixture(t)
				fx.KeyToToken = map[string]ApiSessionEntry{
					"reader": {Token: "token", Scope: model.AccountAuth_JsonAPI,
						Grant: &util.ApiGrant{Spaces: []string{tc.grantedSpace}, Perms: util.GrantPermsRead}},
				}
				fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
					bundle.RelationKeyId:             domain.String("spaceView_space1"),
					bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
					bundle.RelationKeyTargetSpaceId:  domain.String("space1"),
				}})
				require.NoError(t, fx.objectStore.BindSpaceId(context.Background(), tc.fileSpace, "file1"))
				fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
				if tc.status == 200 {
					file := mock_files.NewMockFile(t)
					fx.fileObjectMock.EXPECT().GetImageData(mock.Anything, "file1").Return(nil, errors.New("not an image")).Once()
					fx.fileObjectMock.EXPECT().GetFileData(mock.Anything, "file1").Return(file, nil).Once()
					file.EXPECT().FileId().Return(domain.FileId("file-cid"))
					file.EXPECT().Reader(mock.Anything).Return(strings.NewReader("content"), nil)
					file.EXPECT().Meta().Return(&files.FileMeta{Name: "file.txt", Media: "text/plain"})
				}
				req := httptest.NewRequest(method, "/v2/spaces/space1/files/file1/content", nil)
				req.Host = localApiHost
				req.Header.Set("Authorization", "Bearer reader")
				w := httptest.NewRecorder()
				fx.Engine().ServeHTTP(w, req)
				require.Equal(t, tc.status, w.Code, w.Body.String())
				if tc.status == 200 {
					assert.Equal(t, "7", w.Header().Get("Content-Length"))
					if method == http.MethodGet {
						assert.Equal(t, "content", w.Body.String())
					} else {
						assert.Empty(t, w.Body.String())
					}
				}
			})
		}
	}
}

func TestV2IconDownloadRechecksReference(t *testing.T) {
	const iconId = "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"
	fx := newV2ServerFixture(t)
	grantedSession(fx, "reader", &util.ApiGrant{Spaces: []string{"space1"}, Perms: util.GrantPermsRead})
	view := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("spaceView_space1"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeyTargetSpaceId:  domain.String("space1"),
		bundle.RelationKeyIconImage:      domain.String(iconId),
	}
	fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{view})
	fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
	img := mock_files.NewMockImage(t)
	file := mock_files.NewMockFile(t)
	fx.fileObjectMock.EXPECT().GetImageDataFromRawId(mock.Anything, domain.FileId(iconId)).Return(img, nil).Once()
	img.EXPECT().GetOriginalFile().Return(file, nil).Once()
	file.EXPECT().Name().Return("icon.png")
	file.EXPECT().FileId().Return(domain.FileId(iconId))
	file.EXPECT().MimeType().Return("image/png")
	file.EXPECT().Meta().Return(&files.FileMeta{Name: "icon.png", Media: "image/png"})
	file.EXPECT().Reader(mock.Anything).Return(strings.NewReader("icon bytes"), nil).Once()
	path := "/v2/spaces/space1/files/" + iconId + "/content"
	w := serveWithKey(fx, http.MethodGet, path, "reader")
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, "icon bytes", w.Body.String())
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)

	view[bundle.RelationKeyIconImage] = domain.String("")
	fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{view})
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = localApiHost
	req.Header.Set("Authorization", "Bearer reader")
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	fx.Engine().ServeHTTP(w, req)
	require.Equal(t, 404, w.Code, "a removed reference must not get a cached 304")
	assert.NotContains(t, w.Body.String(), "icon bytes")
}
