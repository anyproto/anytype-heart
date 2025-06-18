package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

func TestSchemaImporter_XFormatSupport(t *testing.T) {
	// Test that x-format is properly parsed and used
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
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
