package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

// TestSchemaImporter_CollectsOptionsFromYAML verifies that when a tag
// relation is declared in schema without an examples list, the importer
// still creates option snapshots for option values encountered in YAML
// files via RegisterOptionValue.
func TestSchemaImporter_CollectsOptionsFromYAML(t *testing.T) {
	si := NewSchemaImporter()
	s := schema.NewSchema()
	if err := s.AddRelation(&schema.Relation{
		Key:    "mood_board_mood_words",
		Name:   "Mood Words",
		Format: model.RelationFormat_tag,
	}); err != nil {
		t.Fatalf("AddRelation: %v", err)
	}
	si.schemas["mood_board.json"] = s

	// Register an option value the way createSnapshots will once the fix
	// is wired up.
	id := si.RegisterOptionValue("mood_board_mood_words", "tactile")
	assert.NotEmpty(t, id, "expected RegisterOptionValue to return a non-empty option ID")

	snapshots := si.CreateRelationOptionSnapshots()

	var found bool
	for _, snap := range snapshots {
		if snap.Id == id {
			found = true
			break
		}
	}
	assert.True(t, found, "expected a snapshot for the YAML-registered option")
}
