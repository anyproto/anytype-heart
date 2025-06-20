package gallery

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

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
		info := buildInfo(server.URL)

		// when
		err := validateManifestSchema(info)

		// then
		assert.NoError(t, err)
	})
	t.Run("some required fields are missing", func(t *testing.T) {
		// given
		info := buildInfo(server.URL)
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
		info := buildInfo(server.URL)
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
		info := buildInfo(server.URL)
		info.Categories = append(info.Categories, "Software Engineering")

		// when
		err := validateManifestSchema(info)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "categories")
	})
	t.Run("author should be a github account", func(t *testing.T) {
		// given
		info := buildInfo(server.URL)
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
		info := buildInfo("")
		rawInfo, _ := json.Marshal(info)
		_, _ = w.Write(rawInfo)
	})
	handler.HandleFunc("/schema.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(schemaJSON)
	})
	return httptest.NewServer(handler)
}

func buildInfo(serverURL string) *model.ManifestInfo {
	return &model.ManifestInfo{
		Schema:       serverURL + "/schema.json",
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
