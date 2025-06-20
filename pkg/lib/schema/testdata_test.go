package schema_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
)

func TestTestdataIntegration(t *testing.T) {
	t.Run("load task schema and parse task YAML", func(t *testing.T) {
		// Load task schema
		schemaPath := filepath.Join("testdata", "task_schema.json")
		schemaFile, err := os.Open(schemaPath)
		require.NoError(t, err)
		defer schemaFile.Close()

		parser := schema.NewJSONSchemaParser()
		taskSchema, err := parser.Parse(schemaFile)
		require.NoError(t, err)
		require.NotNil(t, taskSchema)

		// Create registry and register schema
		registry := NewSimpleSchemaRegistry()
		err = registry.RegisterSchema(taskSchema)
		require.NoError(t, err)

		// Load and parse task YAML
		yamlPath := filepath.Join("testdata", "sample_task.yaml")
		yamlContent, err := os.ReadFile(yamlPath)
		require.NoError(t, err)

		// Extract YAML front matter
		frontMatter, _, err := yaml.ExtractYAMLFrontMatter(yamlContent)
		require.NoError(t, err)
		require.NotNil(t, frontMatter)

		// Parse with schema resolver
		result, err := yaml.ParseYAMLFrontMatterWithResolver(frontMatter, registry)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify object type
		assert.Equal(t, "Task", result.ObjectType)

		// Verify properties
		propMap := make(map[string]*yaml.Property)
		for i := range result.Properties {
			prop := &result.Properties[i]
			propMap[prop.Name] = prop
		}

		// Check Title
		assert.Equal(t, "task_title", propMap["Title"].Key)
		assert.Equal(t, model.RelationFormat_shorttext, propMap["Title"].Format)
		assert.Equal(t, "Implement schema integration", propMap["Title"].Value.String())

		// Check Status
		assert.Equal(t, "task_status", propMap["Status"].Key)
		assert.Equal(t, model.RelationFormat_status, propMap["Status"].Format)
		assert.Equal(t, "opt_task_status_In Progress", propMap["Status"].Value.String())

		// Check Priority
		assert.Equal(t, "task_priority", propMap["Priority"].Key)
		assert.Equal(t, model.RelationFormat_number, propMap["Priority"].Format)
		assert.Equal(t, int64(1), propMap["Priority"].Value.Int64())

		// Check Due Date
		assert.Equal(t, "task_due_date", propMap["Due Date"].Key)
		assert.Equal(t, model.RelationFormat_date, propMap["Due Date"].Format)
		assert.True(t, propMap["Due Date"].IncludeTime)

		// Check Tags
		assert.Equal(t, "task_tags", propMap["Tags"].Key)
		assert.Equal(t, model.RelationFormat_tag, propMap["Tags"].Format)
		expectedTags := []string{"opt_task_tags_feature", "opt_task_tags_urgent"}
		assert.Equal(t, expectedTags, propMap["Tags"].Value.StringList())

		// Check Estimated Hours
		assert.Equal(t, "task_estimated_hours", propMap["Estimated Hours"].Key)
		assert.Equal(t, model.RelationFormat_number, propMap["Estimated Hours"].Format)
		assert.Equal(t, int64(8), propMap["Estimated Hours"].Value.Int64())

		// Check Description
		assert.Equal(t, "task_description", propMap["Description"].Key)
		assert.Equal(t, model.RelationFormat_longtext, propMap["Description"].Format)
		assert.Contains(t, propMap["Description"].Value.String(), "Create proper interfaces")
	})

	t.Run("load project schema and parse project YAML", func(t *testing.T) {
		// Load project schema
		schemaPath := filepath.Join("testdata", "project_schema.json")
		schemaFile, err := os.Open(schemaPath)
		require.NoError(t, err)
		defer schemaFile.Close()

		parser := schema.NewJSONSchemaParser()
		projectSchema, err := parser.Parse(schemaFile)
		require.NoError(t, err)
		require.NotNil(t, projectSchema)

		// Create registry and register schema
		registry := NewSimpleSchemaRegistry()
		err = registry.RegisterSchema(projectSchema)
		require.NoError(t, err)

		// Load and parse project YAML
		yamlPath := filepath.Join("testdata", "sample_project.yaml")
		yamlContent, err := os.ReadFile(yamlPath)
		require.NoError(t, err)

		// Extract YAML front matter
		frontMatter, markdownContent, err := yaml.ExtractYAMLFrontMatter(yamlContent)
		require.NoError(t, err)
		require.NotNil(t, frontMatter)
		require.NotNil(t, markdownContent)

		// Parse with schema resolver
		result, err := yaml.ParseYAMLFrontMatterWithResolver(frontMatter, registry)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify object type
		assert.Equal(t, "Project", result.ObjectType)

		// Verify markdown content was properly extracted
		assert.Contains(t, string(markdownContent), "# Project Overview")
		assert.Contains(t, string(markdownContent), "## Key Features")

		// Verify properties
		propMap := make(map[string]*yaml.Property)
		for i := range result.Properties {
			prop := &result.Properties[i]
			propMap[prop.Name] = prop
		}

		// Check Name
		assert.Equal(t, "project_name", propMap["Name"].Key)
		assert.Equal(t, model.RelationFormat_shorttext, propMap["Name"].Format)
		assert.Equal(t, "Anytype Schema System", propMap["Name"].Value.String())

		// Check Status
		assert.Equal(t, "project_status", propMap["Status"].Key)
		assert.Equal(t, model.RelationFormat_status, propMap["Status"].Format)
		assert.Equal(t, "opt_project_status_Active", propMap["Status"].Value.String())

		// Check Dates
		assert.Equal(t, "project_start_date", propMap["Start Date"].Key)
		assert.Equal(t, model.RelationFormat_date, propMap["Start Date"].Format)
		assert.False(t, propMap["Start Date"].IncludeTime) // No time specified

		assert.Equal(t, "project_end_date", propMap["End Date"].Key)
		assert.Equal(t, model.RelationFormat_date, propMap["End Date"].Format)

		// Check Budget
		assert.Equal(t, "project_budget", propMap["Budget"].Key)
		assert.Equal(t, model.RelationFormat_number, propMap["Budget"].Format)
		assert.Equal(t, int64(50000), propMap["Budget"].Value.Int64())

		// Check Description
		assert.Equal(t, "project_description", propMap["Description"].Key)
		assert.Equal(t, model.RelationFormat_longtext, propMap["Description"].Format)
		assert.Contains(t, propMap["Description"].Value.String(), "Design and implement")
	})

	t.Run("multiple schemas in registry", func(t *testing.T) {
		registry := NewSimpleSchemaRegistry()

		// Load both schemas
		schemas := []string{"task_schema.json", "project_schema.json"}
		for _, schemaFile := range schemas {
			path := filepath.Join("testdata", schemaFile)
			f, err := os.Open(path)
			require.NoError(t, err)
			defer f.Close()

			parser := schema.NewJSONSchemaParser()
			s, err := parser.Parse(f)
			require.NoError(t, err)

			err = registry.RegisterSchema(s)
			require.NoError(t, err)
		}

		// Verify both schemas are available
		taskSchema, ok := registry.GetSchemaByTypeName("Task")
		assert.True(t, ok)
		assert.Equal(t, "task", taskSchema.Type.Key)

		projectSchema, ok := registry.GetSchemaByTypeName("Project")
		assert.True(t, ok)
		assert.Equal(t, "project", projectSchema.Type.Key)

		// Verify relations from both schemas are accessible
		rel, ok := registry.GetRelationByName("Title")
		assert.True(t, ok)
		assert.Equal(t, "task_title", rel.Key)

		rel, ok = registry.GetRelationByName("Name")
		assert.True(t, ok)
		assert.Equal(t, "project_name", rel.Key)

		// Both have Description but with different keys
		rel, ok = registry.GetRelation("task_description")
		assert.True(t, ok)
		assert.Equal(t, "Description", rel.Name)

		rel, ok = registry.GetRelation("project_description")
		assert.True(t, ok)
		assert.Equal(t, "Description", rel.Name)
	})
}

