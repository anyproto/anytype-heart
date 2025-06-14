package markdown

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

func (m *mockSource) Close() {}
func (m *mockSource) IsRootFile(string) bool { return false }

func TestSchemaImporter_LoadSchemas(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
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
	schema, ok := si.schemas["Task"]
	require.True(t, ok)
	assert.Equal(t, "Task", schema.TypeName)
	assert.Equal(t, "task", schema.TypeKey)
	assert.Len(t, schema.Relations, 5) // Excluding id
	
	// Check relations
	assert.Len(t, si.relations, 5)
	
	// Verify relation details
	nameRel, ok := si.relations["name"]
	require.True(t, ok)
	assert.Equal(t, "Name", nameRel.Name)
	assert.Equal(t, "name", nameRel.Key)
	assert.Equal(t, model.RelationFormat_shorttext, nameRel.Format)
	assert.True(t, nameRel.Featured)
	assert.Equal(t, 2, nameRel.Order)
	
	statusRel, ok := si.relations["status"]
	require.True(t, ok)
	assert.Equal(t, "Status", statusRel.Name)
	assert.Equal(t, model.RelationFormat_status, statusRel.Format)
	assert.True(t, statusRel.Featured)
	
	dueDateRel, ok := si.relations["duedate"]
	require.True(t, ok)
	assert.Equal(t, "Due Date", dueDateRel.Name)
	assert.Equal(t, model.RelationFormat_date, dueDateRel.Format)
	assert.False(t, dueDateRel.IncludeTime)
}

func TestSchemaImporter_CreateSnapshots(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Note",
		"x-type-key": "note",
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
	assert.Len(t, relSnapshots, 2) // title and content (type is bundled)
	
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
	assert.Contains(t, typeSnapshot.Id, "note")
	assert.Equal(t, smartblock.SmartBlockTypeObjectType, typeSnapshot.Snapshot.SbType)
	
	// Check type details
	details := typeSnapshot.Snapshot.Data.Details
	assert.Equal(t, "Note", details.GetString(bundle.RelationKeyName))
	
	// Check featured relations
	featuredRels := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	assert.Contains(t, featuredRels, propIdPrefix+"title")
	
	// Check regular relations
	regularRels := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	assert.Contains(t, regularRels, propIdPrefix+"content")
}

func TestSchemaImporter_GetTypeKeyByName(t *testing.T) {
	si := NewSchemaImporter()
	si.schemas["Project"] = &SchemaInfo{
		TypeName: "Project",
		TypeKey:  "project",
	}
	
	// Test existing type
	key := si.GetTypeKeyByName("Project")
	assert.Equal(t, "project", key)
	
	// Test non-existing type
	key = si.GetTypeKeyByName("Unknown")
	assert.Empty(t, key)
}

func TestSchemaImporter_GetRelationKeyByName(t *testing.T) {
	si := NewSchemaImporter()
	si.relations["deadline"] = &RelationInfo{
		Name: "Deadline",
		Key:  "deadline",
	}
	
	// Also add to a schema
	si.schemas["Task"] = &SchemaInfo{
		TypeName: "Task",
		Relations: map[string]*RelationInfo{
			"priority": {
				Name: "Priority",
				Key:  "priority",
			},
		},
	}
	
	// Test global relation
	key := si.GetRelationKeyByName("Deadline")
	assert.Equal(t, "deadline", key)
	
	// Test schema-specific relation
	key = si.GetRelationKeyByName("Priority")
	assert.Equal(t, "priority", key)
	
	// Test non-existing relation
	key = si.GetRelationKeyByName("Unknown")
	assert.Empty(t, key)
}

func TestSchemaImporter_ParseRelationFormats(t *testing.T) {
	tests := []struct {
		name     string
		property map[string]interface{}
		expected model.RelationFormat
		includeTime bool
	}{
		{
			name: "boolean to checkbox",
			property: map[string]interface{}{
				"type": "boolean",
				"x-key": "done",
			},
			expected: model.RelationFormat_checkbox,
		},
		{
			name: "number format",
			property: map[string]interface{}{
				"type": "number",
				"x-key": "score",
			},
			expected: model.RelationFormat_number,
		},
		{
			name: "date format",
			property: map[string]interface{}{
				"type": "string",
				"format": "date",
				"x-key": "birthday",
			},
			expected: model.RelationFormat_date,
			includeTime: false,
		},
		{
			name: "date-time format",
			property: map[string]interface{}{
				"type": "string",
				"format": "date-time",
				"x-key": "meeting",
			},
			expected: model.RelationFormat_date,
			includeTime: true,
		},
		{
			name: "email format",
			property: map[string]interface{}{
				"type": "string",
				"format": "email",
				"x-key": "email",
			},
			expected: model.RelationFormat_email,
		},
		{
			name: "url format",
			property: map[string]interface{}{
				"type": "string",
				"format": "uri",
				"x-key": "website",
			},
			expected: model.RelationFormat_url,
		},
		{
			name: "status with enum",
			property: map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"Open", "Closed"},
				"x-key": "status",
			},
			expected: model.RelationFormat_status,
		},
		{
			name: "array as tag",
			property: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"x-key": "tags",
			},
			expected: model.RelationFormat_tag,
		},
		{
			name: "object relation",
			property: map[string]interface{}{
				"type": "object",
				"x-key": "assignee",
			},
			expected: model.RelationFormat_object,
		},
		{
			name: "long text",
			property: map[string]interface{}{
				"type": "string",
				"description": "Long text field",
				"x-key": "description",
			},
			expected: model.RelationFormat_longtext,
		},
		{
			name: "file format",
			property: map[string]interface{}{
				"type": "string",
				"description": "Path to the file",
				"x-key": "attachment",
			},
			expected: model.RelationFormat_file,
		},
	}
	
	si := NewSchemaImporter()
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := si.parseRelationFromProperty(tt.name, tt.property)
			require.NotNil(t, rel)
			assert.Equal(t, tt.expected, rel.Format)
			if tt.expected == model.RelationFormat_date {
				assert.Equal(t, tt.includeTime, rel.IncludeTime)
			}
		})
	}
}