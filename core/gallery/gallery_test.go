package gallery

import (
	_ "embed"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestStripTags(t *testing.T) {
	bareString := `Links:FooBarBaz`
	taggedString := `<p>Links:</p><ul><li><a href="foo">Foo</a><li><a href="/bar/baz">BarBaz</a></ul><script>Malware that will destroy yor computer</script>`
	info := &model.ManifestInfo{Description: taggedString}
	stripTags(info)
	assert.Equal(t, bareString, info.Description)
}

func TestIsInWhitelist(t *testing.T) {
	assert.True(t, isInWhitelist("https://raw.githubusercontent.com/anyproto/secretrepo/blob/README.md"))
	assert.False(t, isInWhitelist("https://raw.githubusercontent.com/fakeany/anyproto/secretrepo/blob/README.md"))
	assert.True(t, isInWhitelist("ftp://raw.githubusercontent.com/anyproto/ftpserver/README.md"))
	assert.True(t, isInWhitelist("http://github.com/anyproto/othersecretrepo/virus.exe"))
	assert.False(t, isInWhitelist("ftp://github.com/anygroto/othersecretrepoclone/notAvirus.php?breakwhitelist=github.com/anyproto"))
	assert.True(t, isInWhitelist("http://community.anytype.io/localstorage/knowledge_base.zip"))
	assert.True(t, isInWhitelist("anytype://anytype.io/localstorage/knowledge_base.zip"))
	assert.True(t, isInWhitelist("anytype://gallery.any.coop/"))
	assert.True(t, isInWhitelist("anytype://tools.gallery.any.coop/somethingveryimportant.zip"))
	assert.True(t, isInWhitelist("http://storage.gallery.any.coop/img_with_kitten.jpeg"))
}

func TestService_GetManifest(t *testing.T) {
	t.Run("download manifest from remote", func(t *testing.T) {
		// given
		server := buildServer(t, "")
		defer server.Close()

		s := service{}

		// when
		info, err := s.GetManifest(server.URL+"/manifest.json", false)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
	})

	t.Run("failed to get manifest from remote", func(t *testing.T) {
		// when
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		s := service{}

		// when
		_, err := s.GetManifest(server.URL, false)

		// then
		assert.Error(t, err)
	})
}

func TestValidateSchema(t *testing.T) {
	server := buildServer(t, "")
	defer server.Close()
	t.Run("provided info corresponds schema", func(t *testing.T) {
		// given
		info := buildInfo("")

		// when
		err := validateSchema(info.Schema, info)

		// then
		assert.NoError(t, err)
	})
	t.Run("some required fields are missing", func(t *testing.T) {
		// given
		info := buildInfo("")
		info.Categories = nil
		info.Description = ""

		// when
		err := validateSchema(info.Schema, info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "categories")
		assert.Contains(t, err.Error(), "description")
	})
	t.Run("short description", func(t *testing.T) {
		// given
		info := buildInfo("")
		info.Description = "short"

		// when
		err := validateSchema(info.Schema, info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "description")
		assert.Contains(t, err.Error(), "greater")
	})
	t.Run("not existing category", func(t *testing.T) {
		// given
		info := buildInfo("")
		info.Categories = append(info.Categories, "Software Engineering")

		// when
		err := validateSchema(info.Schema, info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "categories")
	})
	t.Run("author should be a github account", func(t *testing.T) {
		// given
		info := buildInfo("")
		info.Author = "https://johnjohnsonpersonal.blog"

		// when
		err := validateSchema(info.Schema, info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "author")
		assert.Contains(t, err.Error(), "github")
	})
}

func TestService_GetGalleryIndex(t *testing.T) {
	t.Run("get gallery index from middleware cache", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()

		fx := newFixture(t)
		fx.indexCache.storage = &testCacheStorage{
			index:   buildIndex(""),
			version: "v1",
		}
		fx.indexCache.indexURL = server.URL

		// when
		index, err := fx.GetGalleryIndex("")

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "name", index.Experiences[0].Name)
	})

	t.Run("get gallery index from client cache", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()

		fx := newFixture(t)
		fx.indexCache.storage = &testCacheStorage{}
		fx.indexCache.indexURL = server.URL

		// when
		index, err := fx.GetGalleryIndex("./testdata/client_cache.zip")

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "get_started", index.Experiences[0].Name)
	})

	t.Run("get gallery index from remote", func(t *testing.T) {
		// given
		server := buildServer(t, "")
		defer server.Close()

		fx := newFixture(t)
		fx.indexCache.storage = &testCacheStorage{
			assertSave: func(index *pb.RpcGalleryDownloadIndexResponse, version string) {
				assert.NotNil(t, index)
				assert.Len(t, index.Experiences, 1)
				assert.Equal(t, "name", index.Experiences[0].Name)
				assert.Equal(t, "v2", version)
			},
		}
		fx.indexCache.indexURL = server.URL + "/index.json"

		// when
		index, err := fx.GetGalleryIndex("./testdata/client_cache.zip")

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "name", index.Experiences[0].Name)
	})

	t.Run("slow internet", func(t *testing.T) {
		// given
		server := buildServer(t, "")
		defer server.Close()

		fx := newFixture(t)
		fx.indexCache.storage = &testCacheStorage{
			assertSave: func(index *pb.RpcGalleryDownloadIndexResponse, version string) {
				assert.NotNil(t, index)
				assert.Len(t, index.Experiences, 1)
				assert.Equal(t, "name", index.Experiences[0].Name)
				assert.Equal(t, "v2", version)
			},
		}
		fx.indexCache.indexURL = server.URL + "/index.json"

		// when
		index, err := fx.GetGalleryIndex("invalid_path")

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "name", index.Experiences[0].Name)
	})

	t.Run("failed to get index from all places", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()

		fx := newFixture(t)
		fx.indexCache.storage = &testCacheStorage{}
		fx.indexCache.indexURL = server.URL

		// when
		_, err := fx.GetGalleryIndex("invalid_path")

		// then
		assert.Error(t, err)
	})
}