func TestSchemaYAMLWithFilePaths(t *testing.T) {
	t.Run("resolve file paths in YAML", func(t *testing.T) {
		// Create a schema with object relations
		s := schema.NewSchema()
		s.SetType(&schema.Type{Key: "doc", Name: "Document"})
		
		s.AddRelation(&schema.Relation{
			Key:         "doc_attachments",
			Name:        "Attachments",
			Format:      model.RelationFormat_file,
			IsMulti:     true,
		})
		
		s.AddRelation(&schema.Relation{
			Key:         "doc_related",
			Name:        "Related Documents",
			Format:      model.RelationFormat_object,
			ObjectTypes: []string{"doc"},
			IsMulti:     true,
		})

		registry := NewSimpleSchemaRegistry()
		registry.RegisterSchema(s)

		// YAML with file paths
		yamlContent := []byte(`type: Document
Attachments: [image.png, data/spreadsheet.xlsx]
Related Documents: [intro.md, chapters/chapter1.md]`)

		basePath := "/Users/test/documents"
		result, err := yaml.ParseYAMLFrontMatterWithResolverAndPath(yamlContent, registry, basePath)
		require.NoError(t, err)

		propMap := make(map[string]*yaml.Property)
		for i := range result.Properties {
			prop := &result.Properties[i]
			propMap[prop.Name] = prop
		}

		// Check attachments - should resolve paths
		attachments := propMap["Attachments"].Value.StringList()
		assert.Equal(t, []string{
			"/Users/test/documents/image.png",
			"/Users/test/documents/data/spreadsheet.xlsx",
		}, attachments)

		// Check related documents - should resolve paths
		related := propMap["Related Documents"].Value.StringList()
		assert.Equal(t, []string{
			"/Users/test/documents/intro.md",
			"/Users/test/documents/chapters/chapter1.md",
		}, related)
	})
}