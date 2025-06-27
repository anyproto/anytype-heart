package schema_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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
			t.Logf("Property found: name=%s, key=%s, format=%v", prop.Name, prop.Key, prop.Format)
		}

		// Title should use schema key
		assert.Equal(t, "task_title", propMap["Title"].Key)
		assert.Equal(t, model.RelationFormat_shorttext, propMap["Title"].Format)
		assert.Equal(t, "Complete integration tests", propMap["Title"].Value.String())

		// Status is mapped to bundle.RelationKeyStatus, which takes precedence over schema
		assert.Equal(t, "status", propMap["Status"].Key)
		assert.Equal(t, model.RelationFormat_status, propMap["Status"].Format)
		// When using bundled relation, option IDs follow different pattern
		assert.Equal(t, "opt_status_In Progress", propMap["Status"].Value.String())

		// Priority should use schema key and number format
		assert.Equal(t, "task_priority", propMap["Priority"].Key)
		assert.Equal(t, model.RelationFormat_number, propMap["Priority"].Format)
		assert.Equal(t, int64(1), propMap["Priority"].Value.Int64())

		// Tags is mapped to bundle.RelationKeyTag with name "Tag"
		tagProp := propMap["Tag"]
		if tagProp == nil {
			t.Fatal("Tag property not found")
		}
		assert.Equal(t, "tag", tagProp.Key)
		assert.Equal(t, model.RelationFormat_tag, tagProp.Format)
		// When using bundled relation, option IDs follow different pattern
		expectedTags := []string{"opt_tag_urgent", "opt_tag_feature"}
		assert.Equal(t, expectedTags, tagProp.Value.StringList())

		// Description should be resolved to schema key since we have it
		assert.Equal(t, "task_description", propMap["Description"].Key)
		// The parser auto-detects format based on content length
		assert.Equal(t, model.RelationFormat_shorttext, propMap["Description"].Format)

		// Created is mapped to bundle.RelationKeyCreatedDate with name "Creation date"
		createdProp := propMap["Creation date"]
		if createdProp == nil {
			t.Fatal("Creation date property not found")
		}
		assert.Equal(t, "createdDate", createdProp.Key)
		assert.Equal(t, model.RelationFormat_date, createdProp.Format)
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
				Key:     "task_status",
				Name:    "Status",
				Format:  model.RelationFormat_status,
				Options: []string{"Todo", "In Progress", "Done"},
			},
			{
				Key:    "task_priority",
				Name:   "Priority",
				Format: model.RelationFormat_number,
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
				Key:    "task_completed",
				Name:   "Completed",
				Format: model.RelationFormat_checkbox,
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

func TestHiddenRelationsExportImport(t *testing.T) {
	t.Run("export type with hidden relations", func(t *testing.T) {
		// Create a schema with hidden relations
		s := schema.NewSchema()

		// Add relations
		titleRel := &schema.Relation{
			Key:    "title",
			Name:   "Title",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(titleRel)

		descRel := &schema.Relation{
			Key:    "description",
			Name:   "Description",
			Format: model.RelationFormat_longtext,
		}
		s.AddRelation(descRel)

		internalIdRel := &schema.Relation{
			Key:    "internal_id",
			Name:   "Internal ID",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(internalIdRel)

		secretRel := &schema.Relation{
			Key:    "secret_key",
			Name:   "Secret Key",
			Format: model.RelationFormat_shorttext,
		}
		s.AddRelation(secretRel)

		// Create type with hidden relations
		typ := &schema.Type{
			Key:                  "document",
			Name:                 "Document",
			FeaturedRelations:    []string{"title"},
			RecommendedRelations: []string{"description"},
			HiddenRelations:      []string{"internal_id", "secret_key"},
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

		// Verify relations should not have lists at schema level
		assert.Nil(t, jsonSchema["x-featured-relations"])
		assert.Nil(t, jsonSchema["x-recommended-relations"])
		assert.Nil(t, jsonSchema["x-hidden-relations"])

		// Verify hidden relations have x-hidden: true
		properties := jsonSchema["properties"].(map[string]interface{})

		// Check internal_id property
		internalIdProp := properties["Internal ID"].(map[string]interface{})
		assert.Equal(t, true, internalIdProp["x-hidden"])
		assert.Equal(t, "internal_id", internalIdProp["x-key"])

		// Check secret_key property
		secretProp := properties["Secret Key"].(map[string]interface{})
		assert.Equal(t, true, secretProp["x-hidden"])
		assert.Equal(t, "secret_key", secretProp["x-key"])

		// Check that featured relation has x-featured
		titleProp := properties["Title"].(map[string]interface{})
		assert.Equal(t, true, titleProp["x-featured"])
		assert.Nil(t, titleProp["x-hidden"])

		// Check that regular relation has neither flag
		descProp := properties["Description"].(map[string]interface{})
		assert.Nil(t, descProp["x-featured"])
		assert.Nil(t, descProp["x-hidden"])
	})

	t.Run("import type with hidden relations", func(t *testing.T) {
		jsonStr := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id": "urn:anytype:schema:2025-01-01:test:type-document:gen-1.0",
			"type": "object",
			"title": "Document",
			"x-type-key": "document",
			"properties": {
				"id": {
					"type": "string",
					"description": "Unique identifier of the Anytype object",
					"readOnly": true,
					"x-order": 0,
					"x-key": "id"
				},
				"Title": {
					"type": "string",
					"x-key": "title",
					"x-format": "shorttext",
					"x-order": 1,
					"x-featured": true
				},
				"Description": {
					"type": "string",
					"x-key": "description",
					"x-format": "longtext",
					"x-order": 2
				},
				"Author": {
					"type": "array",
					"items": {
						"type": "string"
					},
					"x-key": "author",
					"x-format": "object",
					"x-order": 3
				},
				"Internal ID": {
					"type": "string",
					"x-key": "internal_id",
					"x-format": "shorttext",
					"x-order": 4,
					"x-hidden": true
				},
				"Secret Key": {
					"type": "string",
					"x-key": "secret_key",
					"x-format": "shorttext",
					"x-order": 5,
					"x-hidden": true
				},
				"System Flag": {
					"type": "boolean",
					"x-key": "system_flag",
					"x-format": "checkbox",
					"x-order": 6,
					"x-hidden": true
				}
			}
		}`

		// Parse the JSON Schema
		parser := schema.NewJSONSchemaParser()
		s, err := parser.Parse(bytes.NewReader([]byte(jsonStr)))
		require.NoError(t, err)

		// Get the type
		typ := s.Type
		require.NotNil(t, typ)

		// Verify type properties
		assert.Equal(t, "document", typ.Key)
		assert.Equal(t, "Document", typ.Name)

		// Verify relation lists
		assert.ElementsMatch(t, []string{"title"}, typ.FeaturedRelations)
		assert.ElementsMatch(t, []string{"description", "author"}, typ.RecommendedRelations)
		// Verify hidden relations include specified ones + system properties
		for _, expectedHidden := range []string{"internal_id", "secret_key", "system_flag"} {
			assert.Contains(t, typ.HiddenRelations, expectedHidden)
		}
		for _, sysProp := range schema.SystemProperties {
			assert.Contains(t, typ.HiddenRelations, sysProp)
		}

		// Verify all relations were parsed
		rel, ok := s.GetRelation("internal_id")
		assert.True(t, ok)
		assert.Equal(t, "Internal ID", rel.Name)
		assert.Equal(t, model.RelationFormat_shorttext, rel.Format)

		rel, ok = s.GetRelation("secret_key")
		assert.True(t, ok)
		assert.Equal(t, "Secret Key", rel.Name)

		rel, ok = s.GetRelation("system_flag")
		assert.True(t, ok)
		assert.Equal(t, "System Flag", rel.Name)
		assert.Equal(t, model.RelationFormat_checkbox, rel.Format)
	})

	t.Run("round-trip with hidden relations", func(t *testing.T) {
		// Create original schema
		originalSchema := schema.NewSchema()

		// Add various relations
		relations := []*schema.Relation{
			{Key: "name", Name: "Name", Format: model.RelationFormat_shorttext},
			{Key: "status", Name: "Status", Format: model.RelationFormat_status, Options: []string{"Active", "Inactive"}},
			{Key: "created_by", Name: "Created By", Format: model.RelationFormat_object, ObjectTypes: []string{"participant"}},
			{Key: "internal_state", Name: "Internal State", Format: model.RelationFormat_shorttext},
			{Key: "sync_id", Name: "Sync ID", Format: model.RelationFormat_shorttext},
			{Key: "debug_info", Name: "Debug Info", Format: model.RelationFormat_longtext},
		}

		for _, rel := range relations {
			originalSchema.AddRelation(rel)
		}

		// Create type with all three relation lists
		originalType := &schema.Type{
			Key:                  "system_object",
			Name:                 "System Object",
			Description:          "An object with hidden system fields",
			FeaturedRelations:    []string{"name", "status"},
			RecommendedRelations: []string{"created_by", "type"},
			HiddenRelations:      []string{"internal_state", "sync_id", "debug_info"},
		}
		originalSchema.SetType(originalType)

		// Export to JSON
		exporter := schema.NewJSONSchemaExporter("  ")
		var buf bytes.Buffer
		err := exporter.Export(originalSchema, &buf)
		require.NoError(t, err)

		// Import back
		parser := schema.NewJSONSchemaParser()
		importedSchema, err := parser.Parse(&buf)
		require.NoError(t, err)

		// Verify type is the same
		importedType := importedSchema.Type
		assert.Equal(t, originalType.Key, importedType.Key)
		assert.Equal(t, originalType.Name, importedType.Name)
		assert.Equal(t, originalType.Description, importedType.Description)
		assert.ElementsMatch(t, originalType.FeaturedRelations, importedType.FeaturedRelations)
		assert.ElementsMatch(t, originalType.RecommendedRelations, importedType.RecommendedRelations)
		// Hidden relations should include original + system properties added on import
		for _, expectedHidden := range originalType.HiddenRelations {
			assert.Contains(t, importedType.HiddenRelations, expectedHidden)
		}
		// System properties are added during import
		for _, sysProp := range schema.SystemProperties {
			if slices.Contains(importedType.FeaturedRelations, sysProp) ||
				slices.Contains(importedType.RecommendedRelations, sysProp) {
				continue
			}
			assert.Contains(t, importedType.HiddenRelations, sysProp)
		}

		// Verify all relations preserved
		for _, originalRel := range relations {
			importedRel, ok := importedSchema.GetRelation(originalRel.Key)
			assert.True(t, ok, "Relation %s not found", originalRel.Key)
			assert.Equal(t, originalRel.Name, importedRel.Name)
			assert.Equal(t, originalRel.Format, importedRel.Format)
			if originalRel.Format == model.RelationFormat_status {
				assert.Equal(t, originalRel.Options, importedRel.Options)
			}
			if originalRel.Format == model.RelationFormat_object {
				assert.Equal(t, originalRel.ObjectTypes, importedRel.ObjectTypes)
			}
		}
	})

	t.Run("backward compatibility - x-hidden on properties", func(t *testing.T) {
		// Schema with old format using x-hidden on properties
		jsonStr := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id": "urn:anytype:schema:2025-01-01:test:type-legacy:gen-1.0",
			"type": "object",
			"title": "Legacy Type",
			"x-type-key": "legacy",
			"properties": {
				"id": {
					"type": "string",
					"description": "Unique identifier of the Anytype object",
					"readOnly": true,
					"x-order": 0,
					"x-key": "id"
				},
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-format": "shorttext",
					"x-order": 1,
					"x-featured": true
				},
				"Hidden Field": {
					"type": "string",
					"x-key": "hidden_field",
					"x-format": "shorttext",
					"x-order": 2,
					"x-hidden": true
				}
			}
		}`

		parser := schema.NewJSONSchemaParser()
		s, err := parser.Parse(bytes.NewReader([]byte(jsonStr)))
		require.NoError(t, err)

		typ := s.Type
		require.NotNil(t, typ)

		// Should parse x-featured and x-hidden from properties
		assert.Contains(t, typ.FeaturedRelations, "name")
		assert.Contains(t, typ.HiddenRelations, "hidden_field")
	})

	t.Run("type ToDetails exports hidden relations", func(t *testing.T) {
		// Create a type with hidden relations
		typ := &schema.Type{
			Key:                  "document",
			Name:                 "Document",
			Description:          "A document type",
			FeaturedRelations:    []string{"title", "status"},
			RecommendedRelations: []string{"author", "created_date"},
			HiddenRelations:      []string{"internal_id", "sync_state", "debug_flag"},
		}

		// Convert to details
		details := typ.ToDetails()

		// Verify all relation lists are in details
		featuredList := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
		assert.Equal(t, []string{"title", "status"}, featuredList)

		recommendedList := details.GetStringList(bundle.RelationKeyRecommendedRelations)
		assert.Equal(t, []string{"author", "created_date"}, recommendedList)

		hiddenList := details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations)
		assert.Equal(t, []string{"internal_id", "sync_state", "debug_flag"}, hiddenList)
	})

	t.Run("type FromDetails imports hidden relations", func(t *testing.T) {
		// Create details with hidden relations
		details := domain.NewDetails()
		details.SetString(bundle.RelationKeyName, "Document")
		details.SetString(bundle.RelationKeyDescription, "A document type")
		details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"title", "status"})
		details.SetStringList(bundle.RelationKeyRecommendedRelations, []string{"author", "created_date"})
		details.SetStringList(bundle.RelationKeyRecommendedHiddenRelations, []string{"internal_id", "sync_state", "debug_flag"})

		// Create unique key
		uniqueKey, _ := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, "document")
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())

		// Convert from details
		typ, err := schema.TypeFromDetails(details)
		require.NoError(t, err)

		// Verify type properties
		assert.Equal(t, "document", typ.Key)
		assert.Equal(t, "Document", typ.Name)
		assert.Equal(t, "A document type", typ.Description)

		// Verify relation lists
		assert.Equal(t, []string{"title", "status"}, typ.FeaturedRelations)
		assert.Equal(t, []string{"author", "created_date"}, typ.RecommendedRelations)
		assert.Equal(t, []string{"internal_id", "sync_state", "debug_flag"}, typ.HiddenRelations)
	})
}

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

		// Check Status - mapped to bundle.RelationKeyStatus
		assert.Equal(t, "status", propMap["Status"].Key)
		assert.Equal(t, model.RelationFormat_status, propMap["Status"].Format)
		assert.Equal(t, "opt_status_In Progress", propMap["Status"].Value.String())

		// Check Priority
		assert.Equal(t, "task_priority", propMap["Priority"].Key)
		assert.Equal(t, model.RelationFormat_number, propMap["Priority"].Format)
		assert.Equal(t, int64(1), propMap["Priority"].Value.Int64())

		// Check Due Date
		assert.Equal(t, "task_due_date", propMap["Due Date"].Key)
		assert.Equal(t, model.RelationFormat_date, propMap["Due Date"].Format)
		assert.True(t, propMap["Due Date"].IncludeTime)

		// Check Tags - mapped to bundle.RelationKeyTag with name "Tag"
		tagProp := propMap["Tag"]
		require.NotNil(t, tagProp, "Tag property not found")
		assert.Equal(t, "tag", tagProp.Key)
		assert.Equal(t, model.RelationFormat_tag, tagProp.Format)
		expectedTags := []string{"opt_tag_feature", "opt_tag_urgent"}
		assert.Equal(t, expectedTags, tagProp.Value.StringList())

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

		// Check Status - mapped to bundle.RelationKeyStatus
		assert.Equal(t, "status", propMap["Status"].Key)
		assert.Equal(t, model.RelationFormat_status, propMap["Status"].Format)
		assert.Equal(t, "opt_status_Active", propMap["Status"].Value.String())

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
			Key:     "doc_attachments",
			Name:    "Attachments",
			Format:  model.RelationFormat_file,
			IsMulti: true,
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

