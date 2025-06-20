package schema

import (
	"bytes"
	"encoding/json"
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
		assert.Equal(t, 6, len(s.Relations), "Should have all 6 relations")
		
		// Verify hidden relations are included
		rel, ok := s.GetRelation("internal_id")
		assert.True(t, ok, "Hidden relation internal_id should be in schema")
		assert.Equal(t, "Internal ID", rel.Name)
		
		rel, ok = s.GetRelation("sync_state")
		assert.True(t, ok, "Hidden relation sync_state should be in schema")
		assert.Equal(t, "Sync State", rel.Name)
		
		// Verify type has hidden relations
		typ := s.Type
		// The type should have the relation IDs from the details
		assert.Contains(t, typ.HiddenRelations, "rel-internal-id")
		assert.Contains(t, typ.HiddenRelations, "rel-sync-state")
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
		assert.Equal(t, 4, len(s.Relations), "Should have all 4 relations including hidden")
		
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
		
		// The property name should be "Type" (capitalized) based on the relation name
		var typeProp map[string]interface{}
		if tp, ok := properties["Type"].(map[string]interface{}); ok {
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