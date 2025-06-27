package yaml

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestExportToYAML(t *testing.T) {
	tests := []struct {
		name       string
		properties []Property
		options    *ExportOptions
		want       string
	}{
		{
			name: "simple properties",
			properties: []Property{
				{
					Name:   "title",
					Key:    "title",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Test Page"),
				},
				{
					Name:   "author",
					Key:    "author",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("John Doe"),
				},
				{
					Name:   "published",
					Key:    "published",
					Format: model.RelationFormat_checkbox,
					Value:  domain.Bool(true),
				},
			},
			want: `---
title: Test Page
author: John Doe
published: true
---
`,
		},
		{
			name: "with object type",
			properties: []Property{
				{
					Name:   "name",
					Key:    "name",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("My Task"),
				},
				{
					Name:   "Object type",
					Key:    "type",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Task"),
				},
			},
			options: &ExportOptions{},
			want: `---
name: My Task
Object type: Task
---
`,
		},
		{
			name: "with custom property names",
			properties: []Property{
				{
					Name:   "task_title",
					Key:    "task_title",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Complete integration"),
				},
				{
					Name:   "task_status",
					Key:    "task_status",
					Format: model.RelationFormat_status,
					Value:  domain.String("in-progress"),
				},
			},
			options: &ExportOptions{
				PropertyNameMap: map[string]string{
					"task_title":  "Title",
					"task_status": "Status",
				},
			},
			want: `---
Title: Complete integration
Status: in-progress
---
`,
		},
		{
			name: "with skip properties",
			properties: []Property{
				{
					Name:   "title",
					Key:    "title",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Test"),
				},
				{
					Name:   "internal_id",
					Key:    "internal_id",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("12345"),
				},
				{
					Name:   "author",
					Key:    "author",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Jane"),
				},
			},
			options: &ExportOptions{
				SkipProperties: []string{"internal_id"},
			},
			want: `---
title: Test
author: Jane
---
`,
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
					Name:        "updated",
					Key:         "updated",
					Format:      model.RelationFormat_date,
					Value:       domain.Int64(time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC).Unix()),
					IncludeTime: true,
				},
			},
			want: `---
created: "2024-01-15"
updated: "2024-01-15T14:30:00Z"
---
`,
		},
		{
			name: "array properties",
			properties: []Property{
				{
					Name:   "tags",
					Key:    "tags",
					Format: model.RelationFormat_tag,
					Value:  domain.StringList([]string{"test", "markdown", "yaml"}),
				},
				{
					Name:   "files",
					Key:    "files",
					Format: model.RelationFormat_object,
					Value:  domain.StringList([]string{"doc1.md", "doc2.md"}),
				},
			},
			want: `---
tags:
    - test
    - markdown
    - yaml
files:
    - doc1.md
    - doc2.md
---
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExportToYAML(tt.properties, tt.options)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(result))
		})
	}
}

func TestExportDetailsToYAML(t *testing.T) {
	details := domain.NewDetails()
	details.Set("title", domain.String("Test Document"))
	details.Set("author", domain.String("John Doe"))
	details.Set("created", domain.Int64(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix()))
	details.Set("tags", domain.StringList([]string{"test", "yaml"}))
	details.Set("published", domain.Bool(true))
	details.Set("type", domain.String("Page"))

	formats := map[string]model.RelationFormat{
		"title":     model.RelationFormat_shorttext,
		"author":    model.RelationFormat_shorttext,
		"created":   model.RelationFormat_date,
		"tags":      model.RelationFormat_tag,
		"published": model.RelationFormat_checkbox,
		"type":      model.RelationFormat_shorttext,
	}

	options := &ExportOptions{
		PropertyNameMap: map[string]string{
			"title":  "Title",
			"author": "Author",
		},
	}

	result, err := ExportDetailsToYAML(details, formats, options)
	require.NoError(t, err)

	// Parse back to verify
	parsed, _, err := ExtractYAMLFrontMatter(result)
	require.NoError(t, err)

	parsedResult, err := ParseYAMLFrontMatter(parsed)
	require.NoError(t, err)

	// Verify object type
	assert.Equal(t, "Page", parsedResult.ObjectType)

	// Verify properties exist with correct names
	propNames := make(map[string]bool)
	for _, prop := range parsedResult.Properties {
		propNames[prop.Name] = true
	}

	assert.True(t, propNames["Title"])
	assert.True(t, propNames["Author"])
	assert.True(t, propNames["created"])
	assert.True(t, propNames["Tag"])  // "tags" is mapped to bundle.RelationKeyTag which has name "Tag"
	assert.True(t, propNames["published"])
}

func TestYAMLPropertyNameDeduplication(t *testing.T) {
	t.Run("YAML export deduplicates property names", func(t *testing.T) {
		// Create properties with duplicate names
		properties := []Property{
			{
				Name:   "Name",
				Key:    "user_name",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("John Doe"),
			},
			{
				Name:   "Name",
				Key:    "company_name",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Acme Corp"),
			},
			{
				Name:   "Name",
				Key:    "project_name",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Alpha Project"),
			},
			{
				Name:   "Description",
				Key:    "description",
				Format: model.RelationFormat_longtext,
				Value:  domain.String("A detailed description"),
			},
		}

		// Export to YAML
		result, err := ExportToYAML(properties, nil)
		require.NoError(t, err)

		yamlStr := string(result)

		// Should have deduplicated names sorted by key
		// Expected: company_name -> "Name", project_name -> "Name 2", user_name -> "Name 3"
		assert.Contains(t, yamlStr, "Name: Acme Corp", "First Name should be company_name")
		assert.Contains(t, yamlStr, "Name 2: Alpha Project", "Second Name should be project_name")
		assert.Contains(t, yamlStr, "Name 3: John Doe", "Third Name should be user_name")
		assert.Contains(t, yamlStr, "Description: A detailed description", "Description should keep original name")

		// Verify structure
		assert.Contains(t, yamlStr, "---\n")
		assert.Contains(t, yamlStr, "\n---\n")
	})

	t.Run("YAML export with custom property names and deduplication", func(t *testing.T) {
		// Create properties with some custom names that create conflicts
		properties := []Property{
			{
				Name:   "Title",
				Key:    "title",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Main Title"),
			},
			{
				Name:   "Name",
				Key:    "name",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Entity Name"),
			},
			{
				Name:   "Other",
				Key:    "other",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Other Value"),
			},
		}

		// Use custom property name map that creates a conflict
		options := &ExportOptions{
			PropertyNameMap: map[string]string{
				"title": "Name", // This will conflict with the "name" property
				"other": "Custom Name",
			},
		}

		// Export to YAML
		result, err := ExportToYAML(properties, options)
		require.NoError(t, err)

		yamlStr := string(result)

		// Should have deduplicated names
		// Expected: name -> "Name", title (custom) -> "Name 2"
		assert.Contains(t, yamlStr, "Name: Entity Name", "First Name should be from name key")
		assert.Contains(t, yamlStr, "Name 2: Main Title", "Second Name should be from title key with custom name")
		assert.Contains(t, yamlStr, "Custom Name: Other Value", "Custom name should be preserved when no conflict")
	})

	t.Run("YAML export with no duplicate names", func(t *testing.T) {
		// Create properties with unique names
		properties := []Property{
			{
				Name:   "Title",
				Key:    "title",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Page Title"),
			},
			{
				Name:   "Author",
				Key:    "author",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Jane Smith"),
			},
			{
				Name:   "Status",
				Key:    "status",
				Format: model.RelationFormat_status,
				Value:  domain.String("published"),
			},
		}

		// Export to YAML
		result, err := ExportToYAML(properties, nil)
		require.NoError(t, err)

		yamlStr := string(result)

		// Should keep original names without any suffixes
		assert.Contains(t, yamlStr, "Title: Page Title")
		assert.Contains(t, yamlStr, "Author: Jane Smith")
		assert.Contains(t, yamlStr, "Status: published")

		// Should not have any numbered suffixes
		assert.NotContains(t, yamlStr, "Title 2")
		assert.NotContains(t, yamlStr, "Author 2")
		assert.NotContains(t, yamlStr, "Status 2")
	})

	t.Run("Object type property always keeps its name", func(t *testing.T) {
		// Create properties where user properties conflict with "Object type"
		properties := []Property{
			{
				Name:   "Object type",
				Key:    "custom_object_type",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Custom Type"),
			},
			{
				Name:   "Object type",
				Key:    "another_object_type",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Another Type"),
			},
			{
				Name:   "Title",
				Key:    "title",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Document Title"),
			},
			{
				Name:   "Object type",
				Key:    "type",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Document"),
			},
		}

		// Export to YAML
		result, err := ExportToYAML(properties, &ExportOptions{})
		require.NoError(t, err)

		yamlStr := string(result)

		// "Object type" should be reserved for the system type
		assert.Contains(t, yamlStr, "Object type: Document", "System Object type should be present")

		// User properties named "Object type" get suffixes starting from 2
		// They are sorted by key: another_object_type, custom_object_type
		assert.Contains(t, yamlStr, "Object type 2: Another Type", "First user property (another_object_type) gets suffix 2")
		assert.Contains(t, yamlStr, "Object type 3: Custom Type", "Second user property (custom_object_type) gets suffix 3")

		// Other properties remain unchanged
		assert.Contains(t, yamlStr, "Title: Document Title")
	})
}

func TestExportToYAML_WithSchemaReference(t *testing.T) {
	t.Run("includes schema reference when provided", func(t *testing.T) {
		properties := []Property{
			{
				Name:   "Title",
				Key:    "title",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Test Document"),
			},
			{
				Name:   "Object type",
				Key:    "type",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Document"),
			},
		}

		options := &ExportOptions{
			SchemaReference: "./schemas/document.schema.json",
		}

		result, err := ExportToYAML(properties, options)
		require.NoError(t, err)

		yamlStr := string(result)

		// Should contain schema reference comment
		assert.Contains(t, yamlStr, "# yaml-language-server: $schema=./schemas/document.schema.json")

		// Should still have all properties
		assert.Contains(t, yamlStr, "Title: Test Document")
		assert.Contains(t, yamlStr, "Object type: Document")

		// Verify order - schema reference should come right after opening delimiter
		lines := strings.Split(yamlStr, "\n")
		assert.Equal(t, "---", lines[0])
		assert.Equal(t, "# yaml-language-server: $schema=./schemas/document.schema.json", lines[1])
	})

	t.Run("no schema reference when not provided", func(t *testing.T) {
		properties := []Property{
			{
				Name:   "Title",
				Key:    "title",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Test Document"),
			},
			{
				Name:   "Object type",
				Key:    "type",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Document"),
			},
		}

		options := &ExportOptions{}

		result, err := ExportToYAML(properties, options)
		require.NoError(t, err)

		yamlStr := string(result)

		// Should not contain schema reference
		assert.NotContains(t, yamlStr, "yaml-language-server")
	})
}