func TestNameDeduplicationIntegration(t *testing.T) {
	t.Run("end-to-end deduplication in schema and YAML export", func(t *testing.T) {
		// Create a schema with duplicate property names
		s := schema.NewSchema()

		// Add relations with duplicate names
		relations := []*schema.Relation{
			{Key: "user_name", Name: "Name", Format: model.RelationFormat_shorttext},
			{Key: "company_name", Name: "Name", Format: model.RelationFormat_shorttext},
			{Key: "project_title", Name: "Title", Format: model.RelationFormat_shorttext},
			{Key: "document_title", Name: "Title", Format: model.RelationFormat_shorttext},
			{Key: "description", Name: "Description", Format: model.RelationFormat_longtext},
		}

		for _, rel := range relations {
			s.AddRelation(rel)
		}

		// Create type
		typ := &schema.Type{
			Key:                  "entity",
			Name:                 "Entity",
			Description:          "An entity with duplicate property names",
			FeaturedRelations:    []string{"user_name", "project_title"},
			RecommendedRelations: []string{"company_name", "description"},
			HiddenRelations:      []string{"document_title"},
		}
		s.SetType(typ)

		// Test JSON Schema export
		t.Run("JSON Schema export with deduplication", func(t *testing.T) {
			exporter := schema.NewJSONSchemaExporter("  ")
			var buf bytes.Buffer
			err := exporter.Export(s, &buf)
			require.NoError(t, err)

			var jsonSchema map[string]interface{}
			err = json.Unmarshal(buf.Bytes(), &jsonSchema)
			require.NoError(t, err)

			properties := jsonSchema["properties"].(map[string]interface{})

			// Verify deduplicated names (sorted by key)
			// Expected: company_name -> "Name", user_name -> "Name 2"
			// document_title -> "Title", project_title -> "Title 2"
			nameProp := properties["Name"].(map[string]interface{})
			assert.Equal(t, "company_name", nameProp["x-key"])

			name2Prop := properties["Name 2"].(map[string]interface{})
			assert.Equal(t, "user_name", name2Prop["x-key"])

			titleProp := properties["Title"].(map[string]interface{})
			assert.Equal(t, "document_title", titleProp["x-key"])
			assert.Equal(t, true, titleProp["x-hidden"])

			title2Prop := properties["Title 2"].(map[string]interface{})
			assert.Equal(t, "project_title", title2Prop["x-key"])
			assert.Equal(t, true, title2Prop["x-featured"])

			descProp := properties["Description"].(map[string]interface{})
			assert.Equal(t, "description", descProp["x-key"])
		})

		// Test YAML export
		t.Run("YAML export with deduplication", func(t *testing.T) {
			// Create sample data using our schema
			properties := []yaml.Property{
				{Name: "Object type", Key: "type3", Format: model.RelationFormat_shorttext, Value: domain.String("Custom field with name type")},
				{Name: "Object type", Key: "type", Format: model.RelationFormat_shorttext, Value: domain.String("Entity")},
				{Name: "Name", Key: "user_name", Format: model.RelationFormat_shorttext, Value: domain.String("John Doe")},
				{Name: "Name", Key: "company_name", Format: model.RelationFormat_shorttext, Value: domain.String("Acme Corp")},
				{Name: "Title", Key: "project_title", Format: model.RelationFormat_shorttext, Value: domain.String("Project Alpha")},
				{Name: "Title", Key: "document_title", Format: model.RelationFormat_shorttext, Value: domain.String("Requirements Doc")},
				{Name: "Description", Key: "description", Format: model.RelationFormat_longtext, Value: domain.String("A comprehensive entity")},
			}

			// Export to YAML
			result, err := yaml.ExportToYAML(properties, &yaml.ExportOptions{})
			require.NoError(t, err)

			yamlStr := string(result)

			// Verify deduplicated names
			assert.Contains(t, yamlStr, "Object type: Entity")
			assert.Contains(t, yamlStr, "Name: Acme Corp")         // company_name first
			assert.Contains(t, yamlStr, "Name 2: John Doe")        // user_name second
			assert.Contains(t, yamlStr, "Title: Requirements Doc") // document_title first
			assert.Contains(t, yamlStr, "Title 2: Project Alpha")  // project_title second
			assert.Contains(t, yamlStr, "Description: A comprehensive entity")
		})

		// Test round-trip preservation
		t.Run("round-trip preserves deduplication logic", func(t *testing.T) {
			// Export schema to JSON
			exporter := schema.NewJSONSchemaExporter("  ")
			var buf1 bytes.Buffer
			err := exporter.Export(s, &buf1)
			require.NoError(t, err)

			// Import back
			parser := schema.NewJSONSchemaParser()
			importedSchema, err := parser.Parse(bytes.NewReader(buf1.Bytes()))
			require.NoError(t, err)

			// Export again
			var buf2 bytes.Buffer
			err = exporter.Export(importedSchema, &buf2)
			require.NoError(t, err)

			// Parse both exports
			var schema1, schema2 map[string]interface{}
			err = json.Unmarshal(buf1.Bytes(), &schema1)
			require.NoError(t, err)
			err = json.Unmarshal(buf2.Bytes(), &schema2)
			require.NoError(t, err)

			// Property names should be consistent
			props1 := schema1["properties"].(map[string]interface{})
			props2 := schema2["properties"].(map[string]interface{})

			// Check same property names exist
			for propName := range props1 {
				_, exists := props2[propName]
				assert.True(t, exists, "Property %s should exist in both exports", propName)
			}

			// Verify specific deduplicated names are consistent
			assert.Contains(t, props1, "Name")
			assert.Contains(t, props1, "Name 2")
			assert.Contains(t, props1, "Title")
			assert.Contains(t, props1, "Title 2")
			assert.Contains(t, props2, "Name")
			assert.Contains(t, props2, "Name 2")
			assert.Contains(t, props2, "Title")
			assert.Contains(t, props2, "Title 2")
		})
	})
}
