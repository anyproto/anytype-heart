package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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
	relationsById   map[string]*domain.Details
	relationsByKey  map[string]*domain.Details
	relationOptions map[string][]*domain.Details
}

func (m *mockObjectResolver) RelationById(relationId string) (*domain.Details, error) {
	if rel, ok := m.relationsById[relationId]; ok {
		return rel, nil
	}
	return nil, nil
}

func (m *mockObjectResolver) RelationByKey(relationKey string) (*domain.Details, error) {
	if rel, ok := m.relationsByKey[relationKey]; ok {
		return rel, nil
	}
	// Fallback: try to find by ID if key not found (for backward compatibility)
	if rel, ok := m.relationsById[relationKey]; ok {
		return rel, nil
	}
	return nil, nil
}

func (m *mockObjectResolver) RelationOptions(relationKey string) ([]*domain.Details, error) {
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
		relationsById: map[string]*domain.Details{
			"status-rel-id": statusRelDetails,
		},
		relationsByKey: map[string]*domain.Details{
			"status": statusRelDetails,
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
		relationsById: map[string]*domain.Details{
			"tags-rel-id": tagsRelDetails,
		},
		relationsByKey: map[string]*domain.Details{
			"tags": tagsRelDetails,
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

func TestSystemPropertiesInExport(t *testing.T) {
	t.Run("System properties are added to export if not in any list", func(t *testing.T) {
		// Create type with no system properties in any list
		typeDetails := domain.NewDetails()
		typeDetails.SetString(bundle.RelationKeyName, "Article")
		typeDetails.SetString(bundle.RelationKeyUniqueKey, "ot-article")
		typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"rel-title", "rel-content"})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{"rel-tags"})

		// Create relation details
		relationDetailsList := []*domain.Details{
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-title")
				d.SetString(bundle.RelationKeyRelationKey, "title")
				d.SetString(bundle.RelationKeyName, "Title")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
		}

		schema, err := SchemaFromObjectDetails(typeDetails, relationDetailsList, nil)
		require.NoError(t, err)

		// Export to JSON
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err = exporter.Export(schema, &buf)
		require.NoError(t, err)

		// Parse JSON to verify
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		properties := jsonSchema["properties"].(map[string]interface{})

		// System properties should be present
		for _, sysPropKey := range SystemProperties {
			relKey := domain.RelationKey(sysPropKey)
			bundledRel, err := bundle.GetRelation(relKey)
			require.NoError(t, err)

			// Check if property exists (may have deduplicated name)
			found := false
			for _, prop := range properties {
				if propMap, ok := prop.(map[string]interface{}); ok {
					if propMap["x-key"] == sysPropKey {
						found = true
						assert.True(t, propMap["x-hidden"].(bool), "System property %s should be hidden", sysPropKey)
						break
					}
				}
			}
			assert.True(t, found, "System property %s (%s) should be in export", sysPropKey, bundledRel.Name)
		}
	})

	t.Run("System properties already in lists are not duplicated", func(t *testing.T) {
		// Create type with creator in featured relations
		typeDetails := domain.NewDetails()
		typeDetails.SetString(bundle.RelationKeyName, "Article")
		typeDetails.SetString(bundle.RelationKeyUniqueKey, "ot-article")
		typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations,
			[]string{"rel-title", bundle.RelationKeyCreator.String()})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations,
			[]string{bundle.RelationKeyCreatedDate.String()})

		// Create a resolver that can handle bundled relations
		resolver := func(id string) (*domain.Details, error) {
			// Check if it's a bundled relation key
			if relKey, err := bundle.GetRelation(domain.RelationKey(id)); err == nil {
				details := domain.NewDetails()
				details.SetString(bundle.RelationKeyId, id)
				details.SetString(bundle.RelationKeyRelationKey, id)
				details.SetString(bundle.RelationKeyName, relKey.Name)
				details.SetInt64(bundle.RelationKeyRelationFormat, int64(relKey.Format))
				return details, nil
			}
			return nil, fmt.Errorf("relation not found")
		}

		schema, err := SchemaFromObjectDetails(typeDetails, nil, resolver)
		require.NoError(t, err)

		// Export to JSON
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err = exporter.Export(schema, &buf)
		require.NoError(t, err)

		// Parse JSON to verify
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		properties := jsonSchema["properties"].(map[string]interface{})

		// Count occurrences of each system property
		sysPropCount := make(map[string]int)
		for _, prop := range properties {
			if propMap, ok := prop.(map[string]interface{}); ok {
				if xKey, ok := propMap["x-key"].(string); ok {
					for _, sysProp := range SystemProperties {
						if xKey == sysProp {
							sysPropCount[sysProp]++
						}
					}
				}
			}
		}

		// Each system property should appear exactly once
		for _, sysProp := range SystemProperties {
			assert.Equal(t, 1, sysPropCount[sysProp], "System property %s should appear exactly once", sysProp)
		}

		// Verify creator is featured, not hidden
		for _, prop := range properties {
			if propMap, ok := prop.(map[string]interface{}); ok {
				if propMap["x-key"] == bundle.RelationKeyCreator.String() {
					// Creator should be featured since it's in the featured list
					if featured, ok := propMap["x-featured"].(bool); ok {
						assert.True(t, featured, "Creator should be featured")
					} else {
						t.Error("Creator property missing x-featured flag")
					}
					// Should not have x-hidden since it's explicitly featured
					if hidden, hasHidden := propMap["x-hidden"]; hasHidden {
						t.Errorf("Creator should not have x-hidden, but has: %v", hidden)
					}
				}
			}
		}
	})
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

