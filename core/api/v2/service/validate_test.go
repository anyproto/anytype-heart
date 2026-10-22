package v2service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV2ValidateDocument(t *testing.T) {
	t.Run("valid document yields empty lists", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		doc := []byte(`{"formatVersion":"2.0","blocks":[{"type":"paragraph","text":"hello"}]}`)

		// when
		resp := fx.ValidateDocument(doc)

		// then
		assert.Empty(t, resp.Issues)
		assert.Empty(t, resp.Warnings)
		assert.NotNil(t, resp.Issues, "issues serializes as [] not null")
	})

	t.Run("schema violations are path-addressed", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		doc := []byte(`{"formatVersion":"2.0","blocks":[{"type":"nonsense"}]}`)

		// when
		resp := fx.ValidateDocument(doc)

		// then
		require.NotEmpty(t, resp.Issues)
		assert.Contains(t, resp.Issues[0].Path, "/blocks/0")
	})

	t.Run("newer format version names both versions with a hint", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		doc := []byte(`{"formatVersion":"99.0","blocks":[]}`)

		// when
		resp := fx.ValidateDocument(doc)

		// then
		require.NotEmpty(t, resp.Issues)
		issue := resp.Issues[0]
		assert.Equal(t, "/formatVersion", issue.Path)
		assert.Contains(t, issue.Message, "99.0")
		assert.Contains(t, issue.Message, "2.0")
		assert.Contains(t, issue.Hint, "newer version")
	})

	t.Run("malformed JSON reports as an issue, not an error", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		resp := fx.ValidateDocument([]byte(`{not json`))

		// then
		require.NotEmpty(t, resp.Issues)
		assert.Contains(t, resp.Issues[0].Message, "invalid JSON")
	})
}
