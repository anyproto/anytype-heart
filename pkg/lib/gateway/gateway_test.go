package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanUpPathForLogging(t *testing.T) {
	t.Run("with CID in path", func(t *testing.T) {
		path := "/image/bafybeihjujzgyuzjmwc4ar7xpkobgvrxv6jmsyfhxp4mypiexlyrs2y2zu"
		got := cleanUpPathForLogging(path)
		assert.Equal(t, path, got)

		path = "/file/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"
		got = cleanUpPathForLogging(path)
		assert.Equal(t, path, got)
	})

	t.Run("with something else in path", func(t *testing.T) {
		path := "/file/https:/example.com/foo/bar"
		got := cleanUpPathForLogging(path)
		want := "/file/<masked invalid path>"
		assert.Equal(t, want, got)
	})
}