// Tests for hidden relations in schema export
func TestHiddenRelationsInSchemaExport(t *testing.T) {
	t.Run("SchemaFromObjectDetails includes hidden relations", func(t *testing.T) {
		// Create type details with hidden relations
		typeDetails := domain.NewDetails()
		typeDetails.SetString(bundle.RelationKeyName, "Task")
		typeDetails.SetString(bundle.RelationKeyUniqueKey, "ot-task")
		typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"rel-title", "rel-status"})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{"rel-description", "rel-assignee"})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedHiddenRelations, []string{"rel-internal-id", "rel-sync-state"})

		// Create relation details including hidden ones
		relationDetailsList := []*domain.Details{
			// Featured relations
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-title")
				d.SetString(bundle.RelationKeyRelationKey, "title")
				d.SetString(bundle.RelationKeyName, "Title")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-status")
				d.SetString(bundle.RelationKeyRelationKey, "status")
				d.SetString(bundle.RelationKeyName, "Status")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_status))
				return d
			}(),
			// Regular relations
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-description")
				d.SetString(bundle.RelationKeyRelationKey, "description")
				d.SetString(bundle.RelationKeyName, "Description")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_longtext))
				return d
			}(),
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-assignee")
				d.SetString(bundle.RelationKeyRelationKey, "assignee")
				d.SetString(bundle.RelationKeyName, "Assignee")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_object))
				return d
			}(),
			// Hidden relations
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-internal-id")
				d.SetString(bundle.RelationKeyRelationKey, "internal_id")
				d.SetString(bundle.RelationKeyName, "Internal ID")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-sync-state")
				d.SetString(bundle.RelationKeyRelationKey, "sync_state")
				d.SetString(bundle.RelationKeyName, "Sync State")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
		}

		// Create schema from details
		s, err := SchemaFromObjectDetails(typeDetails, relationDetailsList, nil)
		require.NoError(t, err)

		// Verify all relations are in the schema
		// 6 original relations + 6 system properties (type, creator, createdDate, iconEmoji, iconImage, coverId)
		assert.Equal(t, 12, len(s.Relations), "Should have all 6 relations plus 5 system properties")

		// Verify hidden relations are included
		rel, ok := s.GetRelation("internal_id")
		assert.True(t, ok, "Hidden relation internal_id should be in schema")
		assert.Equal(t, "Internal ID", rel.Name)

		rel, ok = s.GetRelation("sync_state")
		assert.True(t, ok, "Hidden relation sync_state should be in schema")
		assert.Equal(t, "Sync State", rel.Name)

		// Verify type has hidden relations
		typ := s.Type
		// The type should have the relation keys (resolved from IDs)
		assert.Contains(t, typ.HiddenRelations, "internal_id")
		assert.Contains(t, typ.HiddenRelations, "sync_state")

		// Verify system properties were added to hidden relations
		assert.Contains(t, typ.HiddenRelations, bundle.RelationKeyCreator.String())
		assert.Contains(t, typ.HiddenRelations, bundle.RelationKeyCreatedDate.String())
		assert.Contains(t, typ.HiddenRelations, bundle.RelationKeyIconEmoji.String())
		assert.Contains(t, typ.HiddenRelations, bundle.RelationKeyIconImage.String())
		assert.Contains(t, typ.HiddenRelations, bundle.RelationKeyCoverId.String())

		// Verify system properties are in the schema
		_, ok = s.GetRelation(bundle.RelationKeyCreator.String())
		assert.True(t, ok, "System relation creator should be in schema")

		_, ok = s.GetRelation(bundle.RelationKeyCreatedDate.String())
		assert.True(t, ok, "System relation createdDate should be in schema")
	})

	t.Run("Hidden relations with resolver", func(t *testing.T) {
		// Create type details with hidden relations
		typeDetails := domain.NewDetails()
		typeDetails.SetString(bundle.RelationKeyName, "Document")
		typeDetails.SetString(bundle.RelationKeyUniqueKey, "ot-document")
		typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"rel-title"})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{"rel-content"})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedHiddenRelations, []string{"rel-version", "rel-checksum"})

		// Only provide featured and regular relations
		relationDetailsList := []*domain.Details{
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-title")
				d.SetString(bundle.RelationKeyRelationKey, "title")
				d.SetString(bundle.RelationKeyName, "Title")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-content")
				d.SetString(bundle.RelationKeyRelationKey, "content")
				d.SetString(bundle.RelationKeyName, "Content")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_longtext))
				return d
			}(),
		}

		// Create resolver that provides hidden relations
		resolver := func(id string) (*domain.Details, error) {
			switch id {
			case "rel-version":
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-version")
				d.SetString(bundle.RelationKeyRelationKey, "version")
				d.SetString(bundle.RelationKeyName, "Version")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_number))
				return d, nil
			case "rel-checksum":
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-checksum")
				d.SetString(bundle.RelationKeyRelationKey, "checksum")
				d.SetString(bundle.RelationKeyName, "Checksum")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d, nil
			}
			return nil, nil
		}

		// Create schema with resolver
		s, err := SchemaFromObjectDetails(typeDetails, relationDetailsList, resolver)
		require.NoError(t, err)

		// Verify all relations including hidden are in the schema
		// 4 original relations + 6 system properties (type, creator, createdDate, iconEmoji, iconImage, coverId)
		assert.Equal(t, 10, len(s.Relations), "Should have all 4 relations plus 5 system properties")

		// Verify hidden relations were resolved and included
		rel, ok := s.GetRelation("version")
		assert.True(t, ok, "Hidden relation version should be in schema")
		assert.Equal(t, "Version", rel.Name)
		assert.Equal(t, model.RelationFormat_number, rel.Format)

		rel, ok = s.GetRelation("checksum")
		assert.True(t, ok, "Hidden relation checksum should be in schema")
		assert.Equal(t, "Checksum", rel.Name)
		assert.Equal(t, model.RelationFormat_shorttext, rel.Format)
	})
}

