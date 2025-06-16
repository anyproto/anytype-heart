package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// MockResolver implements YAMLPropertyResolver for testing
type MockResolver struct {
	properties map[string]string
}

func (m *MockResolver) ResolvePropertyKey(name string) string {
	return m.properties[name]
}

func (m *MockResolver) GetRelationFormat(key string) model.RelationFormat {
	return model.RelationFormat_shorttext
}

func (m *MockResolver) ResolveOptionValue(relationKey string, optionName string) string {
	return optionName
}

func (m *MockResolver) ResolveOptionValues(relationKey string, optionNames []string) []string {
	return optionNames
}

func TestParseYAMLFrontMatterWithResolver(t *testing.T) {
	t.Run("uses resolver keys when available", func(t *testing.T) {
		resolver := &MockResolver{
			properties: map[string]string{
				"Title":    "task_title",
				"Status":   "task_status",
				"Priority": "task_priority",
			},
		}

		frontMatter := []byte(`Title: Complete integration
Status: In Progress
Priority: 1
Unknown Field: test`)

		result, err := parseYAMLFrontMatterWithResolver(frontMatter, resolver)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Find properties by name
		propMap := make(map[string]*yamlProperty)
		for i := range result.Properties {
			prop := &result.Properties[i]
			propMap[prop.name] = prop
		}

		// Verify known properties use resolver keys
		assert.Equal(t, "task_title", propMap["Title"].key)
		assert.Equal(t, "task_status", propMap["Status"].key)
		assert.Equal(t, "task_priority", propMap["Priority"].key)

		// Verify unknown property gets generated key
		unknownProp := propMap["Unknown Field"]
		assert.Len(t, unknownProp.key, 24) // BSON ID length
		assert.Regexp(t, "^[0-9a-f]{24}$", unknownProp.key)
	})

	t.Run("falls back to generated keys without resolver", func(t *testing.T) {
		frontMatter := []byte(`Title: Test
Author: John Doe
Date: 2024-01-15`)

		result, err := parseYAMLFrontMatterWithResolver(frontMatter, nil)
		require.NoError(t, err)
		require.NotNil(t, result)

		// All properties should have generated BSON keys
		for _, prop := range result.Properties {
			assert.Len(t, prop.key, 24)
			assert.Regexp(t, "^[0-9a-f]{24}$", prop.key)
		}
	})

	t.Run("handles object type property", func(t *testing.T) {
		resolver := &MockResolver{
			properties: map[string]string{
				"Name": "name",
			},
		}

		frontMatter := []byte(`type: Task
Name: My Task`)

		result, err := parseYAMLFrontMatterWithResolver(frontMatter, resolver)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Type should be extracted
		assert.Equal(t, "Task", result.ObjectType)
		
		// Should only have one property (type is excluded)
		assert.Len(t, result.Properties, 1)
		assert.Equal(t, "Name", result.Properties[0].name)
		assert.Equal(t, "name", result.Properties[0].key)
	})
}