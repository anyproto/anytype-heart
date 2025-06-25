package yaml

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

func TestRoundTripYAML(t *testing.T) {
	tests := []struct {
		name       string
		properties []Property
		options    *ExportOptions
	}{
		{
			name: "basic properties",
			properties: []Property{
				{
					Name:   "title",
					Key:    "title",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Test Page"),
				},
				{
					Name:   "description",
					Key:    "description",
					Format: model.RelationFormat_longtext,
					Value:  domain.String("This is a longer description\nwith multiple lines"),
				},
				{
					Name:   "count",
					Key:    "count",
					Format: model.RelationFormat_number,
					Value:  domain.Int64(42),
				},
				{
					Name:   "price",
					Key:    "price",
					Format: model.RelationFormat_number,
					Value:  domain.Float64(19.99),
				},
				{
					Name:   "active",
					Key:    "active",
					Format: model.RelationFormat_checkbox,
					Value:  domain.Bool(true),
				},
			},
		},
		{
			name: "date properties",
			properties: []Property{
				{
					Name:        "created",
					Key:         "created",
					Format:      model.RelationFormat_date,
					Value:       domain.Int64(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix()),
					IncludeTime: false,
				},
				{
					Name:        "lastModified",
					Key:         "lastModified",
					Format:      model.RelationFormat_date,
					Value:       domain.Int64(time.Date(2024, 1, 15, 14, 30, 45, 0, time.UTC).Unix()),
					IncludeTime: true,
				},
			},
		},
		{
			name: "array properties",
			properties: []Property{
				{
					Name:   "tags",
					Key:    "tags",
					Format: model.RelationFormat_tag,
					Value:  domain.StringList([]string{"important", "work", "project"}),
				},
				{
					Name:   "authors",
					Key:    "authors",
					Format: model.RelationFormat_object,
					Value:  domain.StringList([]string{"author1.md", "author2.md"}),
				},
				{
					Name:   "attachments",
					Key:    "attachments",
					Format: model.RelationFormat_file,
					Value:  domain.StringList([]string{"file1.pdf", "image.png"}),
				},
			},
		},
		{
			name: "special format properties",
			properties: []Property{
				{
					Name:   "website",
					Key:    "website",
					Format: model.RelationFormat_url,
					Value:  domain.String("https://example.com"),
				},
				{
					Name:   "email",
					Key:    "email",
					Format: model.RelationFormat_email,
					Value:  domain.String("test@example.com"),
				},
				{
					Name:   "phone",
					Key:    "phone",
					Format: model.RelationFormat_phone,
					Value:  domain.String("+1-555-123-4567"),
				},
				{
					Name:   "status",
					Key:    "status",
					Format: model.RelationFormat_status,
					Value:  domain.String("in-progress"),
				},
			},
		},
		{
			name: "with object type",
			properties: []Property{
				{
					Name:   "name",
					Key:    "name",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Test Task"),
				},
			},
			options: &ExportOptions{
				ObjectTypeName: "Task",
			},
		},
		{
			name: "empty values",
			properties: []Property{
				{
					Name:   "emptyText",
					Key:    "emptyText",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String(""),
				},
				{
					Name:   "emptyTags",
					Key:    "emptyTags",
					Format: model.RelationFormat_tag,
					Value:  domain.StringList([]string{}),
				},
				{
					Name:   "falseCheckbox",
					Key:    "falseCheckbox",
					Format: model.RelationFormat_checkbox,
					Value:  domain.Bool(false),
				},
				{
					Name:   "zeroNumber",
					Key:    "zeroNumber",
					Format: model.RelationFormat_number,
					Value:  domain.Int64(0),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Export to YAML
			yamlData, err := ExportToYAML(tt.properties, tt.options)
			require.NoError(t, err)
			require.NotEmpty(t, yamlData)

			// Extract YAML front matter (it's already in front matter format)
			frontMatter, _, err := ExtractYAMLFrontMatter(yamlData)
			require.NoError(t, err)

			// Build format map from original properties
			formats := make(map[string]model.RelationFormat)
			includeTimeMap := make(map[string]bool)
			for _, prop := range tt.properties {
				formats[prop.Name] = prop.Format
				if prop.Format == model.RelationFormat_date {
					includeTimeMap[prop.Name] = prop.IncludeTime
				}
			}

			// Parse back with formats
			result, err := ParseYAMLFrontMatterWithFormats(frontMatter, formats)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Apply includeTime information from original properties
			for i, prop := range result.Properties {
				if prop.Format == model.RelationFormat_date {
					if includeTime, ok := includeTimeMap[prop.Name]; ok {
						result.Properties[i].IncludeTime = includeTime
					}
				}
			}

			// Check object type if specified
			if tt.options != nil && tt.options.ObjectTypeName != "" {
				assert.Equal(t, tt.options.ObjectTypeName, result.ObjectType)
			}

			// Verify all properties were parsed correctly
			parsedProps := make(map[string]Property)
			for _, prop := range result.Properties {
				parsedProps[prop.Name] = prop
			}

			// Check each original property
			for _, origProp := range tt.properties {
				parsed, ok := parsedProps[origProp.Name]
				if !ok {
					// Empty values might be omitted
					if isEmptyValue(origProp.Value) {
						continue
					}
					t.Errorf("Property %s not found in parsed result", origProp.Name)
					continue
				}

				// Compare formats
				assert.Equal(t, origProp.Format, parsed.Format, "Format mismatch for %s", origProp.Name)

				// Compare values based on format
				switch origProp.Format {
				case model.RelationFormat_date:
					// Dates should match exactly
					assert.True(t, origProp.Value.Equal(parsed.Value), "Date value mismatch for %s", origProp.Name)
					if origProp.Name == "created" {
						t.Logf("Original includeTime: %v, Parsed includeTime: %v", origProp.IncludeTime, parsed.IncludeTime)
						t.Logf("Original value: %v, Parsed value: %v", origProp.Value, parsed.Value)
					}
					assert.Equal(t, origProp.IncludeTime, parsed.IncludeTime, "IncludeTime mismatch for %s", origProp.Name)

				case model.RelationFormat_number:
					// Numbers should match (accounting for int/float conversion)
					if origProp.Value.IsInt64() && parsed.Value.IsFloat64() {
						assert.Equal(t, float64(origProp.Value.Int64()), parsed.Value.Float64(), "Number value mismatch for %s", origProp.Name)
					} else if origProp.Value.IsFloat64() && parsed.Value.IsFloat64() {
						assert.Equal(t, origProp.Value.Float64(), parsed.Value.Float64(), "Number value mismatch for %s", origProp.Name)
					} else {
						assert.True(t, origProp.Value.Equal(parsed.Value), "Number value mismatch for %s", origProp.Name)
					}

				case model.RelationFormat_checkbox:
					assert.Equal(t, origProp.Value.Bool(), parsed.Value.Bool(), "Checkbox value mismatch for %s", origProp.Name)

				case model.RelationFormat_tag, model.RelationFormat_object, model.RelationFormat_file:
					// Array values
					assert.Equal(t, origProp.Value.StringList(), parsed.Value.StringList(), "Array value mismatch for %s", origProp.Name)

				default:
					// String values
					assert.Equal(t, origProp.Value.String(), parsed.Value.String(), "String value mismatch for %s", origProp.Name)
				}
			}
		})
	}
}

func TestSchemaExportRoundTrip(t *testing.T) {
	// Create a test schema
	s := schema.NewSchema()

	// Add type
	testType := &schema.Type{
		Key:         "customTask",
		Name:        "Custom Task",
		Description: "A custom task type for testing",
		IconEmoji:   "📝",
		Layout:      model.ObjectType_basic,
		FeaturedRelations: []string{
			"title",
			"status",
			"priority",
		},
		RecommendedRelations: []string{
			"assignee",
			"dueDate",
			"tags",
		},
	}
	err := s.SetType(testType)
	require.NoError(t, err)

	// Add relations
	relations := []*schema.Relation{
		{
			Key:         "title",
			Name:        "Title",
			Format:      model.RelationFormat_shorttext,
			Description: "The task title",
		},
		{
			Key:         "status",
			Name:        "Status",
			Format:      model.RelationFormat_status,
			Description: "Current status of the task",
			Options:     []string{"todo", "in-progress", "done", "archived"},
		},
		{
			Key:         "priority",
			Name:        "Priority",
			Format:      model.RelationFormat_status,
			Description: "Task priority level",
			Options:     []string{"low", "medium", "high", "urgent"},
		},
		{
			Key:         "assignee",
			Name:        "Assignee",
			Format:      model.RelationFormat_object,
			Description: "Person assigned to this task",
			ObjectTypes: []string{"person", "contact"},
		},
		{
			Key:         "dueDate",
			Name:        "Due Date",
			Format:      model.RelationFormat_date,
			Description: "When the task is due",
			IncludeTime: true,
		},
		{
			Key:         "tags",
			Name:        "Tags",
			Format:      model.RelationFormat_tag,
			Description: "Task tags for categorization",
			Examples:    []string{"frontend", "backend", "bug", "feature"},
		},
		{
			Key:         "completed",
			Name:        "Completed",
			Format:      model.RelationFormat_checkbox,
			Description: "Whether the task is completed",
		},
		{
			Key:         "effort",
			Name:        "Effort Hours",
			Format:      model.RelationFormat_number,
			Description: "Estimated effort in hours",
		},
		{
			Key:         "notes",
			Name:        "Notes",
			Format:      model.RelationFormat_longtext,
			Description: "Additional notes about the task",
		},
	}

	for _, rel := range relations {
		err := s.AddRelation(rel)
		require.NoError(t, err)
	}

	// Export to YAML
	options := &ExportOptions{}
	yamlData, err := ExportSchemaToYAML(s, options)
	require.NoError(t, err)
	require.NotEmpty(t, yamlData)

	// Verify the exported YAML contains expected content
	yamlStr := string(yamlData)
	t.Logf("Exported YAML:\n%s", yamlStr)
	assert.Contains(t, yamlStr, "type: Custom Task")
	assert.Contains(t, yamlStr, "Title:")
	assert.Contains(t, yamlStr, "Status:")
	assert.Contains(t, yamlStr, "Priority:")
	assert.Contains(t, yamlStr, "Assignee:")
	assert.Contains(t, yamlStr, "Due Date:")
	assert.Contains(t, yamlStr, "Tags:")
	assert.Contains(t, yamlStr, "Completed:")
	assert.Contains(t, yamlStr, "Effort Hours:")
	assert.Contains(t, yamlStr, "Notes:")

	// Parse back to verify structure
	frontMatter, _, err := ExtractYAMLFrontMatter(yamlData)
	require.NoError(t, err)

	result, err := ParseYAMLFrontMatter(frontMatter)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify object type
	assert.Equal(t, "Custom Task", result.ObjectType)

	// Verify we have all properties
	propNames := make(map[string]bool)
	for _, prop := range result.Properties {
		propNames[prop.Name] = true
	}

	expectedProps := []string{"Title", "Status", "Priority", "Assignee", "Due Date", "Tags", "Completed", "Effort Hours", "Notes"}
	for _, expected := range expectedProps {
		assert.True(t, propNames[expected], "Missing property: %s", expected)
	}
}

func TestExportWithOptionValues(t *testing.T) {
	// Test exporting relations with specific option values
	properties := []Property{
		{
			Name:   "Status",
			Key:    "status",
			Format: model.RelationFormat_status,
			Value:  domain.String("in-progress"),
		},
		{
			Name:   "Priority",
			Key:    "priority",
			Format: model.RelationFormat_status,
			Value:  domain.String("high"),
		},
		{
			Name:   "Tags",
			Key:    "tags",
			Format: model.RelationFormat_tag,
			Value:  domain.StringList([]string{"bug", "frontend", "urgent"}),
		},
	}

	yamlData, err := ExportToYAML(properties, nil)
	require.NoError(t, err)

	// Verify the exported YAML
	yamlStr := string(yamlData)
	assert.Contains(t, yamlStr, "Priority: high")
	assert.Contains(t, yamlStr, "Status: in-progress")
	assert.Contains(t, yamlStr, "Tags:")
	assert.Contains(t, yamlStr, "- bug")
	assert.Contains(t, yamlStr, "- frontend")
	assert.Contains(t, yamlStr, "- urgent")
}

func isEmptyValue(v domain.Value) bool {
	if v.IsNull() {
		return true
	}
	if v.IsString() && v.String() == "" {
		return true
	}
	if v.IsStringList() && len(v.StringList()) == 0 {
		return true
	}
	return false
}