func TestSystemPropertiesInExportBehavior(t *testing.T) {
	t.Run("System properties are added when not in any list", func(t *testing.T) {
		// Create type details without system properties in any list
		typeDetails := domain.NewDetails()
		typeDetails.SetString(bundle.RelationKeyName, "Document")
		typeDetails.SetString(bundle.RelationKeyUniqueKey, "ot-document")
		typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"rel-title"})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{"rel-content"})

		// Create only non-system relations
		relationDetailsList := []*domain.Details{
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-title")
				d.SetString(bundle.RelationKeyRelationKey, "title")
				d.SetString(bundle.RelationKeyName, "Title")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-content")
				d.SetString(bundle.RelationKeyRelationKey, "content")
				d.SetString(bundle.RelationKeyName, "Content")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_longtext))
				return d
			}(),
		}

		// Create schema
		s, err := SchemaFromObjectDetails(typeDetails, relationDetailsList, nil)
		require.NoError(t, err)

		// Export to JSON Schema
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err = exporter.Export(s, &buf)
		require.NoError(t, err)

		// Parse exported JSON
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		// Verify properties include system properties as hidden
		properties := jsonSchema["properties"].(map[string]interface{})

		// Check system properties are present and hidden
		creatorProp, ok := properties["Created by"].(map[string]interface{})
		require.True(t, ok, "Created by property should exist")
		assert.Equal(t, true, creatorProp["x-hidden"])
		assert.Equal(t, bundle.RelationKeyCreator.String(), creatorProp["x-key"])

		createdDateProp, ok := properties["Creation date"].(map[string]interface{})
		require.True(t, ok, "Creation date property should exist")
		assert.Equal(t, true, createdDateProp["x-hidden"])
		assert.Equal(t, bundle.RelationKeyCreatedDate.String(), createdDateProp["x-key"])

		iconEmojiProp, ok := properties["Emoji"].(map[string]interface{})
		require.True(t, ok, "Emoji property should exist")
		assert.Equal(t, true, iconEmojiProp["x-hidden"])
		assert.Equal(t, bundle.RelationKeyIconEmoji.String(), iconEmojiProp["x-key"])

		iconImageProp, ok := properties["Image"].(map[string]interface{})
		require.True(t, ok, "Image property should exist")
		assert.Equal(t, true, iconImageProp["x-hidden"])
		assert.Equal(t, bundle.RelationKeyIconImage.String(), iconImageProp["x-key"])

		coverIdProp, ok := properties["Cover image or color"].(map[string]interface{})
		require.True(t, ok, "Cover image or color property should exist")
		assert.Equal(t, true, coverIdProp["x-hidden"])
		assert.Equal(t, bundle.RelationKeyCoverId.String(), coverIdProp["x-key"])
	})

	t.Run("System properties retain their flags when already in lists", func(t *testing.T) {
		// Create type details with system properties in featured and recommended lists
		typeDetails := domain.NewDetails()
		typeDetails.SetString(bundle.RelationKeyName, "Person")
		typeDetails.SetString(bundle.RelationKeyUniqueKey, "ot-person")
		typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{
			"rel-name",
			bundle.RelationKeyCreator.String(), // System property as featured
		})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{
			"rel-email",
			bundle.RelationKeyCreatedDate.String(), // System property as recommended
		})

		// Create relations including the system properties
		relationDetailsList := []*domain.Details{
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-name")
				d.SetString(bundle.RelationKeyRelationKey, "name")
				d.SetString(bundle.RelationKeyName, "Name")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-email")
				d.SetString(bundle.RelationKeyRelationKey, "email")
				d.SetString(bundle.RelationKeyName, "Email")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_email))
				return d
			}(),
		}

		// Create resolver for system properties
		resolver := func(id string) (*domain.Details, error) {
			switch id {
			case bundle.RelationKeyCreator.String():
				// Get bundled relation details
				rel, _ := bundle.GetRelation(bundle.RelationKeyCreator)
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, bundle.RelationKeyCreator.String())
				d.SetString(bundle.RelationKeyRelationKey, bundle.RelationKeyCreator.String())
				d.SetString(bundle.RelationKeyName, rel.Name)
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(rel.Format))
				return d, nil
			case bundle.RelationKeyCreatedDate.String():
				// Get bundled relation details
				rel, _ := bundle.GetRelation(bundle.RelationKeyCreatedDate)
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, bundle.RelationKeyCreatedDate.String())
				d.SetString(bundle.RelationKeyRelationKey, bundle.RelationKeyCreatedDate.String())
				d.SetString(bundle.RelationKeyName, rel.Name)
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(rel.Format))
				return d, nil
			}
			return nil, nil
		}

		// Create schema
		s, err := SchemaFromObjectDetails(typeDetails, relationDetailsList, resolver)
		require.NoError(t, err)

		// Export to JSON Schema
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err = exporter.Export(s, &buf)
		require.NoError(t, err)

		// Parse exported JSON
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		// Verify properties
		properties := jsonSchema["properties"].(map[string]interface{})

		// Creator should be featured (not hidden)
		creatorProp, ok := properties["Created by"].(map[string]interface{})
		require.True(t, ok, "Created by property should exist")
		assert.Equal(t, true, creatorProp["x-featured"])
		assert.Nil(t, creatorProp["x-hidden"], "Created by should not be hidden when featured")
		assert.Equal(t, bundle.RelationKeyCreator.String(), creatorProp["x-key"])

		// Created date should be regular (neither featured nor hidden)
		createdDateProp, ok := properties["Creation date"].(map[string]interface{})
		require.True(t, ok, "Creation date property should exist")
		assert.Nil(t, createdDateProp["x-featured"], "Creation date should not be featured")
		assert.Nil(t, createdDateProp["x-hidden"], "Creation date should not be hidden when in recommended")
		assert.Equal(t, bundle.RelationKeyCreatedDate.String(), createdDateProp["x-key"])

		// Other system properties not in any list should be hidden
		iconEmojiProp, ok := properties["Emoji"].(map[string]interface{})
		require.True(t, ok, "Emoji property should exist")
		assert.Equal(t, true, iconEmojiProp["x-hidden"])

		iconImageProp, ok := properties["Image"].(map[string]interface{})
		require.True(t, ok, "Image property should exist")
		assert.Equal(t, true, iconImageProp["x-hidden"])

		coverIdProp, ok := properties["Cover image or color"].(map[string]interface{})
		require.True(t, ok, "Cover image or color property should exist")
		assert.Equal(t, true, coverIdProp["x-hidden"])
	})
}

