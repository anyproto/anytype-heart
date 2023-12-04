package gallery

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
)

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
}

func TestDownloadManifest(t *testing.T) {
	t.Run("download knowledge base manifest", func(t *testing.T) {
		// given
		url := "https://raw.githubusercontent.com/anyproto/gallery/main/experiences/knowledge_base/manifest.json"

		// when
		info, err := DownloadManifest(url)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
	})
}

// TODO: This bunch of tests depends on Schema content, so whether versioning must be supported or tests disabled
func TestValidateSchema(t *testing.T) {
	schema := schemaResponse{Schema: "https://gallery.any.coop/experience.schema.json"}

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

func buildInfo() *pb.RpcDownloadManifestResponseManifestInfo {
	return &pb.RpcDownloadManifestResponseManifestInfo{
		Id:           "id",
		Name:         "name",
		Author:       "https://github.com/anyproto",
		License:      "MIT",
		Title:        "Name",
		Description:  "Description of usecase",
		Screenshots:  []string{"https://anytype.io/assets/usecases/Knowledge%20base.jpg", "https://anytype.io/assets/usecases/Knowledge%20base_movie.jpg"},
		DownloadLink: "https://github.com/anyproto/gallery/raw/main/experiences/knowledge_base/knowledge_base.zip",
		FileSize:     42,
		Categories:   []string{"Education", "Hobbies"},
		Language:     "hi-IN",
	}
}
