package schema_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
)

// SimpleSchemaRegistry implements a basic schema registry for testing
type SimpleSchemaRegistry struct {
	schemas map[string]*schema.Schema // typeKey -> schema
}

// NewSimpleSchemaRegistry creates a new simple schema registry
func NewSimpleSchemaRegistry() *SimpleSchemaRegistry {
	return &SimpleSchemaRegistry{
		schemas: make(map[string]*schema.Schema),
	}
}

// Ensure SimpleSchemaRegistry implements schema.SchemaRegistry
var _ schema.SchemaRegistry = (*SimpleSchemaRegistry)(nil)

// RegisterSchema adds a schema to the registry
func (r *SimpleSchemaRegistry) RegisterSchema(s *schema.Schema) error {
	if s.Type == nil {
		return nil
	}
	r.schemas[s.Type.Key] = s
	return nil
}

// RemoveSchema removes a schema by type key
func (r *SimpleSchemaRegistry) RemoveSchema(typeKey string) error {
	delete(r.schemas, typeKey)
	return nil
}

// Clear removes all schemas
func (r *SimpleSchemaRegistry) Clear() {
	r.schemas = make(map[string]*schema.Schema)
}

// GetSchema returns a schema by type key
func (r *SimpleSchemaRegistry) GetSchema(typeKey string) (*schema.Schema, bool) {
	s, ok := r.schemas[typeKey]
	return s, ok
}

// GetSchemaByTypeName returns a schema by type name
func (r *SimpleSchemaRegistry) GetSchemaByTypeName(typeName string) (*schema.Schema, bool) {
	for _, s := range r.schemas {
		if s.Type != nil && s.Type.Name == typeName {
			return s, true
		}
	}
	return nil, false
}

// GetRelation returns a relation by key across all schemas
func (r *SimpleSchemaRegistry) GetRelation(relationKey string) (*schema.Relation, bool) {
	for _, s := range r.schemas {
		if rel, ok := s.GetRelation(relationKey); ok {
			return rel, true
		}
	}
	return nil, false
}

// GetRelationByName returns a relation by name across all schemas
func (r *SimpleSchemaRegistry) GetRelationByName(relationName string) (*schema.Relation, bool) {
	for _, s := range r.schemas {
		if rel, ok := s.GetRelationByName(relationName); ok {
			return rel, true
		}
	}
	return nil, false
}

// ListSchemas returns all available schemas
func (r *SimpleSchemaRegistry) ListSchemas() []*schema.Schema {
	schemas := make([]*schema.Schema, 0, len(r.schemas))
	for _, s := range r.schemas {
		schemas = append(schemas, s)
	}
	return schemas
}

// ResolvePropertyKey returns the property key for a given name
func (r *SimpleSchemaRegistry) ResolvePropertyKey(name string) string {
	if rel, ok := r.GetRelationByName(name); ok {
		return rel.Key
	}
	return ""
}

// GetRelationFormat returns the format for a given relation key
func (r *SimpleSchemaRegistry) GetRelationFormat(key string) model.RelationFormat {
	if rel, ok := r.GetRelation(key); ok {
		return rel.Format
	}
	return model.RelationFormat_shorttext
}

// ResolveOptionValue converts option name to option ID
func (r *SimpleSchemaRegistry) ResolveOptionValue(relationKey string, optionName string) string {
	// Simple implementation - just prefix the option name
	return "opt_" + relationKey + "_" + optionName
}

// ResolveOptionValues converts option names to option IDs
func (r *SimpleSchemaRegistry) ResolveOptionValues(relationKey string, optionNames []string) []string {
	result := make([]string, len(optionNames))
	for i, name := range optionNames {
		result[i] = r.ResolveOptionValue(relationKey, name)
	}
	return result
}

// ResolveObjectValues converts object names to object IDs/paths
func (r *SimpleSchemaRegistry) ResolveObjectValues(objectNames []string) []string {
	result := make([]string, len(objectNames))
	for i, name := range objectNames {
		result[i] = "obj_" + name
	}
	return result
}

