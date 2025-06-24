package schema

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONSchemaParser_XOrderAndFeatured(t *testing.T) {
	schemaJSON := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Test Type",
		"x-type-key": "test_type",
		"properties": {
			"id": {
				"type": "string",
				"x-order": 0,
				"x-key": "id"
			},
			"Object type": {
				"type": "string",
				"const": "Test Type",
				"x-order": 1,
				"x-featured": true,
				"x-key": "type"
			},
			"Name": {
				"type": "string",
				"x-featured": true,
				"x-order": 2,
				"x-key": "name"
			},
			"Status": {
				"type": "string",
				"x-featured": true,
				"x-order": 3,
				"x-key": "status"
			},
			"Description": {
				"type": "string",
				"x-order": 4,
				"x-key": "description"
			},
			"Hidden Field": {
				"type": "string",
				"x-order": 5,
				"x-hidden": true,
				"x-key": "hidden_field"
			}
		}
	}`

	parser := NewJSONSchemaParser()
	schema, err := parser.Parse(bytes.NewReader([]byte(schemaJSON)))
	assert.NoError(t, err)
	assert.NotNil(t, schema)
	assert.NotNil(t, schema.Type)

	// Check featured relations are in correct order
	assert.Equal(t, []string{"type", "name", "status"}, schema.Type.FeaturedRelations)
	
	// Check recommended relations
	assert.Equal(t, []string{"description"}, schema.Type.RecommendedRelations)
	
	// Check hidden relations
	assert.Equal(t, []string{"hidden_field"}, schema.Type.HiddenRelations)
	
	// Verify "type" is included when x-featured is true
	assert.Contains(t, schema.Type.FeaturedRelations, "type")
}