func TestFullExportWithHiddenRelations(t *testing.T) {
	t.Run("Full export workflow includes hidden relations", func(t *testing.T) {
		// Create type details with hidden relations
		typeDetails := domain.NewDetails()
		typeDetails.SetString(bundle.RelationKeyName, "Document")
		typeDetails.SetString(bundle.RelationKeyUniqueKey, "ot-document")
		typeDetails.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"rel-title"})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedRelations, []string{"rel-content"})
		typeDetails.SetStringList(bundle.RelationKeyRecommendedHiddenRelations, []string{"rel-internal-id", "rel-version"})

		// Create relation details
		relationDetailsList := []*domain.Details{
			// Featured relation
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-title")
				d.SetString(bundle.RelationKeyRelationKey, "title")
				d.SetString(bundle.RelationKeyName, "Title")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
			// Regular relation
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-content")
				d.SetString(bundle.RelationKeyRelationKey, "content")
				d.SetString(bundle.RelationKeyName, "Content")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_longtext))
				return d
			}(),
			// Hidden relations
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-internal-id")
				d.SetString(bundle.RelationKeyRelationKey, "internal_id")
				d.SetString(bundle.RelationKeyName, "Internal ID")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_shorttext))
				return d
			}(),
			func() *domain.Details {
				d := domain.NewDetails()
				d.SetString(bundle.RelationKeyId, "rel-version")
				d.SetString(bundle.RelationKeyRelationKey, "version")
				d.SetString(bundle.RelationKeyName, "Version")
				d.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_number))
				return d
			}(),
		}

		// Create schema from details (this simulates what happens in the app)
		s, err := SchemaFromObjectDetails(typeDetails, relationDetailsList, nil)
		require.NoError(t, err)

		// Export to JSON Schema
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err = exporter.Export(s, &buf)
		require.NoError(t, err)

		// Parse exported JSON
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		// Verify properties include hidden relations
		properties := jsonSchema["properties"].(map[string]interface{})

		// Featured relation
		titleProp, ok := properties["Title"].(map[string]interface{})
		require.True(t, ok, "Title property should exist")
		assert.Equal(t, true, titleProp["x-featured"])
		assert.Equal(t, "title", titleProp["x-key"])

		// Regular relation
		contentProp, ok := properties["Content"].(map[string]interface{})
		require.True(t, ok, "Content property should exist")
		assert.Nil(t, contentProp["x-featured"])
		assert.Nil(t, contentProp["x-hidden"])
		assert.Equal(t, "content", contentProp["x-key"])

		// Hidden relations should be included with x-hidden: true
		internalIdProp, ok := properties["Internal ID"].(map[string]interface{})
		require.True(t, ok, "Internal ID property should exist")
		assert.Equal(t, true, internalIdProp["x-hidden"])
		assert.Equal(t, "internal_id", internalIdProp["x-key"])

		versionProp, ok := properties["Version"].(map[string]interface{})
		require.True(t, ok, "Version property should exist")
		assert.Equal(t, true, versionProp["x-hidden"])
		assert.Equal(t, "version", versionProp["x-key"])
		assert.Equal(t, "number", versionProp["x-format"])

		// Verify no root-level relation lists
		assert.Nil(t, jsonSchema["x-featured-relations"])
		assert.Nil(t, jsonSchema["x-recommended-relations"])
		assert.Nil(t, jsonSchema["x-hidden-relations"])
	})
}

