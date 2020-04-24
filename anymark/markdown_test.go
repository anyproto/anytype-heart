package anymark

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommonSmart_importFromMarkdown(t *testing.T) {
	t.Run("No marks, paste middleCut to the middle", func(t *testing.T) {
		anymarkConv := New()
		dir := "md-import-files_test"
		nameToBlocks, _, err := anymarkConv.DirWithMarkdownToBlocks(dir)

		if err != nil {
			assert.NoError(t, err)
		}

		assert.Equal(t, len(nameToBlocks), 24)
	})
}
