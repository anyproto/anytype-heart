package schema_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

func TestTypePropertyExport(t *testing.T) {
	t.Run("Type property has correct const value", func(t *testing.T) {
		// Create a schema with a type
		s := schema.NewSchema()
		
		// Add the type relation
		typeRel := &schema.Relation{
			Key:    "type",
			Name:   "Type",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(typeRel)
		
		// Create a type
		typ := &schema.Type{
			Key:               "task",
			Name:              "Task",
			FeaturedRelations: []string{"type"},
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
		parser := schema.NewJSONSchemaParser()
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
		exporter := schema.NewJSONSchemaExporter("  ")
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