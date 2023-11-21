package gallery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripTags(t *testing.T) {
	bareString := `Links:FooBarBaz`
	taggedString := `<p>Links:</p><ul><li><a href="foo">Foo</a><li><a href="/bar/baz">BarBaz</a></ul><script>Malware that will destroy yor computer</script>`
	stripedString := stripTags(taggedString)
	assert.Equal(t, bareString, stripedString)
}

func TestIsInWhitelist(t *testing.T) {
	assert.True(t, isInWhitelist(t, "https://raw.githubusercontent.com/anyproto/secretrepo/blob/README.md"))
	assert.False(t, isInWhitelist(t, "https://raw.githubusercontent.com/fakeany/anyproto/secretrepo/blob/README.md"))
	assert.True(t, isInWhitelist(t, "ftp://raw.githubusercontent.com/anyproto/ftpserver/README.md"))
	assert.True(t, isInWhitelist(t, "http://github.com/anyproto/othersecretrepo/virus.exe"))
	assert.False(t, isInWhitelist(t, "ftp://github.com/anygroto/othersecretrepoclone/notAvirus.php?breakwhitelist=github.com/anyproto"))
	assert.True(t, isInWhitelist(t, "http://community.anytype.io/localstorage/knowledge_base.zip"))
	assert.True(t, isInWhitelist(t, "anytype://anytype.io/localstorage/knowledge_base.zip"))
	assert.True(t, isInWhitelist(t, "anytype://gallery.any.coop/"))
}

func isInWhitelist(t *testing.T, url string) bool {
	isInWhitelist, err := IsInWhitelist(url)
	if err != nil {
		assert.Errorf(t, err, errURLNotInWhitelist, url)
	}
	return isInWhitelist
}
