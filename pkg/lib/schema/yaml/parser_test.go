package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

// MockResolver implements schema.PropertyResolver for testing
type MockResolver struct {
	properties map[string]string
}

// Ensure MockResolver implements schema.PropertyResolver
var _ schema.PropertyResolver = (*MockResolver)(nil)

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

func (m *MockResolver) ResolveObjectValues(objectNames []string) []string {
	// For testing, just return the same names
	return objectNames
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

		result, err := ParseYAMLFrontMatterWithResolver(frontMatter, resolver)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Find properties by name
		propMap := make(map[string]*Property)
		for i := range result.Properties {
			prop := &result.Properties[i]
			propMap[prop.Name] = prop
		}

		// Verify known properties use resolver keys
		assert.Equal(t, "task_title", propMap["Title"].Key)
		// Status is mapped to bundle.RelationKeyStatus which takes precedence over resolver
		assert.Equal(t, "status", propMap["Status"].Key)
		assert.Equal(t, "task_priority", propMap["Priority"].Key)

		// Verify unknown property gets generated key
		unknownProp := propMap["Unknown Field"]
		assert.Len(t, unknownProp.Key, 24) // BSON ID length
		assert.Regexp(t, "^[0-9a-f]{24}$", unknownProp.Key)
	})

	t.Run("falls back to generated keys without resolver", func(t *testing.T) {
		frontMatter := []byte(`Title: Test
Author: John Doe
Date: 2024-01-15`)

		result, err := ParseYAMLFrontMatterWithResolver(frontMatter, nil)
		require.NoError(t, err)
		require.NotNil(t, result)

		// All properties should have generated BSON keys
		for _, prop := range result.Properties {
			assert.Len(t, prop.Key, 24)
			assert.Regexp(t, "^[0-9a-f]{24}$", prop.Key)
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

		result, err := ParseYAMLFrontMatterWithResolver(frontMatter, resolver)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Type should be extracted
		assert.Equal(t, "Task", result.ObjectType)

		// Should only have one property (type is excluded)
		assert.Len(t, result.Properties, 1)
		assert.Equal(t, "Name", result.Properties[0].Name)
		assert.Equal(t, "name", result.Properties[0].Key)
	})
}
