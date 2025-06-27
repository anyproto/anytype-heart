package gallery

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

//go:embed testdata/schema.json
var schemaJSON []byte

func TestStripTags(t *testing.T) {
	bareString := `Links:FooBarBaz`
	taggedString := `<p>Links:</p><ul><li><a href="foo">Foo</a><li><a href="/bar/baz">BarBaz</a></ul><script>Malware that will destroy yor computer</script>`
	stripedString := stripTags(taggedString)
	assert.Equal(t, bareString, stripedString)
}

func TestIsInWhitelist(t *testing.T) {
	assert.True(t, IsInWhitelist("https://raw.githubusercontent.com/anyproto/secretrepo/blob/README.md"))
	assert.False(t, IsInWhitelist("https://raw.githubusercontent.com/fakeany/anyproto/secretrepo/blob/README.md"))
	assert.True(t, IsInWhitelist("ftp://raw.githubusercontent.com/anyproto/ftpserver/README.md"))
	assert.True(t, IsInWhitelist("http://github.com/anyproto/othersecretrepo/virus.exe"))
	assert.False(t, IsInWhitelist("ftp://github.com/anygroto/othersecretrepoclone/notAvirus.php?breakwhitelist=github.com/anyproto"))
	assert.True(t, IsInWhitelist("http://community.anytype.io/localstorage/knowledge_base.zip"))
	assert.True(t, IsInWhitelist("anytype://anytype.io/localstorage/knowledge_base.zip"))
	assert.True(t, IsInWhitelist("anytype://gallery.any.coop/"))
	assert.True(t, IsInWhitelist("anytype://tools.gallery.any.coop/somethingveryimportant.zip"))
	assert.True(t, IsInWhitelist("http://storage.gallery.any.coop/img_with_kitten.jpeg"))
}

func TestDownloadManifestAndValidateSchema(t *testing.T) {
	server := startHttpServer()
	defer server.Close()
	s := service{}

	t.Run("download knowledge base manifest", func(t *testing.T) {
		// given
		url := server.URL + "/manifest.json"

		// when
		info, err := s.getManifest(url, false, false)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
	})
	t.Run("provided info corresponds schema", func(t *testing.T) {
		// given
		info := buildInfo(server.URL, "Experience")

		// when
		err := validateManifestSchema(info)

		// then
		assert.NoError(t, err)
	})
	t.Run("some required fields are missing", func(t *testing.T) {
		// given
		info := buildInfo(server.URL, "Experience")
		info.Categories = nil
		info.Description = ""

		// when
		err := validateManifestSchema(info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "categories")
		assert.Contains(t, err.Error(), "description")
	})
	t.Run("short description", func(t *testing.T) {
		// given
		info := buildInfo(server.URL, "Experience")
		info.Description = "short"

		// when
		err := validateManifestSchema(info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "description")
		assert.Contains(t, err.Error(), "greater")
	})
	t.Run("not existing category", func(t *testing.T) {
		// given
		info := buildInfo(server.URL, "Experience")
		info.Categories = append(info.Categories, "Software Engineering")

		// when
		err := validateManifestSchema(info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "categories")
	})
	t.Run("author should be a github account", func(t *testing.T) {
		// given
		info := buildInfo(server.URL, "Experience")
		info.Author = "https://johnjohnsonpersonal.blog"

		// when
		err := validateManifestSchema(info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "author")
		assert.Contains(t, err.Error(), "github")
	})
}

func startHttpServer() *httptest.Server {
	handler := http.NewServeMux()
	handler.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		info := buildInfo("", "Experience")
		rawInfo, _ := json.Marshal(info)
		_, _ = w.Write(rawInfo)
	})
	handler.HandleFunc("/schema.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(schemaJSON)
	})
	return httptest.NewServer(handler)
}

func buildInfo(serverURL, name string) *model.ManifestInfo {
	return &model.ManifestInfo{
		Schema:       serverURL + "/schema.json",
		Id:           name,
		Name:         name,
		Author:       "https://github.com/anyproto",
		License:      "MIT",
		Title:        name,
		Description:  "Description of usecase",
		Screenshots:  []string{"https://anytype.io/assets/usecases/Knowledge%20base.jpg", "https://anytype.io/assets/usecases/Knowledge%20base_movie.jpg"},
		DownloadLink: "https://github.com/anyproto/gallery/raw/main/experiences/knowledge_base/knowledge_base.zip",
		FileSize:     42,
		Categories:   []string{"Education", "Work"},
		Language:     "hi-IN",
	}
}

