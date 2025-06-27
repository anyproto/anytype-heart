package schema

import (
	"bytes"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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

	// Check hidden relations - should include hidden_field + system properties
	assert.Contains(t, schema.Type.HiddenRelations, "hidden_field")
	// Check that system properties were also added
	for _, sysProp := range SystemProperties {
		if slices.Contains(schema.Type.FeaturedRelations, sysProp) ||
			slices.Contains(schema.Type.RecommendedRelations, sysProp) {
			continue
		}
		assert.Contains(t, schema.Type.HiddenRelations, sysProp)
	}

	// Verify "type" is included when x-featured is true
	assert.Contains(t, schema.Type.FeaturedRelations, "type")
}

func TestJSONSchemaParser_SystemPropertiesImport(t *testing.T) {
	t.Run("adds system properties to hidden relations on import", func(t *testing.T) {
		schemaJSON := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Task",
			"x-type-key": "task",
			"properties": {
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-featured": true,
					"x-order": 1
				},
				"Status": {
					"type": "string",
					"x-key": "status",
					"x-order": 2
				}
			}
		}`

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader([]byte(schemaJSON)))
		require.NoError(t, err)
		require.NotNil(t, schema)
		require.NotNil(t, schema.Type)

		// Check that system properties are added to hidden relations
		for _, sysProp := range SystemProperties {
			assert.Contains(t, schema.Type.HiddenRelations, sysProp,
				"System property %s should be in hidden relations", sysProp)
		}

		// Should have exactly the system properties in hidden relations
		assert.Len(t, schema.Type.HiddenRelations, len(SystemProperties))
	})

	t.Run("does not duplicate system properties if already present", func(t *testing.T) {
		schemaJSON := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Article",
			"x-type-key": "article",
			"properties": {
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-featured": true,
					"x-order": 1
				},
				"Creator": {
					"type": "string",
					"x-key": "creator",
					"x-featured": true,
					"x-order": 2,
					"x-format": "object"
				},
				"Created Date": {
					"type": "string",
					"x-key": "createdDate",
					"x-order": 3,
					"x-format": "date"
				}
			}
		}`

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader([]byte(schemaJSON)))
		require.NoError(t, err)
		require.NotNil(t, schema)
		require.NotNil(t, schema.Type)

		// Creator should be in featured relations
		assert.Contains(t, schema.Type.FeaturedRelations, "creator")
		// createdDate should be in recommended relations
		assert.Contains(t, schema.Type.RecommendedRelations, "createdDate")

		// But they should NOT be duplicated in hidden relations
		for _, rel := range schema.Type.HiddenRelations {
			assert.NotEqual(t, "creator", rel)
			assert.NotEqual(t, "createdDate", rel)
		}

		// Other system properties should still be in hidden relations
		assert.Contains(t, schema.Type.HiddenRelations, bundle.RelationKeyIconEmoji.String())
		assert.Contains(t, schema.Type.HiddenRelations, bundle.RelationKeyIconImage.String())
		assert.Contains(t, schema.Type.HiddenRelations, bundle.RelationKeyCoverId.String())
	})
}
