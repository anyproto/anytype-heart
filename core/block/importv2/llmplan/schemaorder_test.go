package llmplan

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
)

// The property order in the emitted schema is the model's generation order
// (constrained decoding follows the schema's declaration order), so it is
// pinned here rather than left to whatever a map happens to produce.
func TestKindsSchemaPropertyOrder(t *testing.T) {
	// given
	want := []string{"containers", "featured", "icon", "layout", "name_plural", "name_singular"}

	// when — read the order off the raw bytes, since that is what the server sees
	var doc struct {
		Properties struct {
			Kinds struct {
				Items struct {
					Properties json.RawMessage `json:"properties"`
				} `json:"items"`
			} `json:"kinds"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(kindsResponseSchema, &doc))

	var got []string
	decoder := json.NewDecoder(bytes.NewReader(doc.Properties.Kinds.Items.Properties))
	tok, err := decoder.Token() // opening {
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), tok)
	for decoder.More() {
		key, err := decoder.Token()
		require.NoError(t, err)
		got = append(got, key.(string))
		var skip json.RawMessage
		require.NoError(t, decoder.Decode(&skip))
	}

	// then
	assert.Equal(t, want, got, "generation order changed — this alters naming quality")
	assert.NotEmpty(t, schemaplan.AllowedIcons)
}