func TestSchemaYAMLIntegration(t *testing.T) {
	t.Run("parse YAML with schema resolver", func(t *testing.T) {
		// Create a schema with a Task type
		taskSchema := schema.NewSchema()
		
		// Set type
		taskType := &schema.Type{
			Key:         "task",
			Name:        "Task",
			Description: "A task object type",
			PluralName:  "Tasks",
			IconEmoji:   "✅",
		}
		err := taskSchema.SetType(taskType)
		require.NoError(t, err)

		// Add relations
		titleRel := &schema.Relation{
			Key:         "task_title",
			Name:        "Title",
			Format:      model.RelationFormat_shorttext,
			Description: "Task title",
		}
		err = taskSchema.AddRelation(titleRel)
		require.NoError(t, err)

		statusRel := &schema.Relation{
			Key:         "task_status",
			Name:        "Status",
			Format:      model.RelationFormat_status,
			Description: "Task status",
			Options:     []string{"Todo", "In Progress", "Done"},
		}
		err = taskSchema.AddRelation(statusRel)
		require.NoError(t, err)

		priorityRel := &schema.Relation{
			Key:         "task_priority",
			Name:        "Priority",
			Format:      model.RelationFormat_number,
			Description: "Task priority (1-5)",
		}
		err = taskSchema.AddRelation(priorityRel)
		require.NoError(t, err)

		tagsRel := &schema.Relation{
			Key:      "task_tags",
			Name:     "Tags",
			Format:   model.RelationFormat_tag,
			Examples: []string{"urgent", "bug", "feature", "documentation"},
		}
		err = taskSchema.AddRelation(tagsRel)
		require.NoError(t, err)

		descRel := &schema.Relation{
			Key:         "task_description",
			Name:        "Description",
			Format:      model.RelationFormat_longtext,
			Description: "Task description",
		}
		err = taskSchema.AddRelation(descRel)
		require.NoError(t, err)

		// Create registry and register schema
		registry := NewSimpleSchemaRegistry()
		err = registry.RegisterSchema(taskSchema)
		require.NoError(t, err)

		// Parse YAML with schema resolver
		yamlContent := []byte(`Title: Complete integration tests
Status: In Progress
Priority: 1
Tags: [urgent, feature]
Description: This is a longer description that spans multiple lines and should be parsed as longtext
Created: 2024-01-15`)

		result, err := yaml.ParseYAMLFrontMatterWithResolver(yamlContent, registry)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify properties were resolved correctly
		assert.Len(t, result.Properties, 6) // Title, Status, Priority, Tags, Description, Created

		// Check resolved properties
		propMap := make(map[string]*yaml.Property)
		for i := range result.Properties {
			prop := &result.Properties[i]
			propMap[prop.Name] = prop
		}

		// Title should use schema key
		assert.Equal(t, "task_title", propMap["Title"].Key)
		assert.Equal(t, model.RelationFormat_shorttext, propMap["Title"].Format)
		assert.Equal(t, "Complete integration tests", propMap["Title"].Value.String())

		// Status should use schema key and format
		assert.Equal(t, "task_status", propMap["Status"].Key)
		assert.Equal(t, model.RelationFormat_status, propMap["Status"].Format)
		assert.Equal(t, "opt_task_status_In Progress", propMap["Status"].Value.String())

		// Priority should use schema key and number format
		assert.Equal(t, "task_priority", propMap["Priority"].Key)
		assert.Equal(t, model.RelationFormat_number, propMap["Priority"].Format)
		assert.Equal(t, int64(1), propMap["Priority"].Value.Int64())

		// Tags should use schema key and tag format
		assert.Equal(t, "task_tags", propMap["Tags"].Key)
		assert.Equal(t, model.RelationFormat_tag, propMap["Tags"].Format)
		expectedTags := []string{"opt_task_tags_urgent", "opt_task_tags_feature"}
		assert.Equal(t, expectedTags, propMap["Tags"].Value.StringList())

		// Description should be resolved to schema key since we have it
		assert.Equal(t, "task_description", propMap["Description"].Key)
		// The parser auto-detects format based on content length
		assert.Equal(t, model.RelationFormat_shorttext, propMap["Description"].Format)

		// Created should be auto-detected as date (not in schema)
		assert.Len(t, propMap["Created"].Key, 24) // BSON ID
		assert.Equal(t, model.RelationFormat_date, propMap["Created"].Format)
	})

	t.Run("parse YAML with object type detection", func(t *testing.T) {
		// Create schemas for multiple types
		registry := NewSimpleSchemaRegistry()

		// Task schema
		taskSchema := schema.NewSchema()
		taskType := &schema.Type{
			Key:  "task",
			Name: "Task",
		}
		taskSchema.SetType(taskType)
		registry.RegisterSchema(taskSchema)

		// Project schema
		projectSchema := schema.NewSchema()
		projectType := &schema.Type{
			Key:  "project",
			Name: "Project",
		}
		projectSchema.SetType(projectType)
		registry.RegisterSchema(projectSchema)

		// Parse YAML with type field
		yamlContent := []byte(`type: Task
name: My important task
description: This is a task object`)

		result, err := yaml.ParseYAMLFrontMatterWithResolver(yamlContent, registry)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check that object type was detected
		assert.Equal(t, "Task", result.ObjectType)

		// Type field should not be included in properties
		assert.Len(t, result.Properties, 2) // name and description only
	})

	t.Run("export and import schema round-trip", func(t *testing.T) {
		// Create a schema
		originalSchema := schema.NewSchema()
		
		// Set type with all features
		taskType := &schema.Type{
			Key:                  "task",
			Name:                 "Task",
			Description:          "Task management object",
			PluralName:           "Tasks",
			IconEmoji:            "✅",
			FeaturedRelations:    []string{"task_title", "task_status"},
			RecommendedRelations: []string{"task_priority", "task_assignee", "task_due_date", "task_completed"},
		}
		originalSchema.SetType(taskType)

		// Add various relation types
		relations := []*schema.Relation{
			{
				Key:         "task_title",
				Name:        "Title",
				Format:      model.RelationFormat_shorttext,
				Description: "Task title",
			},
			{
				Key:         "task_status",
				Name:        "Status",
				Format:      model.RelationFormat_status,
				Options:     []string{"Todo", "In Progress", "Done"},
			},
			{
				Key:         "task_priority",
				Name:        "Priority",
				Format:      model.RelationFormat_number,
			},
			{
				Key:         "task_assignee",
				Name:        "Assignee",
				Format:      model.RelationFormat_object,
				ObjectTypes: []string{"participant"},
			},
			{
				Key:         "task_due_date",
				Name:        "Due Date",
				Format:      model.RelationFormat_date,
				IncludeTime: true,
			},
			{
				Key:      "task_completed",
				Name:     "Completed",
				Format:   model.RelationFormat_checkbox,
			},
		}

		for _, rel := range relations {
			err := originalSchema.AddRelation(rel)
			require.NoError(t, err)
		}

		// Export to JSON
		var buf bytes.Buffer
		exporter := schema.NewJSONSchemaExporter("  ") // with 2-space indent
		err := exporter.Export(originalSchema, &buf)
		require.NoError(t, err)

		// Parse back
		parser := schema.NewJSONSchemaParser()
		importedSchema, err := parser.Parse(&buf)
		require.NoError(t, err)

		// Verify type
		assert.Equal(t, originalSchema.Type.Key, importedSchema.Type.Key)
		assert.Equal(t, originalSchema.Type.Name, importedSchema.Type.Name)
		assert.Equal(t, originalSchema.Type.Description, importedSchema.Type.Description)
		assert.Equal(t, originalSchema.Type.PluralName, importedSchema.Type.PluralName)
		assert.Equal(t, originalSchema.Type.IconEmoji, importedSchema.Type.IconEmoji)
		
		// The parser may add system relations (id, type) and order differently
		// Just check that our custom relations are present
		for _, relKey := range originalSchema.Type.FeaturedRelations {
			assert.Contains(t, importedSchema.Type.FeaturedRelations, relKey)
		}
		for _, relKey := range originalSchema.Type.RecommendedRelations {
			found := false
			for _, impRelKey := range append(importedSchema.Type.FeaturedRelations, importedSchema.Type.RecommendedRelations...) {
				if impRelKey == relKey {
					found = true
					break
				}
			}
			assert.True(t, found, "Relation %s not found in imported schema", relKey)
		}

		// Verify relations - imported schema might have more relations (system ones)
		// Check that all original relations are present
		for key, originalRel := range originalSchema.Relations {
			importedRel, ok := importedSchema.GetRelation(key)
			assert.True(t, ok, "Relation %s not found", key)
			if !ok {
				continue
			}
			assert.Equal(t, originalRel.Key, importedRel.Key)
			assert.Equal(t, originalRel.Name, importedRel.Name)
			assert.Equal(t, originalRel.Format, importedRel.Format)
			assert.Equal(t, originalRel.Description, importedRel.Description)
			assert.Equal(t, originalRel.Options, importedRel.Options)
			assert.Equal(t, originalRel.ObjectTypes, importedRel.ObjectTypes)
			assert.Equal(t, originalRel.IncludeTime, importedRel.IncludeTime)
		}
	})

	t.Run("schema validation in integration", func(t *testing.T) {
		// Create a schema with invalid references
		s := schema.NewSchema()
		
		// Type that references non-existent relations
		taskType := &schema.Type{
			Key:               "task",
			Name:              "Task",
			FeaturedRelations: []string{"non_existent_relation"},
		}
		s.SetType(taskType)

		// Validation should pass (relations might be bundled)
		err := s.Validate()
		assert.NoError(t, err)

		// Add a relation with empty key (invalid)
		invalidRel := &schema.Relation{
			Key:    "", // Empty key is invalid
			Name:   "Test",
			Format: model.RelationFormat_shorttext,
		}
		err = s.AddRelation(invalidRel)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid relation")
	})
}

