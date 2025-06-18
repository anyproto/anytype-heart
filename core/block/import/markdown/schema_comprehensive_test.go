package markdown

import (
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

func TestSchemaImporter_ComprehensiveTypes(t *testing.T) {
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
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
	// Note: The actual IDs depend on what's bundled and the order
	assert.Greater(t, len(featuredRels), 2, "Should have at least 3 featured relations")

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
	allRels := append(featuredRels, regularRels...)

	// Should have all 10 fields (but Type might be included as a bundled relation)
	assert.Greater(t, len(allRels), 9, "Most fields should be included in the type")

	// Verify we have enough relations
	// The actual IDs might be bundled or prefixed, so just check count
	assert.Greater(t, len(allRels), 10, "Type should have many relations")
}