func TestService_GetGalleryIndex(t *testing.T) {
	t.Run("successful fetch with no local cache", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		server := setupMockGalleryServer(t, mockGalleryResponse{
			statusCode: http.StatusOK,
			etag:       "v1.0.0",
			index:      buildIndex(),
		})
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Categories, 3)
		assert.Len(t, index.Experiences, 3)

		_, err = os.Stat(s.indexPath)
		assert.NoError(t, err)
		_, err = os.Stat(s.versionPath)
		assert.NoError(t, err)
	})

	t.Run("fetch with existing local cache - not modified", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		localIndex := buildIndex()
		saveTestIndex(t, s, localIndex, "v1.0.0")

		server := setupMockGalleryServer(t, mockGalleryResponse{
			statusCode: http.StatusNotModified,
			etag:       "v1.0.0",
		})
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Equal(t, localIndex.Categories[0].Id, index.Categories[0].Id)
	})

	t.Run("fetch with existing local cache - has update", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		oldIndex := buildIndex()
		oldIndex.Categories = oldIndex.Categories[:1]
		saveTestIndex(t, s, oldIndex, "v1.0.0")

		newIndex := buildIndex()
		server := setupMockGalleryServer(t, mockGalleryResponse{
			statusCode: http.StatusOK,
			etag:       "v2.0.0",
			index:      newIndex,
		})
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Categories, 3)
	})

	t.Run("network error with valid local cache fallback", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		localIndex := buildIndex()
		saveTestIndex(t, s, localIndex, "v1.0.0")

		server := setupMockGalleryServer(t, mockGalleryResponse{
			statusCode: http.StatusInternalServerError,
		})
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Equal(t, localIndex.Categories[0].Id, index.Categories[0].Id)
	})

	t.Run("network timeout with valid local cache fallback", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		localIndex := buildIndex()
		saveTestIndex(t, s, localIndex, "v1.0.0")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
		}))
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", 10*time.Millisecond)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Equal(t, localIndex.Categories[0].Id, index.Categories[0].Id)
	})

	t.Run("network error with no local cache", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		server := setupMockGalleryServer(t, mockGalleryResponse{
			statusCode: http.StatusInternalServerError,
		})
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.Error(t, err)
		assert.Nil(t, index)
	})

	t.Run("corrupted local index file", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		err := os.WriteFile(s.indexPath, []byte("corrupted data"), 0600)
		require.NoError(t, err)
		err = os.WriteFile(s.versionPath, []byte("v1.0.0"), 0600)
		require.NoError(t, err)

		server := setupMockGalleryServer(t, mockGalleryResponse{
			statusCode: http.StatusOK,
			etag:       "v2.0.0",
			index:      buildIndex(),
		})
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Categories, 3)
	})

	t.Run("corrupted version file", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		localIndex := buildIndex()
		data, err := proto.Marshal(localIndex)
		require.NoError(t, err)
		err = os.WriteFile(s.indexPath, data, 0600)
		require.NoError(t, err)

		server := setupMockGalleryServer(t, mockGalleryResponse{
			statusCode: http.StatusOK,
			etag:       "v2.0.0",
			index:      buildIndex(),
		})
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
	})

	t.Run("invalid JSON response from server", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		localIndex := buildIndex()
		saveTestIndex(t, s, localIndex, "v1.0.0")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("ETag", "v2.0.0")
			w.Write([]byte("invalid json response"))
		}))
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Equal(t, localIndex.Categories[0].Id, index.Categories[0].Id)
	})

	t.Run("server returns empty response", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		localIndex := buildIndex()
		saveTestIndex(t, s, localIndex, "v1.0.0")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("ETag", "v2.0.0")
			w.Write([]byte("{}"))
		}))
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Categories, 0)
		assert.Len(t, index.Experiences, 0)
	})

	t.Run("ETag header handling", func(t *testing.T) {
		// given
		s, cleanup := setupTestService(t)
		defer cleanup()

		saveTestIndex(t, s, buildIndex(), "v1.0.0")

		var receivedETag string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedETag = r.Header.Get("If-None-Match")
			w.WriteHeader(http.StatusNotModified)
		}))
		defer server.Close()

		// when
		_, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "v1.0.0", receivedETag)
	})

	t.Run("file system permission error", func(t *testing.T) {
		// given
		tempDir, err := os.MkdirTemp("", "gallery_test_readonly_*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		s := &service{
			indexPath:   filepath.Join(tempDir, indexFileName),
			versionPath: filepath.Join(tempDir, eTagFileName),
		}

		err = os.Chmod(tempDir, 0444)
		require.NoError(t, err)
		defer os.Chmod(tempDir, 0755)

		server := setupMockGalleryServer(t, mockGalleryResponse{
			statusCode: http.StatusOK,
			etag:       "v1.0.0",
			index:      buildIndex(),
		})
		defer server.Close()

		// when
		index, err := s.getGalleryIndex(server.URL+"/index.json", defaultTimeout)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Categories, 3)
	})
}

type mockGalleryResponse struct {
	statusCode int
	etag       string
	index      *pb.RpcGalleryDownloadIndexResponse
}

func setupMockGalleryServer(t *testing.T, response mockGalleryResponse) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if response.etag != "" {
			w.Header().Set("ETag", response.etag)
		}

		w.WriteHeader(response.statusCode)

		if response.statusCode == http.StatusOK && response.index != nil {
			indexJSON, err := json.Marshal(response.index)
			require.NoError(t, err)
			w.Write(indexJSON)
		}
	}))
}

func saveTestIndex(t *testing.T, s *service, index *pb.RpcGalleryDownloadIndexResponse, version string) {
	data, err := proto.Marshal(index)
	require.NoError(t, err)

	err = os.WriteFile(s.indexPath, data, 0600)
	require.NoError(t, err)

	err = os.WriteFile(s.versionPath, []byte(version), 0600)
	require.NoError(t, err)
}

func buildIndex() *pb.RpcGalleryDownloadIndexResponse {
	return &pb.RpcGalleryDownloadIndexResponse{
		Categories: []*pb.RpcGalleryDownloadIndexResponseCategory{
			{"work", []string{"RnD", "KPI", "CRM", "PARA"}, "🧑‍💻"},
			{"life", []string{"Travel planner", "Plant database"}, "🌹"},
			{"balance", []string{"Yin", "yang"}, "☯️"},
		},
		Experiences: []*model.ManifestInfo{
			buildInfo("", "1"),
			buildInfo("", "2"),
			buildInfo("", "3"),
		},
	}
}

func setupTestService(t *testing.T) (*service, func()) {
	tempDir, err := os.MkdirTemp("", "gallery_test_*")
	require.NoError(t, err)

	s := &service{
		indexPath:   filepath.Join(tempDir, indexFileName),
		versionPath: filepath.Join(tempDir, eTagFileName),
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return s, cleanup
}
