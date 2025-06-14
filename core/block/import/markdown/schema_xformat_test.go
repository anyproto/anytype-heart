package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
				"x-format": "RelationFormat_file"
			},
			"Tags": {
				"type": "array",
				"items": {"type": "string"},
				"examples": ["important", "urgent"],
				"x-key": "tags",
				"x-format": "RelationFormat_tag"
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
				"x-format": "RelationFormat_object"
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
	
	// Verify all relations have correct formats from x-format
	attachmentsRel := si.relations["attachments"]
	require.NotNil(t, attachmentsRel)
	assert.Equal(t, model.RelationFormat_file, attachmentsRel.Format, "Attachments should be file format")
	
	tagsRel := si.relations["tags"]
	require.NotNil(t, tagsRel)
	assert.Equal(t, model.RelationFormat_tag, tagsRel.Format, "Tags should be tag format")
	assert.Equal(t, []string{"important", "urgent"}, tagsRel.Examples, "Tag examples should be parsed")
	
	referencesRel := si.relations["references"]
	require.NotNil(t, referencesRel)
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
				"x-format": "RelationFormat_status"
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
	
	progressRel := si.relations["progress"]
	require.NotNil(t, progressRel)
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
	
	// Verify format inference still works
	emailRel := si.relations["email_field"]
	require.NotNil(t, emailRel)
	assert.Equal(t, model.RelationFormat_email, emailRel.Format)
	
	tagsRel := si.relations["tag_field"]
	require.NotNil(t, tagsRel)
	assert.Equal(t, model.RelationFormat_tag, tagsRel.Format)
	
	doneRel := si.relations["done_field"]
	require.NotNil(t, doneRel)
	assert.Equal(t, model.RelationFormat_checkbox, doneRel.Format)
}