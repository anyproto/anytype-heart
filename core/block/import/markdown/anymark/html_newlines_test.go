package anymark

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// collectText concatenates the text of all top-level paragraph text blocks.
func collectText(blocks []*model.Block) []string {
	var texts []string
	for _, b := range blocks {
		if t := b.GetText(); t != nil {
			texts = append(texts, t.Text)
		}
	}
	return texts
}

func TestHTMLInlineNewlinesCollapse(t *testing.T) {
	t.Run("strong content newlines collapse", func(t *testing.T) {
		src := `<html><body><strong>000
            111
            222
        </strong></body></html>`

		blocks, _, err := HTMLToBlocks([]byte(src), "http://test.com/test")
		require.NoError(t, err)

		joined := strings.Join(collectText(blocks), "\n")
		assert.Equal(t, "000 111 222", joined,
			"source newlines inside <strong> must collapse to spaces, got:\n%q", joined)
		assert.NotContains(t, joined, "\n")
	})

	t.Run("anchor content newlines collapse", func(t *testing.T) {
		src := `<html><body><a href="https://anytype.io/">abc
            def
            hij</a></body></html>`

		blocks, _, err := HTMLToBlocks([]byte(src), "http://test.com/test")
		require.NoError(t, err)

		joined := strings.Join(collectText(blocks), "\n")
		assert.Equal(t, "abc def hij", joined,
			"source newlines inside <a> must collapse to spaces, got:\n%q", joined)
		assert.NotContains(t, joined, "\n")
	})

	t.Run("pre block keeps newlines", func(t *testing.T) {
		src := `<html><body>
        <pre>line one
line two</pre>
    </body></html>`

		blocks, _, err := HTMLToBlocks([]byte(src), "http://test.com/test")
		require.NoError(t, err)

		var codeText string
		for _, b := range blocks {
			if t := b.GetText(); t != nil && t.Style == model.BlockContentText_Code {
				codeText = t.Text
			}
		}
		assert.Contains(t, codeText, "\n", "whitespace in <pre> must be preserved, got:\n%q", codeText)
		assert.Contains(t, codeText, "line one", "got:\n%q", codeText)
	})
}