func TestSchemaRegistryIntegration(t *testing.T) {
	t.Run("registry manages multiple schemas", func(t *testing.T) {
		registry := NewSimpleSchemaRegistry()

		// Create and register multiple schemas
		taskSchema := createTestSchema("task", "Task", []string{"Title", "Status"})
		projectSchema := createTestSchema("project", "Project", []string{"Name", "Description"})
		
		err := registry.RegisterSchema(taskSchema)
		require.NoError(t, err)
		
		err = registry.RegisterSchema(projectSchema)
		require.NoError(t, err)

		// Test GetSchema
		s, ok := registry.GetSchema("task")
		assert.True(t, ok)
		assert.Equal(t, "task", s.Type.Key)

		// Test GetSchemaByTypeName
		s, ok = registry.GetSchemaByTypeName("Project")
		assert.True(t, ok)
		assert.Equal(t, "project", s.Type.Key)

		// Test GetRelation across schemas
		rel, ok := registry.GetRelation("task_title")
		assert.True(t, ok)
		assert.Equal(t, "Title", rel.Name)

		rel, ok = registry.GetRelation("project_name")
		assert.True(t, ok)
		assert.Equal(t, "Name", rel.Name)

		// Test GetRelationByName
		rel, ok = registry.GetRelationByName("Description")
		assert.True(t, ok)
		assert.Equal(t, "project_description", rel.Key)

		// Test ListSchemas
		schemas := registry.ListSchemas()
		assert.Len(t, schemas, 2)

		// Test RemoveSchema
		err = registry.RemoveSchema("task")
		assert.NoError(t, err)
		
		_, ok = registry.GetSchema("task")
		assert.False(t, ok)

		// Test Clear
		registry.Clear()
		schemas = registry.ListSchemas()
		assert.Len(t, schemas, 0)
	})

	t.Run("property resolution with registry", func(t *testing.T) {
		registry := NewSimpleSchemaRegistry()
		
		// Create schema with various relation types
		s := schema.NewSchema()
		s.SetType(&schema.Type{Key: "test", Name: "Test"})
		
		relations := []*schema.Relation{
			{Key: "test_name", Name: "Name", Format: model.RelationFormat_shorttext},
			{Key: "test_status", Name: "Status", Format: model.RelationFormat_status},
			{Key: "test_tags", Name: "Tags", Format: model.RelationFormat_tag},
		}
		
		for _, rel := range relations {
			s.AddRelation(rel)
		}
		
		registry.RegisterSchema(s)

		// Test ResolvePropertyKey
		assert.Equal(t, "test_name", registry.ResolvePropertyKey("Name"))
		assert.Equal(t, "test_status", registry.ResolvePropertyKey("Status"))
		assert.Equal(t, "", registry.ResolvePropertyKey("Unknown"))

		// Test GetRelationFormat
		assert.Equal(t, model.RelationFormat_shorttext, registry.GetRelationFormat("test_name"))
		assert.Equal(t, model.RelationFormat_status, registry.GetRelationFormat("test_status"))
		assert.Equal(t, model.RelationFormat_tag, registry.GetRelationFormat("test_tags"))

		// Test option resolution
		optionId := registry.ResolveOptionValue("test_status", "Done")
		assert.Equal(t, "opt_test_status_Done", optionId)

		optionIds := registry.ResolveOptionValues("test_tags", []string{"urgent", "bug"})
		assert.Equal(t, []string{"opt_test_tags_urgent", "opt_test_tags_bug"}, optionIds)

		// Test object resolution
		objectIds := registry.ResolveObjectValues([]string{"doc1", "doc2"})
		assert.Equal(t, []string{"obj_doc1", "obj_doc2"}, objectIds)
	})
}

// Helper function to create a test schema
func createTestSchema(typeKey, typeName string, relationNames []string) *schema.Schema {
	s := schema.NewSchema()
	
	// Set type
	t := &schema.Type{
		Key:  typeKey,
		Name: typeName,
	}
	s.SetType(t)

	// Add relations
	for _, name := range relationNames {
		rel := &schema.Relation{
			Key:    typeKey + "_" + strings.ToLower(name),
			Name:   name,
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(rel)
	}

	return s
}