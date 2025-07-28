package schema

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestPropertyLevelFlags(t *testing.T) {
	t.Run("export uses property-level flags not root-level lists", func(t *testing.T) {
		// Create a comprehensive schema
		s := NewSchema()

		// Add various relations
		relations := []struct {
			key    string
			name   string
			format model.RelationFormat
		}{
			{"title", "Title", model.RelationFormat_shorttext},
			{"status", "Status", model.RelationFormat_status},
			{"priority", "Priority", model.RelationFormat_number},
			{"assignee", "Assignee", model.RelationFormat_object},
			{"internal_notes", "Internal Notes", model.RelationFormat_longtext},
			{"sync_id", "Sync ID", model.RelationFormat_shorttext},
		}

		for _, r := range relations {
			rel := &Relation{
				Key:    r.key,
				Name:   r.name,
				Format: r.format,
			}
			s.AddRelation(rel)
		}

		// Create type with different relation categories
		typ := &Type{
			Key:                  "task",
			Name:                 "Task",
			Description:          "Task object",
			FeaturedRelations:    []string{"title", "status"},
			RecommendedRelations: []string{"priority", "assignee"},
			HiddenRelations:      []string{"internal_notes", "sync_id"},
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

		// Verify NO root-level relation lists
		assert.Nil(t, jsonSchema["x-featured-relations"], "Should not have x-featured-relations at root")
		assert.Nil(t, jsonSchema["x-recommended-relations"], "Should not have x-recommended-relations at root")
		assert.Nil(t, jsonSchema["x-hidden-relations"], "Should not have x-hidden-relations at root")

		// Verify property-level flags
		properties := jsonSchema["properties"].(map[string]interface{})

		// Featured relations should have x-featured: true
		titleProp := properties["Title"].(map[string]interface{})
		assert.Equal(t, true, titleProp["x-featured"], "Title should be featured")
		assert.Nil(t, titleProp["x-hidden"])

		statusProp := properties["Status"].(map[string]interface{})
		assert.Equal(t, true, statusProp["x-featured"], "Status should be featured")
		assert.Nil(t, statusProp["x-hidden"])

		// Regular relations should have neither flag
		priorityProp := properties["Priority"].(map[string]interface{})
		assert.Nil(t, priorityProp["x-featured"])
		assert.Nil(t, priorityProp["x-hidden"])

		assigneeProp := properties["Assignee"].(map[string]interface{})
		assert.Nil(t, assigneeProp["x-featured"])
		assert.Nil(t, assigneeProp["x-hidden"])

		// Hidden relations should have x-hidden: true
		internalProp := properties["Internal Notes"].(map[string]interface{})
		assert.Nil(t, internalProp["x-featured"])
		assert.Equal(t, true, internalProp["x-hidden"], "Internal Notes should be hidden")

		syncProp := properties["Sync ID"].(map[string]interface{})
		assert.Nil(t, syncProp["x-featured"])
		assert.Equal(t, true, syncProp["x-hidden"], "Sync ID should be hidden")
	})

	t.Run("import from property-level flags works correctly", func(t *testing.T) {
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
				"Content": {
					"type": "string",
					"x-key": "content",
					"x-format": "longtext",
					"x-order": 2
				},
				"Author": {
					"type": "array",
					"items": {"type": "string"},
					"x-key": "author",
					"x-format": "object",
					"x-order": 3,
					"x-featured": true
				},
				"Tags": {
					"type": "array",
					"items": {"type": "string"},
					"x-key": "tags",
					"x-format": "tag",
					"x-order": 4
				},
				"Internal ID": {
					"type": "string",
					"x-key": "internal_id",
					"x-format": "shorttext",
					"x-order": 5,
					"x-hidden": true
				},
				"Debug Info": {
					"type": "string",
					"x-key": "debug_info",
					"x-format": "longtext",
					"x-order": 6,
					"x-hidden": true
				}
			}
		}`

		// Parse the JSON Schema
		parser := NewJSONSchemaParser()
		s, err := parser.Parse(bytes.NewReader([]byte(jsonStr)))
		require.NoError(t, err)

		// Get the type
		typ := s.Type
		require.NotNil(t, typ)

		// Verify relations were categorized correctly
		assert.ElementsMatch(t, []string{"title", "author"}, typ.FeaturedRelations, "Should have featured relations from x-featured properties")
		assert.ElementsMatch(t, []string{"content", "tags"}, typ.RecommendedRelations, "Should have regular relations")
		// Check that hidden relations include specified hidden relations + system properties
		assert.Contains(t, typ.HiddenRelations, "internal_id")
		assert.Contains(t, typ.HiddenRelations, "debug_info")
		for _, sysProp := range SystemProperties {
			assert.Contains(t, typ.HiddenRelations, sysProp)
		}
	})
}

func TestJSONSchemaParser_VersionCheck(t *testing.T) {
	t.Run("accepts current version", func(t *testing.T) {
		jsonSchema := map[string]interface{}{
			"$schema":          "http://json-schema.org/draft-07/schema#",
			"type":             "object",
			"title":            "Test Type",
			"x-type-key":       "test_type",
			"x-schema-version": "1.0",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":    "string",
					"x-key":   "id",
					"x-order": 0,
				},
			},
		}

		data, err := json.Marshal(jsonSchema)
		require.NoError(t, err)

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader(data))
		require.NoError(t, err)
		require.NotNil(t, schema)
		require.NotNil(t, schema.Type)
		assert.Equal(t, "Test Type", schema.Type.Name)
	})

	t.Run("accepts older version", func(t *testing.T) {
		jsonSchema := map[string]interface{}{
			"$schema":          "http://json-schema.org/draft-07/schema#",
			"type":             "object",
			"title":            "Test Type",
			"x-type-key":       "test_type",
			"x-schema-version": "0.9",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":    "string",
					"x-key":   "id",
					"x-order": 0,
				},
			},
		}

		data, err := json.Marshal(jsonSchema)
		require.NoError(t, err)

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader(data))
		require.NoError(t, err)
		require.NotNil(t, schema)
	})

	t.Run("rejects newer major version", func(t *testing.T) {
		jsonSchema := map[string]interface{}{
			"$schema":          "http://json-schema.org/draft-07/schema#",
			"type":             "object",
			"title":            "Test Type",
			"x-type-key":       "test_type",
			"x-schema-version": "2.0",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":    "string",
					"x-key":   "id",
					"x-order": 0,
				},
			},
		}

		data, err := json.Marshal(jsonSchema)
		require.NoError(t, err)

		parser := NewJSONSchemaParser()
		_, err = parser.Parse(bytes.NewReader(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "schema version 2.0 is not compatible")
		assert.Contains(t, err.Error(), "major version is too new")
	})

	t.Run("accepts same major with higher minor version", func(t *testing.T) {
		jsonSchema := map[string]interface{}{
			"$schema":          "http://json-schema.org/draft-07/schema#",
			"type":             "object",
			"title":            "Test Type",
			"x-type-key":       "test_type",
			"x-schema-version": "1.5",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":    "string",
					"x-key":   "id",
					"x-order": 0,
				},
			},
		}

		data, err := json.Marshal(jsonSchema)
		require.NoError(t, err)

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader(data))
		require.NoError(t, err)
		require.NotNil(t, schema)
	})

	t.Run("handles missing version", func(t *testing.T) {
		jsonSchema := map[string]interface{}{
			"$schema":    "http://json-schema.org/draft-07/schema#",
			"type":       "object",
			"title":      "Test Type",
			"x-type-key": "test_type",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":    "string",
					"x-key":   "id",
					"x-order": 0,
				},
			},
		}

		data, err := json.Marshal(jsonSchema)
		require.NoError(t, err)

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader(data))
		require.NoError(t, err)
		require.NotNil(t, schema)
	})

	t.Run("handles invalid version format", func(t *testing.T) {
		jsonSchema := map[string]interface{}{
			"$schema":          "http://json-schema.org/draft-07/schema#",
			"type":             "object",
			"title":            "Test Type",
			"x-type-key":       "test_type",
			"x-schema-version": "invalid",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":    "string",
					"x-key":   "id",
					"x-order": 0,
				},
			},
		}

		data, err := json.Marshal(jsonSchema)
		require.NoError(t, err)

		parser := NewJSONSchemaParser()
		_, err = parser.Parse(bytes.NewReader(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid schema version format")
	})
}
