package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestSchemaImporter_ResolvePropertyKey_ScopedByObjectType verifies that when
// multiple schemas declare properties with the same display name but different
// x-keys, the resolver returns the key from the schema matching the file's
// object type. An empty object type name must fall back to a global scan so
// cross-type lookups still work.
func TestSchemaImporter_ResolvePropertyKey_ScopedByObjectType(t *testing.T) {
	moodBoardSchema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Mood Board",
		"x-app": "Anytype",
		"x-type-key": "mood_board",
		"properties": {
			"References": {
				"type": "array",
				"x-key": "mood_board_references",
				"x-format": "object",
				"items": {"type": "object"}
			}
		}
	}`
	pieceSchema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Piece",
		"x-app": "Anytype",
		"x-type-key": "piece",
		"properties": {
			"References": {
				"type": "array",
				"x-key": "piece_references",
				"x-format": "object",
				"items": {"type": "object"}
			}
		}
	}`

	si := NewSchemaImporter()
	src := &mockSource{files: map[string]string{
		"mood_board.json": moodBoardSchema,
		"piece.json":      pieceSchema,
	}}
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	require.NoError(t, si.LoadSchemas(src, allErrors))

	// Mood Board file must get mood_board_references, not piece_references.
	assert.Equal(t, "mood_board_references", si.ResolvePropertyKey("Mood Board", "References"))

	// Piece file must get piece_references, not mood_board_references.
	assert.Equal(t, "piece_references", si.ResolvePropertyKey("Piece", "References"))

	// Empty object type falls back to any matching relation.
	assert.NotEmpty(t, si.ResolvePropertyKey("", "References"))

	// Format is returned for both scopes.
	assert.Equal(t, model.RelationFormat_object, si.GetRelationFormat("Mood Board", "mood_board_references"))
	assert.Equal(t, model.RelationFormat_object, si.GetRelationFormat("Piece", "piece_references"))
}
