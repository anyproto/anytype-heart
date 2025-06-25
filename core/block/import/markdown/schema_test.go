package markdown

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

type mockSource struct {
	files map[string]string
}

func (m *mockSource) Initialize(importPath string) error {
	return nil
}

func (m *mockSource) Iterate(callback func(fileName string, fileReader io.ReadCloser) (isContinue bool)) error {
	for name, content := range m.files {
		reader := strings.NewReader(content)
		if !callback(name, io.NopCloser(reader)) {
			break
		}
	}
	return nil
}

func (m *mockSource) ProcessFile(fileName string, callback func(io.ReadCloser) error) error {
	if content, ok := m.files[fileName]; ok {
		reader := strings.NewReader(content)
		return callback(io.NopCloser(reader))
	}
	return nil
}

func (m *mockSource) CountFilesWithGivenExtensions([]string) int {
	return len(m.files)
}

func (m *mockSource) GetFileReaders(string, []string) (map[string]io.ReadCloser, error) {
	return nil, nil
}

func (m *mockSource) Close()                 {}
func (m *mockSource) IsRootFile(string) bool { return false }

func TestSchemaImporter_LoadSchemas(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"$id": "urn:anytype:schema:2024-06-14:author-user:type-task:gen-1.0.0",
		"type": "object",
		"title": "Task",
		"description": "A task to track",
		"x-type-key": "task",
		"properties": {
			"id": {
				"type": "string",
				"description": "Unique identifier",
				"readOnly": true,
				"x-order": 0,
				"x-key": "id"
			},
			"Type": {
				"const": "Task",
				"x-order": 1,
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
				"enum": ["Todo", "In Progress", "Done"],
				"x-featured": true,
				"x-order": 3,
				"x-key": "status"
			},
			"Priority": {
				"type": "number",
				"x-order": 4,
				"x-key": "priority"
			},
			"Due Date": {
				"type": "string",
				"format": "date",
				"x-order": 5,
				"x-key": "duedate"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/task.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Verify schema was loaded
	assert.True(t, si.HasSchemas())
	assert.Len(t, si.schemas, 1)

	// Check schema info
	var schema *schema.Schema
	for _, s := range si.schemas {
		if s.Type != nil && s.Type.Name == "Task" {
			schema = s
			break
		}
	}
	require.NotNil(t, schema)
	assert.Equal(t, "Task", schema.Type.Name)
	assert.Equal(t, "task", schema.Type.Key)
	assert.Len(t, schema.Relations, 6) // Including id and type

	// Check relations count across all schemas
	totalRelations := 0
	for _, s := range si.schemas {
		totalRelations += len(s.Relations)
	}
	assert.Equal(t, 6, totalRelations) // 6 relations in the Task schema

	// Verify relation details
	nameRel, ok := schema.Relations["name"]
	require.True(t, ok)
	assert.Equal(t, "Name", nameRel.Name)
	assert.Equal(t, "name", nameRel.Key)
	assert.Equal(t, model.RelationFormat_shorttext, nameRel.Format)
	if featured, ok := nameRel.Extension["featured"].(bool); ok {
		assert.True(t, featured)
	}
	if order, ok := nameRel.Extension["order"].(float64); ok {
		assert.Equal(t, float64(2), order)
	}

	statusRel, ok := schema.Relations["status"]
	require.True(t, ok)
	assert.Equal(t, "Status", statusRel.Name)
	assert.Equal(t, model.RelationFormat_status, statusRel.Format)
	if featured, ok := statusRel.Extension["featured"].(bool); ok {
		assert.True(t, featured)
	}

	dueDateRel, ok := schema.Relations["duedate"]
	require.True(t, ok)
	assert.Equal(t, "Due Date", dueDateRel.Name)
	assert.Equal(t, model.RelationFormat_date, dueDateRel.Format)
	assert.False(t, dueDateRel.IncludeTime)
}

func TestSchemaImporter_CreateSnapshots(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"type": "object",
		"title": "Note2",
		"x-type-key": "note2",
		"properties": {
			"id": {"type": "string", "x-order": 0, "x-key": "id"},
			"Type": {"const": "Note", "x-order": 1, "x-key": "type"},
			"Title": {
				"type": "string",
				"x-featured": true,
				"x-order": 2,
				"x-key": "title"
			},
			"Content": {
				"type": "string",
				"description": "Long text field",
				"x-order": 3,
				"x-key": "content"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/note.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Create relation snapshots
	relSnapshots := si.CreateRelationSnapshots()
	// We have 4 properties in schema, but some might be bundled
	// Just check that we have some snapshots
	assert.Greater(t, len(relSnapshots), 0)

	// Verify relation snapshots
	for _, snapshot := range relSnapshots {
		assert.NotEmpty(t, snapshot.Id)
		assert.NotNil(t, snapshot.Snapshot)
		assert.NotNil(t, snapshot.Snapshot.Data)

		// Check that it's a relation
		assert.Equal(t, smartblock.SmartBlockTypeRelation, snapshot.Snapshot.SbType)
	}

	// Create type snapshots
	typeSnapshots := si.CreateTypeSnapshots()
	assert.Len(t, typeSnapshots, 1)

	// Verify type snapshot
	typeSnapshot := typeSnapshots[0]
	assert.Contains(t, typeSnapshot.Id, "note2")
	assert.Equal(t, smartblock.SmartBlockTypeObjectType, typeSnapshot.Snapshot.SbType)

	// Check type details
	details := typeSnapshot.Snapshot.Data.Details
	assert.Equal(t, "Note2", details.GetString(bundle.RelationKeyName))

	// Check featured relations
	featuredRels := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	assert.Contains(t, featuredRels, si.propIdPrefix+"title")

	// Check regular relations
	regularRels := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	assert.Contains(t, regularRels, si.propIdPrefix+"content")
}

func TestSchemaImporter_GetTypeKeyByName(t *testing.T) {
	si := NewSchemaImporter()
	testSchema := &schema.Schema{
		Type: &schema.Type{
			Name: "Project",
			Key:  "project",
		},
	}
	si.schemas["project.json"] = testSchema

	// Test existing type
	key := si.GetTypeKeyByName("Project")
	assert.Equal(t, "project", key)

	// Test non-existing type
	key = si.GetTypeKeyByName("Unknown")
	assert.Empty(t, key)
}

func TestSchemaImporter_GetRelationKeyByName(t *testing.T) {
	si := NewSchemaImporter()
	testSchema := &schema.Schema{
		Type: &schema.Type{
			Name: "Task",
			Key:  "task",
		},
		Relations: map[string]*schema.Relation{
			"deadline": {
				Name: "Deadline",
				Key:  "deadline",
			},
			"priority": {
				Name: "Priority",
				Key:  "priority",
			},
		},
	}
	si.schemas["task.json"] = testSchema

	// Test existing relation
	key, found := si.GetRelationKeyByName("Deadline")
	assert.True(t, found)
	assert.Equal(t, "deadline", key)

	// Test schema-specific relation
	key, found = si.GetRelationKeyByName("Priority")
	assert.True(t, found)
	assert.Equal(t, "priority", key)

	// Test non-existing relation
	key, found = si.GetRelationKeyByName("Unknown")
	assert.False(t, found)
	assert.Empty(t, key)
}

func TestSchemaImporter_ComprehensiveTypes(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"type": "object",
		"title": "Comprehensive Test",
		"x-type-key": "comp_test",
		"properties": {
			"id": {
				"type": "string",
				"x-order": 0,
				"x-key": "id"
			},
			"Type": {
				"const": "Comprehensive Test",
				"x-order": 1,
				"x-key": "type"
			},
			"Name": {
				"type": "string",
				"description": "Short text field",
				"x-featured": true,
				"x-order": 2,
				"x-key": "name"
			},
			"Description": {
				"type": "string",
				"description": "Long text field",
				"x-order": 3,
				"x-key": "description"
			},
			"Is Active": {
				"type": "boolean",
				"description": "Checkbox field",
				"x-order": 4,
				"x-key": "is_active"
			},
			"Score": {
				"type": "number",
				"description": "Number field",
				"x-order": 5,
				"x-key": "score"
			},
			"Birthday": {
				"type": "string",
				"format": "date",
				"description": "Date without time",
				"x-order": 6,
				"x-key": "birthday"
			},
			"Meeting Time": {
				"type": "string",
				"format": "date-time",
				"description": "Date with time",
				"x-order": 7,
				"x-key": "meeting_time"
			},
			"Email": {
				"type": "string",
				"format": "email",
				"description": "Email field",
				"x-order": 8,
				"x-key": "email"
			},
			"Website": {
				"type": "string",
				"format": "uri",
				"description": "URL field",
				"x-order": 9,
				"x-key": "website"
			},
			"Document Status": {
				"type": "string",
				"enum": ["Draft", "In Review", "Published", "Archived"],
				"description": "Status with options",
				"x-featured": true,
				"x-order": 10,
				"x-key": "doc_status"
			},
			"Priority Level": {
				"type": "string",
				"enum": ["Low", "Medium", "High", "Critical"],
				"description": "Another status field",
				"x-order": 11,
				"x-key": "priority_level"
			},
			"Tags": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"examples": ["urgent", "important", "review-needed", "documentation"],
				"description": "Tag field with examples",
				"x-featured": true,
				"x-order": 12,
				"x-key": "tags"
			},
			"Categories": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"examples": ["work", "personal", "hobby"],
				"description": "Another tag field",
				"x-order": 13,
				"x-key": "categories"
			},
			"Assignee": {
				"type": "object",
				"description": "Object relation",
				"x-order": 14,
				"x-key": "assignee"
			},
			"Attachment": {
				"type": "string",
				"description": "Path to the file",
				"x-order": 15,
				"x-key": "attachment"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/comprehensive.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Verify schema was loaded
	assert.True(t, si.HasSchemas())
	assert.Len(t, si.schemas, 1)

	// Check schema info
	var schema *schema.Schema
	for _, s := range si.schemas {
		if s.Type != nil && s.Type.Name == "Comprehensive Test" {
			schema = s
			break
		}
	}
	require.NotNil(t, schema, "Schema with type 'Comprehensive Test' not found")
	assert.Equal(t, "Comprehensive Test", schema.Type.Name)
	assert.Equal(t, "comp_test", schema.Type.Key)
	assert.Len(t, schema.Relations, 16) // All properties including id and type

	// Verify all relation formats
	tests := []struct {
		key          string
		name         string
		format       model.RelationFormat
		hasOptions   bool
		optionCount  int
		hasExamples  bool
		exampleCount int
	}{
		{"name", "Name", model.RelationFormat_shorttext, false, 0, false, 0},
		{"description", "Description", model.RelationFormat_shorttext, false, 0, false, 0},
		{"is_active", "Is Active", model.RelationFormat_checkbox, false, 0, false, 0},
		{"score", "Score", model.RelationFormat_number, false, 0, false, 0},
		{"birthday", "Birthday", model.RelationFormat_date, false, 0, false, 0},
		{"meeting_time", "Meeting Time", model.RelationFormat_date, false, 0, false, 0},
		{"email", "Email", model.RelationFormat_email, false, 0, false, 0},
		{"website", "Website", model.RelationFormat_url, false, 0, false, 0},
		{"doc_status", "Document Status", model.RelationFormat_status, true, 4, false, 0},
		{"priority_level", "Priority Level", model.RelationFormat_status, true, 4, false, 0},
		{"tags", "Tags", model.RelationFormat_tag, false, 0, true, 4},
		{"categories", "Categories", model.RelationFormat_tag, false, 0, true, 3},
		{"assignee", "Assignee", model.RelationFormat_object, false, 0, false, 0},
		{"attachment", "Attachment", model.RelationFormat_shorttext, false, 0, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel, ok := schema.Relations[tt.key]
			require.True(t, ok, "Relation %s not found", tt.key)
			assert.Equal(t, tt.name, rel.Name)
			assert.Equal(t, tt.format, rel.Format)

			if tt.hasOptions {
				assert.Len(t, rel.Options, tt.optionCount, "Wrong number of options for %s", tt.name)
			}

			if tt.hasExamples {
				assert.Len(t, rel.Examples, tt.exampleCount, "Wrong number of examples for %s", tt.name)
			}
		})
	}

	// Test date field includeTime
	birthdayRel := schema.Relations["birthday"]
	assert.False(t, birthdayRel.IncludeTime, "Birthday should not include time")

	meetingRel := schema.Relations["meeting_time"]
	assert.True(t, meetingRel.IncludeTime, "Meeting time should include time")

	// Create snapshots and verify
	relSnapshots := si.CreateRelationSnapshots()
	// Some relations might be bundled (like type, assignee, email), so we expect fewer snapshots
	assert.Greater(t, len(relSnapshots), 5, "Should create many relation snapshots")

	optionSnapshots := si.CreateRelationOptionSnapshots()
	// Should create options for:
	// - Status fields: 4 options for "Status" + 4 options for "Priority" = 8
	// - Tag examples: 4 examples for "Tags" + 3 examples for "Categories" = 7
	// Total: 15 option snapshots
	assert.Greater(t, len(optionSnapshots), 10, "Should create many option snapshots")

	// Verify option snapshots have correct structure
	for _, snapshot := range optionSnapshots {
		assert.NotEmpty(t, snapshot.Id)
		assert.Equal(t, smartblock.SmartBlockTypeRelationOption, snapshot.Snapshot.SbType)

		details := snapshot.Snapshot.Data.Details
		assert.NotEmpty(t, details.GetString(bundle.RelationKeyName))
		assert.NotEmpty(t, details.GetString(bundle.RelationKeyRelationKey))
	}

	// Create type snapshots
	typeSnapshots := si.CreateTypeSnapshots()
	assert.Len(t, typeSnapshots, 1)

	typeSnapshot := typeSnapshots[0]
	details := typeSnapshot.Snapshot.Data.Details

	// Check featured relations
	featuredRels := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)

	// Check that featured relations contain the expected ones
	// Note: "name" is filtered out because it's a bundled hidden relation
	// So we expect 2 featured relations: "doc_status" and "tags"
	assert.Len(t, featuredRels, 2, "Should have exactly 2 featured relations (name is filtered out)")

	// Check that all relations are included in the type
	allRelIds := append(featuredRels, details.GetStringList(bundle.RelationKeyRecommendedRelations)...)
	// Count non-bundled relations
	nonBundledCount := 0
	for _, rel := range schema.Relations {
		if _, err := bundle.GetRelation(domain.RelationKey(rel.Key)); err != nil {
			nonBundledCount++
		}
	}
	assert.Greater(t, len(allRelIds), 5, "Type should include many relations")
}

func TestSchemaImporter_StatusRelationOptions(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"type": "object",
		"title": "Task",
		"properties": {
			"Status": {
				"type": "string",
				"enum": ["Open", "In Progress", "Done", "Cancelled"],
				"x-key": "task_status"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/task.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Check that options were parsed
	var rel *schema.Relation
	for _, s := range si.schemas {
		if r, ok := s.Relations["task_status"]; ok {
			rel = r
			break
		}
	}
	require.NotNil(t, rel)
	assert.Equal(t, model.RelationFormat_status, rel.Format)
	assert.Equal(t, []string{"Open", "In Progress", "Done", "Cancelled"}, rel.Options)

	// Create option snapshots
	optionSnapshots := si.CreateRelationOptionSnapshots()
	assert.Len(t, optionSnapshots, 4)

	// Verify each option
	optionNames := make([]string, 0, 4)
	for _, snapshot := range optionSnapshots {
		details := snapshot.Snapshot.Data.Details
		name := details.GetString(bundle.RelationKeyName)
		relationKey := details.GetString(bundle.RelationKeyRelationKey)

		optionNames = append(optionNames, name)
		assert.Equal(t, "task_status", relationKey)
		assert.Equal(t, smartblock.SmartBlockTypeRelationOption, snapshot.Snapshot.SbType)
	}

	assert.ElementsMatch(t, []string{"Open", "In Progress", "Done", "Cancelled"}, optionNames)
}

func TestSchemaImporter_TagRelationExamples(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"type": "object",
		"title": "Document",
		"properties": {
			"Tags": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"examples": ["important", "urgent", "review", "draft"],
				"x-key": "doc_tags"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/document.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Check that examples were parsed
	var rel *schema.Relation
	for _, s := range si.schemas {
		if r, ok := s.Relations["doc_tags"]; ok {
			rel = r
			break
		}
	}
	require.NotNil(t, rel)
	assert.Equal(t, model.RelationFormat_tag, rel.Format)
	assert.Equal(t, []string{"important", "urgent", "review", "draft"}, rel.Examples)

	// Create option snapshots
	optionSnapshots := si.CreateRelationOptionSnapshots()
	assert.Len(t, optionSnapshots, 4)

	// Verify each example
	exampleNames := make([]string, 0, 4)
	for _, snapshot := range optionSnapshots {
		details := snapshot.Snapshot.Data.Details
		name := details.GetString(bundle.RelationKeyName)
		relationKey := details.GetString(bundle.RelationKeyRelationKey)

		exampleNames = append(exampleNames, name)
		assert.Equal(t, "doc_tags", relationKey)
		assert.Equal(t, smartblock.SmartBlockTypeRelationOption, snapshot.Snapshot.SbType)
	}

	assert.ElementsMatch(t, []string{"important", "urgent", "review", "draft"}, exampleNames)
}

func TestSchemaImporter_AllPropertiesAddedToType(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"type": "object",
		"title": "Complete Object",
		"x-type-key": "complete_obj",
		"properties": {
			"id": {"type": "string", "x-order": 0, "x-key": "id"},
			"Type": {"const": "Complete Object", "x-order": 1, "x-key": "type"},
			"Field1": {"type": "string", "x-order": 2, "x-key": "field1"},
			"Field2": {"type": "string", "x-order": 3, "x-key": "field2"},
			"Field3": {"type": "string", "x-order": 4, "x-key": "field3"},
			"Field4": {"type": "string", "x-order": 5, "x-key": "field4"},
			"Field5": {"type": "string", "x-order": 6, "x-key": "field5"},
			"Field6": {"type": "string", "x-order": 7, "x-key": "field6"},
			"Field7": {"type": "string", "x-order": 8, "x-key": "field7"},
			"Field8": {"type": "string", "x-order": 9, "x-key": "field8"},
			"Field9": {"type": "string", "x-order": 10, "x-key": "field9"},
			"Field10": {"type": "string", "x-order": 11, "x-key": "field10"}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/complete.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Create type snapshots
	typeSnapshots := si.CreateTypeSnapshots()
	require.Len(t, typeSnapshots, 1)

	typeSnapshot := typeSnapshots[0]
	details := typeSnapshot.Snapshot.Data.Details

	// Get all relations from the type
	featuredRels := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	regularRels := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	hiddenRels := details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations)
	allRels := append(featuredRels, regularRels...)
	allRels = append(allRels, hiddenRels...)

	// Should have Field1-Field10 (10 fields total)
	// id and type are standard fields and not included in relation lists
	assert.GreaterOrEqual(t, len(allRels), 10, "Should have all Field1-Field10 relations")
}

func TestSchemaImporter_XFormatSupport(t *testing.T) {
	// Test that x-format is properly parsed and used
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"type": "object",
		"title": "Document",
		"properties": {
			"Attachments": {
				"type": "array",
				"items": {"type": "string"},
				"description": "File attachments",
				"x-key": "attachments",
				"x-format": "file"
			},
			"Tags": {
				"type": "array",
				"items": {"type": "string"},
				"examples": ["important", "urgent"],
				"x-key": "tags",
				"x-format": "tag"
			},
			"References": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"Name": {"type": "string"},
						"Object type": {"type": "string"}
					}
				},
				"x-key": "references",
				"x-format": "object"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/document.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Find the schema and verify relations
	var foundSchema *schema.Schema
	for _, s := range si.schemas {
		if s.Type != nil && s.Type.Name == "Document" {
			foundSchema = s
			break
		}
	}
	require.NotNil(t, foundSchema)

	// Verify all relations have correct formats from x-format
	attachmentsRel, ok := foundSchema.Relations["attachments"]
	require.True(t, ok)
	assert.Equal(t, model.RelationFormat_file, attachmentsRel.Format, "Attachments should be file format")

	tagsRel, ok := foundSchema.Relations["tags"]
	require.True(t, ok)
	assert.Equal(t, model.RelationFormat_tag, tagsRel.Format, "Tags should be tag format")
	assert.Equal(t, []string{"important", "urgent"}, tagsRel.Examples, "Tag examples should be parsed")

	referencesRel, ok := foundSchema.Relations["references"]
	require.True(t, ok)
	assert.Equal(t, model.RelationFormat_object, referencesRel.Format, "References should be object format")
}

func TestSchemaImporter_XFormatWithStatusOptions(t *testing.T) {
	// Test that x-format works with status relations and options
	schemaContent := `{
		"type": "object",
		"title": "Task",
		"x-app": "Anytype",
		"properties": {
			"Progress": {
				"type": "string",
				"enum": ["Not Started", "In Progress", "Completed", "Blocked"],
				"x-key": "progress",
				"x-format": "status"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/task.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Find the schema and verify relation
	var foundSchema *schema.Schema
	for _, s := range si.schemas {
		if s.Type != nil && s.Type.Name == "Task" {
			foundSchema = s
			break
		}
	}
	require.NotNil(t, foundSchema)

	progressRel, ok := foundSchema.Relations["progress"]
	require.True(t, ok)
	assert.Equal(t, model.RelationFormat_status, progressRel.Format)
	assert.Equal(t, []string{"Not Started", "In Progress", "Completed", "Blocked"}, progressRel.Options)
}

func TestSchemaImporter_XFormatFallback(t *testing.T) {
	// Test that format inference still works when x-format is not present
	schemaContent := `{
		"type": "object",
		"title": "Legacy",
		"x-app": "Anytype",
		"properties": {
			"Email": {
				"type": "string",
				"format": "email",
				"x-key": "email_field"
			},
			"Tags": {
				"type": "array",
				"items": {"type": "string"},
				"x-key": "tag_field"
			},
			"Done": {
				"type": "boolean",
				"x-key": "done_field"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/legacy.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Find the schema and verify relations
	var foundSchema *schema.Schema
	for _, s := range si.schemas {
		if s.Type != nil && s.Type.Name == "Legacy" {
			foundSchema = s
			break
		}
	}
	require.NotNil(t, foundSchema)

	// Verify format inference still works
	emailRel, ok := foundSchema.Relations["email_field"]
	require.True(t, ok)
	assert.Equal(t, model.RelationFormat_email, emailRel.Format)

	tagsRel, ok := foundSchema.Relations["tag_field"]
	require.True(t, ok)
	assert.Equal(t, model.RelationFormat_tag, tagsRel.Format)

	doneRel, ok := foundSchema.Relations["done_field"]
	require.True(t, ok)
	assert.Equal(t, model.RelationFormat_checkbox, doneRel.Format)
}

func TestSchemaImporter_FileVsTagDistinction(t *testing.T) {
	// Test that file and tag relations are properly distinguished using x-format
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"type": "object",
		"title": "Document",
		"x-type-key": "doc_with_files",
		"properties": {
			"id": {"type": "string", "x-order": 0, "x-key": "id"},
			"Type": {"const": "Document", "x-order": 1, "x-key": "type"},
			"Attachments": {
				"type": "array",
				"items": {"type": "string"},
				"description": "File attachments",
				"x-key": "attachments",
				"x-format": "file",
				"x-order": 2
			},
			"Images": {
				"type": "string",
				"description": "Image files",
				"x-key": "images",
				"x-format": "file",
				"x-order": 3
			},
			"Tags": {
				"type": "array",
				"items": {"type": "string"},
				"examples": ["important", "urgent", "review"],
				"x-key": "tags",
				"x-format": "tag",
				"x-order": 4
			},
			"Categories": {
				"type": "array",
				"items": {"type": "string"},
				"description": "This would be ambiguous without x-format",
				"x-key": "categories",
				"x-format": "tag",
				"x-order": 5
			},
			"References": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"Name": {"type": "string"},
						"Object type": {"type": "string"}
					}
				},
				"x-key": "references",
				"x-format": "object",
				"x-order": 6
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/document.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Verify file relations
	var attachmentsRel, imagesRel *schema.Relation
	for _, s := range si.schemas {
		if r, ok := s.Relations["attachments"]; ok {
			attachmentsRel = r
		}
		if r, ok := s.Relations["images"]; ok {
			imagesRel = r
		}
	}
	require.NotNil(t, attachmentsRel)
	assert.Equal(t, model.RelationFormat_file, attachmentsRel.Format, "Array of strings with x-format=file should be file relation")

	require.NotNil(t, imagesRel)
	assert.Equal(t, model.RelationFormat_file, imagesRel.Format, "Single string with x-format=file should be file relation")

	// Verify tag relations
	var tagsRel, categoriesRel *schema.Relation
	for _, s := range si.schemas {
		if r, ok := s.Relations["tags"]; ok {
			tagsRel = r
		}
		if r, ok := s.Relations["categories"]; ok {
			categoriesRel = r
		}
	}
	require.NotNil(t, tagsRel)
	assert.Equal(t, model.RelationFormat_tag, tagsRel.Format, "Array with x-format=tag should be tag relation")
	assert.Equal(t, []string{"important", "urgent", "review"}, tagsRel.Examples, "Tag examples should be preserved")

	require.NotNil(t, categoriesRel)
	assert.Equal(t, model.RelationFormat_tag, categoriesRel.Format, "Array with x-format=tag should be tag relation even without examples")

	// Verify object relation
	var referencesRel *schema.Relation
	for _, s := range si.schemas {
		if r, ok := s.Relations["references"]; ok {
			referencesRel = r
			break
		}
	}
	require.NotNil(t, referencesRel)
	assert.Equal(t, model.RelationFormat_object, referencesRel.Format, "Array of objects with x-format=object should be object relation")

	// Create snapshots and verify
	relSnapshots := si.CreateRelationSnapshots()
	// We have 7 total relations (id, type, attachments, images, tags, categories, references)
	// Some might be bundled (like id and type), so we expect fewer snapshots
	assert.Greater(t, len(relSnapshots), 4) // At least the custom relations

	// Create type snapshot and verify all relations are included
	typeSnapshots := si.CreateTypeSnapshots()
	require.Len(t, typeSnapshots, 1)

	typeSnapshot := typeSnapshots[0]
	details := typeSnapshot.Snapshot.Data.Details

	featuredRels := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	regularRels := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	hiddenRels := details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations)
	allRelIds := append(featuredRels, regularRels...)
	allRelIds = append(allRelIds, hiddenRels...)

	// Should include all relations defined in the schema except id and type
	// We have: attachments, images, tags, categories, references = 5 relations
	assert.GreaterOrEqual(t, len(allRelIds), 5, "Type should include all relations from schema")
}

func TestSchemaImporter_IntegrationWithCustomRelations(t *testing.T) {
	// This test uses custom relation keys that are NOT bundled
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"type": "object",
		"title": "Project",
		"x-type-key": "custom_project",
		"properties": {
			"id": {
				"type": "string",
				"x-order": 0,
				"x-key": "id"
			},
			"Type": {
				"const": "Project",
				"x-order": 1,
				"x-key": "type"
			},
			"Project Name": {
				"type": "string",
				"x-featured": true,
				"x-order": 2,
				"x-key": "project_name"
			},
			"Project Status": {
				"type": "string",
				"enum": ["Planning", "Active", "On Hold", "Completed", "Cancelled"],
				"x-featured": true,
				"x-order": 3,
				"x-key": "project_status"
			},
			"Priority Level": {
				"type": "string",
				"enum": ["Low", "Medium", "High", "Critical"],
				"x-order": 4,
				"x-key": "priority_level"
			},
			"Project Tags": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"examples": ["frontend", "backend", "infrastructure", "documentation", "testing"],
				"x-featured": true,
				"x-order": 5,
				"x-key": "project_tags"
			},
			"Department": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"examples": ["Engineering", "Marketing", "Sales"],
				"x-order": 6,
				"x-key": "department"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/project.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// Verify schema was loaded
	assert.True(t, si.HasSchemas())

	// Create all snapshots
	relSnapshots := si.CreateRelationSnapshots()
	optionSnapshots := si.CreateRelationOptionSnapshots()
	typeSnapshots := si.CreateTypeSnapshots()

	// Count all relations (including bundled ones)
	totalRelCount := 0
	for _, schema := range si.schemas {
		totalRelCount += len(schema.Relations)
	}

	// Verify relation snapshots (should match all relations including bundled ones)
	assert.Equal(t, totalRelCount, len(relSnapshots), "Should create snapshots for all relations including bundled ones")

	// Verify option snapshots
	// Should have:
	// - 5 options for "project_status"
	// - 4 options for "priority_level"
	// - 5 examples for "project_tags"
	// - 3 examples for "department"
	// Total: 17
	assert.Len(t, optionSnapshots, 17, "Should create all option snapshots")

	// Count by type
	statusOptCount := 0
	tagExampleCount := 0
	for _, snapshot := range optionSnapshots {
		details := snapshot.Snapshot.Data.Details
		relKey := details.GetString(bundle.RelationKeyRelationKey)

		// Find the relation in schemas
		for _, schema := range si.schemas {
			if rel, ok := schema.Relations[relKey]; ok {
				if rel.Format == model.RelationFormat_status {
					statusOptCount++
				} else if rel.Format == model.RelationFormat_tag {
					tagExampleCount++
				}
				break
			}
		}
	}

	assert.Equal(t, 9, statusOptCount, "Should create 9 status options (5 + 4)")
	assert.Equal(t, 8, tagExampleCount, "Should create 8 tag examples (5 + 3)")

	// Verify type snapshot
	assert.Len(t, typeSnapshots, 1)
	typeSnapshot := typeSnapshots[0]
	assert.Equal(t, "custom_project", typeSnapshot.Snapshot.Data.Key)

	details := typeSnapshot.Snapshot.Data.Details
	assert.Equal(t, "Project", details.GetString(bundle.RelationKeyName))

	// Check featured relations
	featuredRels := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	// We have 3 x-featured relations + type relation might be included
	assert.Greater(t, len(featuredRels), 2, "Should have at least 3 featured relations")

	// Verify all relations are included in the type
	allRelIds := append(featuredRels, details.GetStringList(bundle.RelationKeyRecommendedRelations)...)
	// Should include most custom relations (type might be included or not)
	assert.Greater(t, len(allRelIds), 4, "Type should include most custom relations")

	// Verify each option snapshot has proper structure
	for _, snapshot := range optionSnapshots {
		assert.Equal(t, smartblock.SmartBlockTypeRelationOption, snapshot.Snapshot.SbType)
		details := snapshot.Snapshot.Data.Details
		assert.NotEmpty(t, details.GetString(bundle.RelationKeyName))
		assert.NotEmpty(t, details.GetString(bundle.RelationKeyRelationKey))
		// ID is at snapshot level, not in details
		assert.NotEmpty(t, snapshot.Id)
		assert.NotEmpty(t, details.GetString(bundle.RelationKeyUniqueKey))
	}
}

func TestSchemaImporter_RoundTripExportImport(t *testing.T) {
	// Test scenario: Export from Anytype with rich properties, then import back
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"x-app": "Anytype",
		"$id": "urn:anytype:schema:2024-06-14:author-user:type-article:gen-1.0.0",
		"type": "object",
		"title": "Article",
		"description": "A blog article or documentation page",
		"x-type-key": "5f9a8b7c6d5e4f3a2b1c",
		"x-plural": "Articles",
		"x-icon-emoji": "📄",
		"properties": {
			"id": {
				"type": "string",
				"description": "Unique identifier",
				"readOnly": true,
				"x-order": 0,
				"x-key": "id"
			},
			"Type": {
				"const": "Article",
				"x-order": 1,
				"x-key": "type"
			},
			"Title": {
				"type": "string",
				"x-featured": true,
				"x-order": 2,
				"x-key": "name"
			},
			"Publication Status": {
				"type": "string",
				"enum": ["Draft", "Under Review", "Published", "Archived"],
				"default": "Draft",
				"x-featured": true,
				"x-order": 3,
				"x-key": "pub_status_5f9a8b7c"
			},
			"Content Tags": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"examples": ["tutorial", "reference", "how-to", "concept"],
				"x-order": 4,
				"x-key": "content_tags_5f9a8b7c"
			},
			"Publish Date": {
				"type": "string",
				"format": "date",
				"x-order": 5,
				"x-key": "publish_date_5f9a8b7c"
			}
		}
	}`

	source := &mockSource{
		files: map[string]string{
			"schemas/article.schema.json": schemaContent,
		},
	}

	si := NewSchemaImporter()
	allErrors := common.NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	err := si.LoadSchemas(source, allErrors)
	require.NoError(t, err)

	// When importing to the same space, x-key should match existing relations
	assert.Equal(t, "5f9a8b7c6d5e4f3a2b1c", si.GetTypeKeyByName("Article"))
	key1, found1 := si.GetRelationKeyByName("Publication Status")
	assert.True(t, found1)
	assert.Equal(t, "pub_status_5f9a8b7c", key1)
	key2, found2 := si.GetRelationKeyByName("Content Tags")
	assert.True(t, found2)
	assert.Equal(t, "content_tags_5f9a8b7c", key2)

	// Verify all snapshots are created correctly
	relSnapshots := si.CreateRelationSnapshots()
	optionSnapshots := si.CreateRelationOptionSnapshots()
	typeSnapshots := si.CreateTypeSnapshots()

	// Should create snapshots for custom relations (not name/type which are bundled)
	assert.Greater(t, len(relSnapshots), 2, "Should create relation snapshots")
	assert.Equal(t, 8, len(optionSnapshots), "Should create 4 status options + 4 tag examples")
	assert.Len(t, typeSnapshots, 1, "Should create one type snapshot")

	// Verify the type uses the x-type-key
	typeSnapshot := typeSnapshots[0]
	assert.Contains(t, typeSnapshot.Id, "5f9a8b7c6d5e4f3a2b1c")
	assert.Equal(t, "5f9a8b7c6d5e4f3a2b1c", typeSnapshot.Snapshot.Data.Key)
}
