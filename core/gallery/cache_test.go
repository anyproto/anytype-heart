package gallery

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestIndexCache_GetIndex(t *testing.T) {
	t.Run("get index from cache, failed to retrieve remote cache", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		c := cache{
			indexURL: server.URL + "/index.json",
			storage: &testCacheStorage{
				index:   &pb.RpcGalleryDownloadIndexResponse{},
				version: "v1",
			},
		}

		// when
		_, err := c.GetIndex(0)

		// then
		assert.NoError(t, err)
	})

	t.Run("get index from remote, version differs", func(t *testing.T) {
		// given
		server := buildServer(t, "")
		defer server.Close()

		c := cache{
			indexURL: server.URL + "/index.json",
			storage: &testCacheStorage{
				index:   &pb.RpcGalleryDownloadIndexResponse{},
				version: "v1",
				assertSave: func(index *pb.RpcGalleryDownloadIndexResponse, version string) {
					assert.Equal(t, "v2", version)
					assert.NotNil(t, index)
					assert.Len(t, index.Experiences, 1)
					assert.Equal(t, "name", index.Experiences[0].Name)
				},
			},
		}

		// when
		index, err := c.GetIndex(0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "name", index.Experiences[0].Name)
	})

	t.Run("get index from remote, version is the same", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotModified)
		}))
		defer server.Close()

		c := cache{
			indexURL: server.URL + "/index.json",
			storage: &testCacheStorage{
				index:   &pb.RpcGalleryDownloadIndexResponse{},
				version: "v2",
			},
		}

		// when
		index, err := c.GetIndex(0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 0)
	})

	t.Run("failed to read local index", func(t *testing.T) {
		// given
		server := buildServer(t, "")
		defer server.Close()

		c := cache{
			indexURL: server.URL + "/index.json",
			storage: &testCacheStorage{
				version: "v1",
				assertSave: func(index *pb.RpcGalleryDownloadIndexResponse, version string) {
					assert.Equal(t, "v2", version)
					assert.NotNil(t, index)
					assert.Len(t, index.Experiences, 1)
				},
			},
		}

		// when
		index, err := c.GetIndex(0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "name", index.Experiences[0].Name)
	})

	t.Run("failed to both read local index and download remote one", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()
		c := cache{
			indexURL: server.URL + "/index.json",
			storage:  &testCacheStorage{},
		}

		// when
		_, err := c.GetIndex(0)

		// then
		assert.Error(t, err)
	})
}

func TestIndexCache_GetManifest(t *testing.T) {
	const link = "https://github.com/anyproto/gallery/raw/main/experiences/knowledge_base/knowledge_base.zip"

	t.Run("get manifest from cache, failed to fetch index from remote", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		c := cache{
			indexURL: server.URL + "/index.json",
			storage: &testCacheStorage{
				index: &pb.RpcGalleryDownloadIndexResponse{Experiences: []*model.ManifestInfo{{
					DownloadLink: "test.link",
					Name:         "test",
				}}},
				version: "v1",
			},
		}

		// when
		info, err := c.GetManifest("test.link", 0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "test", info.Name)
	})

	t.Run("get manifest from remote, version differs", func(t *testing.T) {
		// given
		server := buildServer(t, "")
		defer server.Close()

		c := cache{
			indexURL: server.URL + "/index.json",
			storage: &testCacheStorage{
				index:   &pb.RpcGalleryDownloadIndexResponse{},
				version: "v1",
				assertSave: func(index *pb.RpcGalleryDownloadIndexResponse, version string) {
					assert.Equal(t, "v2", version)
					assert.NotNil(t, index)
					assert.Len(t, index.Experiences, 1)
				},
			},
		}

		// when
		info, err := c.GetManifest(link, 0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "name", info.Name)
	})

	t.Run("get manifest from remote, version is the same", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotModified)
		}))
		defer server.Close()

		c := cache{
			indexURL: server.URL + "/index.json",
			storage: &testCacheStorage{
				index: &pb.RpcGalleryDownloadIndexResponse{Experiences: []*model.ManifestInfo{{
					DownloadLink: "test.link",
					Name:         "test",
				}}},
				version: "v2",
			},
		}

		// when
		info, err := c.GetManifest("test.link", 0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "test", info.Name)
	})

	t.Run("failed to read local index", func(t *testing.T) {
		// given
		server := buildServer(t, "")
		defer server.Close()

		c := cache{
			indexURL: server.URL + "/index.json",
			storage: &testCacheStorage{
				version: "v1",
				assertSave: func(index *pb.RpcGalleryDownloadIndexResponse, version string) {
					assert.Equal(t, "v2", version)
					assert.NotNil(t, index)
					assert.Len(t, index.Experiences, 1)
				},
			},
		}

		// when
		info, err := c.GetManifest(link, 0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "name", info.Name)
	})

	t.Run("failed to both read local index and download remote one", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()

		c := cache{
			indexURL: server.URL + "/index.json",
			storage:  &testCacheStorage{},
		}

		// when
		_, err := c.GetManifest("link", 0)

		// then
		assert.Error(t, err)
	})
}
