package yaml

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

// TestCompleteWorkflow demonstrates the full YAML package workflow with version support
func TestCompleteWorkflow(t *testing.T) {
	// Step 1: Create a schema with various property types
	testSchema := &schema.Schema{
		Type: &schema.Type{
			Key:  "custom_type",
			Name: "Project",
		},
		Relations: map[string]*schema.Relation{
			"proj_name": {
				Key:    "proj_name",
				Name:   "Project Name",
				Format: model.RelationFormat_shorttext,
			},
			"proj_status": {
				Key:     "proj_status",
				Name:    "Status",
				Format:  model.RelationFormat_status,
				Options: []string{"Planning", "In Progress", "Completed"},
			},
			"proj_tags": {
				Key:     "proj_tags",
				Name:    "Tags",
				Format:  model.RelationFormat_tag,
				Options: []string{"urgent", "review", "approved"},
			},
			"proj_deadline": {
				Key:         "proj_deadline",
				Name:        "Deadline",
				Format:      model.RelationFormat_date,
				IncludeTime: false,
			},
			"proj_links": {
				Key:         "proj_links",
				Name:        "Related Documents",
				Format:      model.RelationFormat_object,
				ObjectTypes: []string{"document", "note"},
			},
		},
	}

	// Step 2: Create a mock property resolver
	resolver := &mockResolver{schema: testSchema}

	// Step 3: Test parsing legacy YAML (v1)
	t.Run("parse legacy YAML", func(t *testing.T) {
		legacyYAML := `---
type: Project
Project Name: My Important Project
Status: In Progress
Tags:
  - urgent
  - review
Deadline: 2024-12-31
Related Documents:
  - ./docs/spec.md
  - ./docs/design.md
---

# Project Content

This is the project content.`

		// Extract front matter
		frontMatter, content, err := ExtractYAMLFrontMatter([]byte(legacyYAML))
		require.NoError(t, err)
		assert.Contains(t, string(content), "# Project Content")

		// Parse with resolver and base path
		result, err := ParseYAMLFrontMatterWithResolverAndPath(frontMatter, resolver, "/projects")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify object type
		assert.Equal(t, "Project", result.ObjectType)

		// Verify properties were resolved correctly
		assert.Equal(t, "My Important Project", result.Details.GetString("proj_name"))
		
		// Debug: print all properties to see what keys are actually used
		for _, prop := range result.Properties {
			t.Logf("Property: name=%s, key=%s, value=%v", prop.Name, prop.Key, prop.Value)
		}
		
		// Status might be mapped to bundle.RelationKeyStatus
		statusValue := result.Details.GetString("proj_status")
		if statusValue == "" {
			statusValue = result.Details.GetString(bundle.RelationKeyStatus)
		}
		assert.Equal(t, "In Progress", statusValue)
		
		// Tags might be mapped to bundle.RelationKeyTag
		tagsValue := result.Details.GetStringList("proj_tags")
		if len(tagsValue) == 0 {
			tagsValue = result.Details.GetStringList(bundle.RelationKeyTag)
		}
		assert.Equal(t, []string{"urgent", "review"}, tagsValue)

		// Check deadline
		deadline := result.Details.GetInt64("proj_deadline")
		assert.NotZero(t, deadline)

		// Check file paths were resolved
		docs := result.Details.GetStringList("proj_links")
		assert.Equal(t, []string{"/projects/docs/spec.md", "/projects/docs/design.md"}, docs)
	})

	// Step 4: Test exporting with current version (v2)
	t.Run("export to current version", func(t *testing.T) {
		// Create properties to export
		properties := []Property{
			{
				Name:   "Project Name",
				Key:    "proj_name",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("My Important Project"),
			},
			{
				Name:   "Status",
				Key:    "proj_status",
				Format: model.RelationFormat_status,
				Value:  domain.String("In Progress"),
			},
			{
				Name:   "Tags",
				Key:    "proj_tags",
				Format: model.RelationFormat_tag,
				Value:  domain.StringList([]string{"urgent", "review"}),
			},
			{
				Name:        "Deadline",
				Key:         "proj_deadline",
				Format:      model.RelationFormat_date,
				Value:       domain.Int64(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC).Unix()),
				IncludeTime: false,
			},
			{
				Name:   "Related Documents",
				Key:    "proj_links",
				Format: model.RelationFormat_object,
				Value:  domain.StringList([]string{"./docs/spec.md", "./docs/design.md"}),
			},
			{
				Name:   "Object type",
				Key:    "type",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String("Project"),
			},
		}

		// Export with options
		yamlContent, err := ExportToYAML(properties, &ExportOptions{
			PropertyNameMap: map[string]string{
				"proj_name":     "name", // Use shorter names in export
				"proj_status":   "status",
				"proj_tags":     "tags",
				"proj_deadline": "deadline",
				"proj_links":    "documents",
			},
		})
		require.NoError(t, err)

		// Verify exported content
		content := string(yamlContent)
		assert.Contains(t, content, "Object type: Project")
		assert.Contains(t, content, "name: My Important Project")
		assert.Contains(t, content, "status: In Progress")
		assert.Contains(t, content, "deadline: \"2024-12-31\"") // Date format
		assert.Contains(t, content, "- urgent")
		assert.Contains(t, content, "- review")
		assert.Contains(t, content, "- ./docs/spec.md")
	})

	// Step 7: Test error handling and edge cases
	t.Run("edge cases", func(t *testing.T) {
		// Test parsing with no front matter
		result, err := ParseYAMLFrontMatter([]byte(""))
		assert.NoError(t, err)
		assert.Nil(t, result)

		// Test parsing with invalid YAML
		_, err = ParseYAMLFrontMatter([]byte("invalid: yaml: content:"))
		assert.Error(t, err)
	})

	// Step 8: Test complete markdown workflow
	t.Run("complete markdown workflow", func(t *testing.T) {
		// Original markdown with YAML front matter
		originalContent := `---
type: Project
name: Complete Workflow Test
status: Planning
tags:
  - test
  - integration
deadline: 2025-01-15
documents:
  - ./refs/requirements.md
---

# Complete Workflow Test

This demonstrates the complete workflow of the YAML package.

## Features
- Version support
- Property resolution
- Schema integration
- File path handling`

		// Extract and parse
		frontMatter, markdownContent, err := ExtractYAMLFrontMatter([]byte(originalContent))
		require.NoError(t, err)

		result, err := ParseYAMLFrontMatterWithResolverAndPath(frontMatter, resolver, "/workspace")
		require.NoError(t, err)

		// Modify some properties
		updatedProperties := make([]Property, 0)
		for _, prop := range result.Properties {
			// Check for both "status" and "Status" since bundle mapping might change the name
			if prop.Name == "status" || prop.Name == "Status" {
				prop.Value = domain.String("In Progress")
			}
			updatedProperties = append(updatedProperties, prop)
		}

		// Add Object type property if it was in the original result
		if result.ObjectType != "" {
			updatedProperties = append(updatedProperties, Property{
				Name:   "Object type",
				Key:    "type",
				Format: model.RelationFormat_shorttext,
				Value:  domain.String(result.ObjectType),
			})
		}

		// Export back to YAML
		newFrontMatter, err := ExportToYAML(updatedProperties, &ExportOptions{})
		require.NoError(t, err)

		// Reconstruct the document
		reconstructed := string(newFrontMatter) + string(markdownContent)

		// Verify the update
		// Note: "status" might be exported as "Status" due to bundle mapping
		assert.True(t, strings.Contains(reconstructed, "status: In Progress") || strings.Contains(reconstructed, "Status: In Progress"))
		assert.Contains(t, reconstructed, "# Complete Workflow Test")
	})
}

// mockResolver implements PropertyResolver for testing
type mockResolver struct {
	schema *schema.Schema
}

func (m *mockResolver) ResolvePropertyKey(name string) string {
	// Check exact match first
	for key, rel := range m.schema.Relations {
		if rel.Name == name {
			return key
		}
	}

	// Check case-insensitive match
	nameLower := strings.ToLower(name)
	for key, rel := range m.schema.Relations {
		if strings.ToLower(rel.Name) == nameLower {
			return key
		}
	}

	return ""
}

func (m *mockResolver) GetRelationFormat(key string) model.RelationFormat {
	if rel, exists := m.schema.Relations[key]; exists {
		return rel.Format
	}
	return model.RelationFormat_longtext
}

func (m *mockResolver) ResolveOptionValue(relationKey string, optionName string) string {
	// For this test, just return the option name as-is
	return optionName
}

func (m *mockResolver) ResolveOptionValues(relationKey string, optionNames []string) []string {
	// For this test, just return the option names as-is
	return optionNames
}

func (m *mockResolver) ResolveObjectValues(objectNames []string) []string {
	// For this test, just return the object names as-is
	return objectNames
}
