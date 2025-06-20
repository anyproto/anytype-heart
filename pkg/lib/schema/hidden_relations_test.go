package schema_test

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
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

func TestHiddenRelationsExportImport(t *testing.T) {
	t.Run("export type with hidden relations", func(t *testing.T) {
		// Create a schema with hidden relations
		s := schema.NewSchema()
		
		// Add relations
		titleRel := &schema.Relation{
			Key:    "title",
			Name:   "Title",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(titleRel)
		
		descRel := &schema.Relation{
			Key:    "description",
			Name:   "Description", 
			Format: model.RelationFormat_longtext,
		}
		s.AddRelation(descRel)
		
		internalIdRel := &schema.Relation{
			Key:    "internal_id",
			Name:   "Internal ID",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(internalIdRel)
		
		secretRel := &schema.Relation{
			Key:    "secret_key",
			Name:   "Secret Key",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(secretRel)
		
		// Create type with hidden relations
		typ := &schema.Type{
			Key:                  "document",
			Name:                 "Document",
			FeaturedRelations:    []string{"title"},
			RecommendedRelations: []string{"description"},
			HiddenRelations:      []string{"internal_id", "secret_key"},
		}
		s.SetType(typ)
		
		// Export to JSON Schema
		exporter := schema.NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err := exporter.Export(s, &buf)
		require.NoError(t, err)
		
		// Parse exported JSON
		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)
		
		// Verify relations should not have lists at schema level
		assert.Nil(t, jsonSchema["x-featured-relations"])
		assert.Nil(t, jsonSchema["x-recommended-relations"])
		assert.Nil(t, jsonSchema["x-hidden-relations"])
		
		// Verify hidden relations have x-hidden: true
		properties := jsonSchema["properties"].(map[string]interface{})
		
		// Check internal_id property
		internalIdProp := properties["Internal ID"].(map[string]interface{})
		assert.Equal(t, true, internalIdProp["x-hidden"])
		assert.Equal(t, "internal_id", internalIdProp["x-key"])
		
		// Check secret_key property
		secretProp := properties["Secret Key"].(map[string]interface{})
		assert.Equal(t, true, secretProp["x-hidden"])
		assert.Equal(t, "secret_key", secretProp["x-key"])
		
		// Check that featured relation has x-featured
		titleProp := properties["Title"].(map[string]interface{})
		assert.Equal(t, true, titleProp["x-featured"])
		assert.Nil(t, titleProp["x-hidden"])
		
		// Check that regular relation has neither flag
		descProp := properties["Description"].(map[string]interface{})
		assert.Nil(t, descProp["x-featured"])
		assert.Nil(t, descProp["x-hidden"])
	})
	
	t.Run("import type with hidden relations", func(t *testing.T) {
		jsonStr := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id": "urn:anytype:schema:2025-01-01:test:type-document:gen-1.0",
			"type": "object",
			"title": "Document",
			"x-type-key": "document",
			"properties": {
				"id": {
					"type": "string",
					"description": "Unique identifier of the Anytype object",
					"readOnly": true,
					"x-order": 0,
					"x-key": "id"
				},
				"Title": {
					"type": "string",
					"x-key": "title",
					"x-format": "shorttext",
					"x-order": 1,
					"x-featured": true
				},
				"Description": {
					"type": "string",
					"x-key": "description",
					"x-format": "longtext",
					"x-order": 2
				},
				"Author": {
					"type": "array",
					"items": {
						"type": "string"
					},
					"x-key": "author",
					"x-format": "object",
					"x-order": 3
				},
				"Internal ID": {
					"type": "string",
					"x-key": "internal_id",
					"x-format": "shorttext",
					"x-order": 4,
					"x-hidden": true
				},
				"Secret Key": {
					"type": "string",
					"x-key": "secret_key",
					"x-format": "shorttext",
					"x-order": 5,
					"x-hidden": true
				},
				"System Flag": {
					"type": "boolean",
					"x-key": "system_flag",
					"x-format": "checkbox",
					"x-order": 6,
					"x-hidden": true
				}
			}
		}`
		
		// Parse the JSON Schema
		parser := schema.NewJSONSchemaParser()
		s, err := parser.Parse(bytes.NewReader([]byte(jsonStr)))
		require.NoError(t, err)
		
		// Get the type
		typ := s.Type
		require.NotNil(t, typ)
		
		// Verify type properties
		assert.Equal(t, "document", typ.Key)
		assert.Equal(t, "Document", typ.Name)
		
		// Verify relation lists
		assert.Equal(t, []string{"title"}, typ.FeaturedRelations)
		assert.Equal(t, []string{"description", "author"}, typ.RecommendedRelations)
		assert.Equal(t, []string{"internal_id", "secret_key", "system_flag"}, typ.HiddenRelations)
		
		// Verify all relations were parsed
		rel, ok := s.GetRelation("internal_id")
		assert.True(t, ok)
		assert.Equal(t, "Internal ID", rel.Name)
		assert.Equal(t, model.RelationFormat_shorttext, rel.Format)
		
		rel, ok = s.GetRelation("secret_key")
		assert.True(t, ok)
		assert.Equal(t, "Secret Key", rel.Name)
		
		rel, ok = s.GetRelation("system_flag")
		assert.True(t, ok)
		assert.Equal(t, "System Flag", rel.Name)
		assert.Equal(t, model.RelationFormat_checkbox, rel.Format)
	})
	
	t.Run("round-trip with hidden relations", func(t *testing.T) {
		// Create original schema
		originalSchema := schema.NewSchema()
		
		// Add various relations
		relations := []*schema.Relation{
			{Key: "name", Name: "Name", Format: model.RelationFormat_shorttext},
			{Key: "status", Name: "Status", Format: model.RelationFormat_status, Options: []string{"Active", "Inactive"}},
			{Key: "created_by", Name: "Created By", Format: model.RelationFormat_object, ObjectTypes: []string{"participant"}},
			{Key: "internal_state", Name: "Internal State", Format: model.RelationFormat_shorttext},
			{Key: "sync_id", Name: "Sync ID", Format: model.RelationFormat_shorttext},
			{Key: "debug_info", Name: "Debug Info", Format: model.RelationFormat_longtext},
		}
		
		for _, rel := range relations {
			originalSchema.AddRelation(rel)
		}
		
		// Create type with all three relation lists
		originalType := &schema.Type{
			Key:                  "system_object",
			Name:                 "System Object",
			Description:          "An object with hidden system fields",
			FeaturedRelations:    []string{"name", "status"},
			RecommendedRelations: []string{"created_by"},
			HiddenRelations:      []string{"internal_state", "sync_id", "debug_info"},
		}
		originalSchema.SetType(originalType)
		
		// Export to JSON
		exporter := schema.NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err := exporter.Export(originalSchema, &buf)
		require.NoError(t, err)
		
		// Import back
		parser := schema.NewJSONSchemaParser()
		importedSchema, err := parser.Parse(&buf)
		require.NoError(t, err)
		
		// Verify type is the same
		importedType := importedSchema.Type
		assert.Equal(t, originalType.Key, importedType.Key)
		assert.Equal(t, originalType.Name, importedType.Name)
		assert.Equal(t, originalType.Description, importedType.Description)
		assert.ElementsMatch(t, originalType.FeaturedRelations, importedType.FeaturedRelations)
		assert.ElementsMatch(t, originalType.RecommendedRelations, importedType.RecommendedRelations)
		assert.ElementsMatch(t, originalType.HiddenRelations, importedType.HiddenRelations)
		
		// Verify all relations preserved
		for _, originalRel := range relations {
			importedRel, ok := importedSchema.GetRelation(originalRel.Key)
			assert.True(t, ok, "Relation %s not found", originalRel.Key)
			assert.Equal(t, originalRel.Name, importedRel.Name)
			assert.Equal(t, originalRel.Format, importedRel.Format)
			if originalRel.Format == model.RelationFormat_status {
				assert.Equal(t, originalRel.Options, importedRel.Options)
			}
			if originalRel.Format == model.RelationFormat_object {
				assert.Equal(t, originalRel.ObjectTypes, importedRel.ObjectTypes)
			}
		}
	})
	
	t.Run("backward compatibility - x-hidden on properties", func(t *testing.T) {
		// Schema with old format using x-hidden on properties
		jsonStr := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id": "urn:anytype:schema:2025-01-01:test:type-legacy:gen-1.0",
			"type": "object",
			"title": "Legacy Type",
			"x-type-key": "legacy",
			"properties": {
				"id": {
					"type": "string",
					"description": "Unique identifier of the Anytype object",
					"readOnly": true,
					"x-order": 0,
					"x-key": "id"
				},
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-format": "shorttext",
					"x-order": 1,
					"x-featured": true
				},
				"Hidden Field": {
					"type": "string",
					"x-key": "hidden_field",
					"x-format": "shorttext",
					"x-order": 2,
					"x-hidden": true
				}
			}
		}`
		
		parser := schema.NewJSONSchemaParser()
		s, err := parser.Parse(bytes.NewReader([]byte(jsonStr)))
		require.NoError(t, err)
		
		typ := s.Type
		require.NotNil(t, typ)
		
		// Should parse x-featured and x-hidden from properties
		assert.Contains(t, typ.FeaturedRelations, "name")
		assert.Contains(t, typ.HiddenRelations, "hidden_field")
	})
	
	t.Run("type ToDetails exports hidden relations", func(t *testing.T) {
		// Create a type with hidden relations
		typ := &schema.Type{
			Key:                  "document",
			Name:                 "Document",
			Description:          "A document type",
			FeaturedRelations:    []string{"title", "status"},
			RecommendedRelations: []string{"author", "created_date"},
			HiddenRelations:      []string{"internal_id", "sync_state", "debug_flag"},
		}
		
		// Convert to details
		details := typ.ToDetails()
		
		// Verify all relation lists are in details
		featuredList := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
		assert.Equal(t, []string{"title", "status"}, featuredList)
		
		recommendedList := details.GetStringList(bundle.RelationKeyRecommendedRelations)
		assert.Equal(t, []string{"author", "created_date"}, recommendedList)
		
		hiddenList := details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations)
		assert.Equal(t, []string{"internal_id", "sync_state", "debug_flag"}, hiddenList)
	})
	
	t.Run("type FromDetails imports hidden relations", func(t *testing.T) {
		// Create details with hidden relations
		details := domain.NewDetails()
		details.SetString(bundle.RelationKeyName, "Document")
		details.SetString(bundle.RelationKeyDescription, "A document type")
		details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"title", "status"})
		details.SetStringList(bundle.RelationKeyRecommendedRelations, []string{"author", "created_date"})
		details.SetStringList(bundle.RelationKeyRecommendedHiddenRelations, []string{"internal_id", "sync_state", "debug_flag"})
		
		// Create unique key
		uniqueKey, _ := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, "document")
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
		
		// Convert from details
		typ, err := schema.TypeFromDetails(details)
		require.NoError(t, err)
		
		// Verify type properties
		assert.Equal(t, "document", typ.Key)
		assert.Equal(t, "Document", typ.Name)
		assert.Equal(t, "A document type", typ.Description)
		
		// Verify relation lists
		assert.Equal(t, []string{"title", "status"}, typ.FeaturedRelations)
		assert.Equal(t, []string{"author", "created_date"}, typ.RecommendedRelations)
		assert.Equal(t, []string{"internal_id", "sync_state", "debug_flag"}, typ.HiddenRelations)
	})
}