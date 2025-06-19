package schema

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Mock resolver for testing
type mockObjectResolver struct {
	relations      map[string]*domain.Details
	relationOptions map[string][]*domain.Details
}

func (m *mockObjectResolver) ResolveRelation(relationId string) (*domain.Details, error) {
	if rel, ok := m.relations[relationId]; ok {
		return rel, nil
	}
	return nil, nil
}

func (m *mockObjectResolver) ResolveRelationOptions(relationKey string) ([]*domain.Details, error) {
	if options, ok := m.relationOptions[relationKey]; ok {
		return options, nil
	}
	return nil, nil
}

func TestSchemaFromObjectDetailsWithResolver_RelationOptions(t *testing.T) {
	// Create type details
	typeDetails := domain.NewDetails()
	typeDetails.SetString(bundle.RelationKeyId, "task-type-id")
	typeDetails.SetString(bundle.RelationKeyName, "Task")
	typeDetails.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))
	typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"status-rel-id"})
	typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{})
	// Set unique key to allow type key extraction
	uniqueKey, _ := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, "task")
	typeDetails.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())

	// Create status relation details
	statusRelDetails := domain.NewDetails()
	statusRelDetails.SetString(bundle.RelationKeyId, "status-rel-id")
	statusRelDetails.SetString(bundle.RelationKeyName, "Status")
	statusRelDetails.SetString(bundle.RelationKeyRelationKey, "status")
	statusRelDetails.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_status))
	statusRelDetails.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))

	// Create relation option details
	option1 := domain.NewDetails()
	option1.SetString(bundle.RelationKeyName, "Open")
	option1.SetString(bundle.RelationKeyRelationKey, "status")
	option1.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))

	option2 := domain.NewDetails()
	option2.SetString(bundle.RelationKeyName, "Done")
	option2.SetString(bundle.RelationKeyRelationKey, "status")
	option2.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))

	// Create mock resolver
	resolver := &mockObjectResolver{
		relations: map[string]*domain.Details{
			"status-rel-id": statusRelDetails,
		},
		relationOptions: map[string][]*domain.Details{
			"status": {option1, option2},
		},
	}

	// Create schema using the resolver
	schema, err := SchemaFromObjectDetailsWithResolver(
		typeDetails,
		[]*domain.Details{statusRelDetails},
		resolver,
	)
	require.NoError(t, err)
	require.NotNil(t, schema)

	// Check that relation options were populated
	statusRel, exists := schema.GetRelation("status")
	require.True(t, exists)
	assert.Equal(t, model.RelationFormat_status, statusRel.Format)
	assert.Equal(t, []string{"Open", "Done"}, statusRel.Options)

	// Export schema and check that options appear in JSON
	exporter := NewJSONSchemaExporter("  ")
	var buf bytes.Buffer
	err = exporter.Export(schema, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"Open"`)
	assert.Contains(t, output, `"Done"`)
	assert.Contains(t, output, `"enum"`)
	assert.Contains(t, output, `"x-format": "status"`)
	// Verify that internal IDs are not exported
	assert.NotContains(t, output, `"id": "status-rel-id"`)
	assert.NotContains(t, output, `"id": "task-type-id"`)
}

func TestSchemaFromObjectDetailsWithResolver_TagRelationOptions(t *testing.T) {
	// Create type details
	typeDetails := domain.NewDetails()
	typeDetails.SetString(bundle.RelationKeyId, "note-type-id")
	typeDetails.SetString(bundle.RelationKeyName, "Note")
	typeDetails.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))
	typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"tags-rel-id"})
	typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{})
	// Set unique key to allow type key extraction
	uniqueKey, _ := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, "note")
	typeDetails.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())

	// Create tag relation details
	tagsRelDetails := domain.NewDetails()
	tagsRelDetails.SetString(bundle.RelationKeyId, "tags-rel-id")
	tagsRelDetails.SetString(bundle.RelationKeyName, "Tags")
	tagsRelDetails.SetString(bundle.RelationKeyRelationKey, "tags")
	tagsRelDetails.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_tag))
	tagsRelDetails.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))

	// Create relation option details
	tag1 := domain.NewDetails()
	tag1.SetString(bundle.RelationKeyName, "Important")
	tag1.SetString(bundle.RelationKeyRelationKey, "tags")
	tag1.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))

	tag2 := domain.NewDetails()
	tag2.SetString(bundle.RelationKeyName, "Urgent")
	tag2.SetString(bundle.RelationKeyRelationKey, "tags")
	tag2.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))

	// Create mock resolver
	resolver := &mockObjectResolver{
		relations: map[string]*domain.Details{
			"tags-rel-id": tagsRelDetails,
		},
		relationOptions: map[string][]*domain.Details{
			"tags": {tag1, tag2},
		},
	}

	// Create schema using the resolver
	schema, err := SchemaFromObjectDetailsWithResolver(
		typeDetails,
		[]*domain.Details{tagsRelDetails},
		resolver,
	)
	require.NoError(t, err)
	require.NotNil(t, schema)

	// Check that relation options were populated as examples for tag relations
	tagsRel, exists := schema.GetRelation("tags")
	require.True(t, exists)
	assert.Equal(t, model.RelationFormat_tag, tagsRel.Format)
	assert.Equal(t, []string{"Important", "Urgent"}, tagsRel.Examples)

	// Export schema and check that examples appear in JSON
	exporter := NewJSONSchemaExporter("  ")
	var buf bytes.Buffer
	err = exporter.Export(schema, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"Important"`)
	assert.Contains(t, output, `"Urgent"`)
	assert.Contains(t, output, `"examples"`)
	assert.Contains(t, output, `"x-format": "tag"`)
	// Verify that internal IDs are not exported
	assert.NotContains(t, output, `"id": "tags-rel-id"`)
	assert.NotContains(t, output, `"id": "note-type-id"`)
}

func TestSchemaFromObjectDetails_BackwardCompatibility(t *testing.T) {
	// Test that the original function still works without relation options
	typeDetails := domain.NewDetails()
	typeDetails.SetString(bundle.RelationKeyId, "task-type-id")
	typeDetails.SetString(bundle.RelationKeyName, "Task")
	typeDetails.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))
	typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"status-rel-id"})
	typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{})
	// Set unique key to allow type key extraction
	uniqueKey, _ := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, "task")
	typeDetails.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())

	statusRelDetails := domain.NewDetails()
	statusRelDetails.SetString(bundle.RelationKeyId, "status-rel-id")
	statusRelDetails.SetString(bundle.RelationKeyName, "Status")
	statusRelDetails.SetString(bundle.RelationKeyRelationKey, "status")
	statusRelDetails.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_status))
	statusRelDetails.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))

	// Use the original function (no relation options should be populated)
	schema, err := SchemaFromObjectDetails(
		typeDetails,
		[]*domain.Details{statusRelDetails},
		nil, // no resolver
	)
	require.NoError(t, err)
	require.NotNil(t, schema)

	// Check that relation exists but has no options
	statusRel, exists := schema.GetRelation("status")
	require.True(t, exists)
	assert.Equal(t, model.RelationFormat_status, statusRel.Format)
	assert.Empty(t, statusRel.Options) // No options should be populated
}