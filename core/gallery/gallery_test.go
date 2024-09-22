package gallery

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	port        = ":7070"
	testVersion = "v2"
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

func TestDownloadManifestAndValidateSchema(t *testing.T) {
	schema := "http://localhost" + port + "/schema.json"
	server := startHttpServer()
	defer server.Shutdown(nil)

	t.Run("download knowledge base manifest", func(t *testing.T) {
		// given
		url := "http://localhost" + port + "/manifest.json"

		// when
		info, err := DownloadManifest(url, false)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
	})
	t.Run("provided info corresponds schema", func(t *testing.T) {
		// given
		info := buildInfo()

		// when
		err := validateSchema(schema, info)

		// then
		assert.NoError(t, err)
	})
	t.Run("some required fields are missing", func(t *testing.T) {
		// given
		info := buildInfo()
		info.Categories = nil
		info.Description = ""

		// when
		err := validateSchema(schema, info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "categories")
		assert.Contains(t, err.Error(), "description")
	})
	t.Run("short description", func(t *testing.T) {
		// given
		info := buildInfo()
		info.Description = "short"

		// when
		err := validateSchema(schema, info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "description")
		assert.Contains(t, err.Error(), "greater")
	})
	t.Run("not existing category", func(t *testing.T) {
		// given
		info := buildInfo()
		info.Categories = append(info.Categories, "Software Engineering")

		// when
		err := validateSchema(schema, info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "categories")
	})
	t.Run("author should be a github account", func(t *testing.T) {
		// given
		info := buildInfo()
		info.Author = "https://johnjohnsonpersonal.blog"

		// when
		err := validateSchema(schema, info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "author")
		assert.Contains(t, err.Error(), "github")
	})
}

func startHttpServer() *http.Server {
	handler := http.NewServeMux()
	handler.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		info := buildInfo()
		rawInfo, _ := json.Marshal(info)
		_, _ = w.Write(rawInfo)
	})
	handler.HandleFunc("/schema.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(schemaJSON)
	})
	handler.HandleFunc("/index.json", func(w http.ResponseWriter, req *http.Request) {
		version := req.Header.Get(versionHeader)
		if version == testVersion {
			w.WriteHeader(http.StatusNotModified)
		} else {
			w.Header().Set(eTagHeader, testVersion)
		}
		w.Header().Set("Content-Type", "application/json")
		info := buildIndex()
		rawInfo, _ := json.Marshal(info)
		_, _ = w.Write(rawInfo)
	})
	server := &http.Server{Addr: port, Handler: handler}
	go server.ListenAndServe()
	return server
}

func buildInfo() *model.ManifestInfo {
	return &model.ManifestInfo{
		Id:           "id",
		Name:         "name",
		Author:       "https://github.com/anyproto",
		License:      "MIT",
		Title:        "Name",
		Description:  "Description of usecase",
		Screenshots:  []string{"https://anytype.io/assets/usecases/Knowledge%20base.jpg", "https://anytype.io/assets/usecases/Knowledge%20base_movie.jpg"},
		DownloadLink: "https://github.com/anyproto/gallery/raw/main/experiences/knowledge_base/knowledge_base.zip",
		FileSize:     42,
		Categories:   []string{"Education", "Work"},
		Language:     "hi-IN",
	}
}

func buildIndex() *pb.RpcGalleryDownloadIndexResponse {
	return &pb.RpcGalleryDownloadIndexResponse{
		Experiences: []*model.ManifestInfo{
			buildInfo(),
		},
	}
}