func TestTypePropertyExport(t *testing.T) {
	t.Run("Type property has correct const value", func(t *testing.T) {
		// Create a schema with a type
		s := NewSchema()

		// Add the type relation
		typeRel := &Relation{
			Key:    "type",
			Name:   "Type",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(typeRel)

		// Create a type
		typ := &Type{
			Key:               "task",
			Name:              "Task",
			FeaturedRelations: []string{"type"},
		}
		s.SetType(typ)

		// Export to JSON Schema
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err := exporter.Export(s, &buf)
		require.NoError(t, err)

		// Parse exported JSON
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		// Check properties
		properties, ok := jsonSchema["properties"].(map[string]interface{})
		require.True(t, ok)

		// Check Type property
		typeProp, ok := properties["Type"].(map[string]interface{})
		require.True(t, ok)

		// Verify const value is the type name
		assert.Equal(t, "Task", typeProp["const"])
		assert.Equal(t, "type", typeProp["x-key"])
	})

	t.Run("Object type property in testdata schemas", func(t *testing.T) {
		// Load and check task schema
		parser := NewJSONSchemaParser()
		taskFile := bytes.NewReader([]byte(`{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id": "urn:anytype:schema:2024-06-14:author-user:type-task:gen-1.0",
			"type": "object",
			"title": "Task",
			"x-type-key": "task",
			"properties": {
				"id": {
					"type": "string",
					"description": "Unique identifier of the Anytype object",
					"readOnly": true,
					"x-order": 0,
					"x-key": "id"
				},
				"type": {
					"const": "Task",
					"x-order": 1,
					"x-key": "type"
				}
			}
		}`))

		s, err := parser.Parse(taskFile)
		require.NoError(t, err)

		// Export back
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err = exporter.Export(s, &buf)
		require.NoError(t, err)

		// Parse and verify
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		properties := jsonSchema["properties"].(map[string]interface{})

		// The property name should be "Object type" based on the relation name
		var typeProp map[string]interface{}
		if tp, ok := properties["Object type"].(map[string]interface{}); ok {
			typeProp = tp
		} else if tp, ok := properties["Type"].(map[string]interface{}); ok {
			typeProp = tp
		} else if tp, ok := properties["type"].(map[string]interface{}); ok {
			typeProp = tp
		} else {
			t.Fatal("Type property not found")
		}

		// Should preserve the const value
		assert.Equal(t, "Task", typeProp["const"])
	})
}

func TestJSONSchemaPropertyOrdering(t *testing.T) {
	t.Run("Properties are ordered by x-order", func(t *testing.T) {
		// Create a schema with multiple properties in random order
		s := NewSchema()

		// Add relations in non-sequential order
		rel3 := &Relation{
			Key:    "status",
			Name:   "Status",
			Format: model.RelationFormat_status,
		}
		s.AddRelation(rel3)

		rel1 := &Relation{
			Key:    "title",
			Name:   "Title",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel1)

		rel2 := &Relation{
			Key:    "description",
			Name:   "Description",
			Format: model.RelationFormat_longtext,
		}
		s.AddRelation(rel2)

		// Create type with relations in specific order
		typ := &Type{
			Key:                  "task",
			Name:                 "Task",
			FeaturedRelations:    []string{"title", "status"}, // Order: title=2, status=3 (after id=0 and Type=1)
			RecommendedRelations: []string{"description"},     // Order: description=4
		}
		s.SetType(typ)

		// Export to JSON Schema
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err := exporter.Export(s, &buf)
		require.NoError(t, err)

		output := buf.String()

		// Find positions of properties in the output
		titlePos := strings.Index(output, `"Title":`)
		statusPos := strings.Index(output, `"Status":`)
		descPos := strings.Index(output, `"Description":`)

		// Verify properties appear in the correct order
		assert.True(t, titlePos > 0, "Title property should be present")
		assert.True(t, statusPos > 0, "Status property should be present")
		assert.True(t, descPos > 0, "Description property should be present")

		// Title (x-order: 1) should come before Status (x-order: 2)
		assert.True(t, titlePos < statusPos, "Title should appear before Status")
		// Status (x-order: 2) should come before Description (x-order: 3)
		assert.True(t, statusPos < descPos, "Status should appear before Description")

		// Also verify the schema structure has correct ordering
		assert.Contains(t, output, `"$schema":`)
		assert.Contains(t, output, `"$id":`)
		assert.Contains(t, output, `"type": "object"`)
		assert.Contains(t, output, `"title": "Task"`)

		// Verify $schema comes first
		schemaPos := strings.Index(output, `"$schema":`)
		idPos := strings.Index(output, `"$id":`)
		typePos := strings.Index(output, `"type":`)
		titleTypePos := strings.Index(output, `"title":`)
		propsPos := strings.Index(output, `"properties":`)

		// Standard fields should come in order
		assert.True(t, schemaPos < idPos, "$schema should come before $id")
		assert.True(t, idPos < typePos, "$id should come before type")
		assert.True(t, typePos < titleTypePos, "type should come before title")
		assert.True(t, titleTypePos < propsPos, "title should come before properties")
	})

	t.Run("Properties without x-order come after ordered ones", func(t *testing.T) {
		// Test that hidden relations (which get higher x-order values) come after featured/recommended
		s := NewSchema()

		// Add relations
		rel1 := &Relation{
			Key:    "custom_field",
			Name:   "Custom Field",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel1)

		rel2 := &Relation{
			Key:    "name",
			Name:   "Name",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel2)

		rel3 := &Relation{
			Key:    "hidden_field",
			Name:   "Hidden Field",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel3)

		typ := &Type{
			Key:                  "custom",
			Name:                 "Custom Type",
			FeaturedRelations:    []string{"name"},         // x-order: 2
			RecommendedRelations: []string{"custom_field"}, // x-order: 3
			HiddenRelations:      []string{"hidden_field"}, // x-order: 4
		}
		s.SetType(typ)

		// Export
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err := exporter.Export(s, &buf)
		require.NoError(t, err)

		output := buf.String()

		// All properties should be present
		assert.Contains(t, output, `"Name":`)
		assert.Contains(t, output, `"Custom Field":`)
		assert.Contains(t, output, `"Hidden Field":`)

		// Find positions
		idPos := strings.Index(output, `"id":`)
		namePos := strings.Index(output, `"Name":`)
		customPos := strings.Index(output, `"Custom Field":`)
		hiddenPos := strings.Index(output, `"Hidden Field":`)

		// Verify order: id (0) < Name (2) < Custom Field (3) < Hidden Field (4)
		assert.True(t, idPos < namePos, "id should come before Name")
		assert.True(t, namePos < customPos, "Name should come before Custom Field")
		assert.True(t, customPos < hiddenPos, "Custom Field should come before Hidden Field")

		// Also check that x-order values are correct in the output
		assert.Contains(t, output, `"x-order": 0`) // id
		assert.Contains(t, output, `"x-order": 2`) // name
		assert.Contains(t, output, `"x-order": 3`) // custom_field
		assert.Contains(t, output, `"x-order": 4`) // hidden_field
	})
}

func TestPropertyNameDeduplication(t *testing.T) {
	t.Run("JSON Schema export deduplicates property names", func(t *testing.T) {
		// Create a schema with duplicate property names
		s := NewSchema()

		// Add relations with duplicate names but different keys
		rel1 := &Relation{
			Key:    "user_name",
			Name:   "Name", // Same name
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel1)

		rel2 := &Relation{
			Key:    "company_name",
			Name:   "Name", // Same name
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel2)

		rel3 := &Relation{
			Key:    "project_name",
			Name:   "Name", // Same name
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel3)

		rel4 := &Relation{
			Key:    "description",
			Name:   "Description", // Unique name
			Format: model.RelationFormat_longtext,
		}
		s.AddRelation(rel4)

		// Create type with these relations
		typ := &Type{
			Key:                  "entity",
			Name:                 "Entity",
			FeaturedRelations:    []string{"user_name"},
			RecommendedRelations: []string{"company_name", "description"},
			HiddenRelations:      []string{"project_name"},
		}
		s.SetType(typ)

		// Export to JSON Schema
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err := exporter.Export(s, &buf)
		require.NoError(t, err)

		// Parse exported JSON
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		// Check properties
		properties := jsonSchema["properties"].(map[string]interface{})

		// Should have deduplicated names (sorted by key: company_name, project_name, user_name)
		// Expected names: "Name" (first), "Name 2" (second), "Name 3" (third)
		nameProp, ok := properties["Name"].(map[string]interface{})
		require.True(t, ok, "Should have 'Name' property")
		assert.Equal(t, "company_name", nameProp["x-key"], "First 'Name' should be company_name (alphabetically first)")

		name2Prop, ok := properties["Name 2"].(map[string]interface{})
		require.True(t, ok, "Should have 'Name 2' property")
		assert.Equal(t, "project_name", name2Prop["x-key"], "Second 'Name' should be project_name")

		name3Prop, ok := properties["Name 3"].(map[string]interface{})
		require.True(t, ok, "Should have 'Name 3' property")
		assert.Equal(t, "user_name", name3Prop["x-key"], "Third 'Name' should be user_name")

		// Description should keep original name
		descProp, ok := properties["Description"].(map[string]interface{})
		require.True(t, ok, "Should have 'Description' property")
		assert.Equal(t, "description", descProp["x-key"])

		// Verify correct flags are preserved
		assert.Equal(t, true, name3Prop["x-featured"], "user_name should be featured")
		assert.Equal(t, true, name2Prop["x-hidden"], "project_name should be hidden")
		assert.Nil(t, nameProp["x-featured"], "company_name should not be featured")
		assert.Nil(t, nameProp["x-hidden"], "company_name should not be hidden")
	})

	t.Run("Type property always keeps its name without suffix", func(t *testing.T) {
		// Test that system Type property always remains "Type" even with conflicts
		s := NewSchema()

		// Add the system type relation
		typeRel := &Relation{
			Key:    "type",
			Name:   "Type",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(typeRel)

		// Add relations that have "Type" as their display name
		rel1 := &Relation{
			Key:    "custom_type",
			Name:   "Type", // Same as system Type property
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel1)

		rel2 := &Relation{
			Key:    "another_type",
			Name:   "Type", // Same as system Type property
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel2)

		// Create type
		typ := &Type{
			Key:               "document",
			Name:              "Document",
			FeaturedRelations: []string{"type", "custom_type", "another_type"}, // Include system type relation
		}
		s.SetType(typ)

		// Export to JSON Schema
		exporter := NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err := exporter.Export(s, &buf)
		require.NoError(t, err)

		// Parse exported JSON
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		// Check properties
		properties := jsonSchema["properties"].(map[string]interface{})

		// System Type property should always be "Type" without suffix
		typeProp, ok := properties["Type"].(map[string]interface{})
		require.True(t, ok, "Should have 'Type' property without suffix")
		assert.Equal(t, "type", typeProp["x-key"], "Type property should be system type")
		assert.Equal(t, "Document", typeProp["const"], "Type value should be the type name")

		// Other relations with "Type" name should get suffixes
		type2Prop, ok := properties["Type 2"].(map[string]interface{})
		require.True(t, ok, "Should have 'Type 2' property")
		assert.Equal(t, "another_type", type2Prop["x-key"], "Type 2 should be another_type (alphabetically first)")

		type3Prop, ok := properties["Type 3"].(map[string]interface{})
		require.True(t, ok, "Should have 'Type 3' property")
		assert.Equal(t, "custom_type", type3Prop["x-key"], "Type 3 should be custom_type")
	})
}
