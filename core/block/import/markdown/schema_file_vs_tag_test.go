package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSchemaImporter_FileVsTagDistinction(t *testing.T) {
	// Test that file and tag relations are properly distinguished using x-format
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
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
				"x-format": "RelationFormat_file",
				"x-order": 2
			},
			"Images": {
				"type": "string",
				"description": "Image files",
				"x-key": "images",
				"x-format": "RelationFormat_file",
				"x-order": 3
			},
			"Tags": {
				"type": "array",
				"items": {"type": "string"},
				"examples": ["important", "urgent", "review"],
				"x-key": "tags",
				"x-format": "RelationFormat_tag",
				"x-order": 4
			},
			"Categories": {
				"type": "array",
				"items": {"type": "string"},
				"description": "This would be ambiguous without x-format",
				"x-key": "categories",
				"x-format": "RelationFormat_tag",
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
				"x-format": "RelationFormat_object",
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
	attachmentsRel := si.relations["attachments"]
	require.NotNil(t, attachmentsRel)
	assert.Equal(t, model.RelationFormat_file, attachmentsRel.Format, "Array of strings with x-format=file should be file relation")
	
	imagesRel := si.relations["images"]
	require.NotNil(t, imagesRel)
	assert.Equal(t, model.RelationFormat_file, imagesRel.Format, "Single string with x-format=file should be file relation")
	
	// Verify tag relations
	tagsRel := si.relations["tags"]
	require.NotNil(t, tagsRel)
	assert.Equal(t, model.RelationFormat_tag, tagsRel.Format, "Array with x-format=tag should be tag relation")
	assert.Equal(t, []string{"important", "urgent", "review"}, tagsRel.Examples, "Tag examples should be preserved")
	
	categoriesRel := si.relations["categories"]
	require.NotNil(t, categoriesRel)
	assert.Equal(t, model.RelationFormat_tag, categoriesRel.Format, "Array with x-format=tag should be tag relation even without examples")
	
	// Verify object relation
	referencesRel := si.relations["references"]
	require.NotNil(t, referencesRel)
	assert.Equal(t, model.RelationFormat_object, referencesRel.Format, "Array of objects with x-format=object should be object relation")
	
	// Create snapshots and verify
	relSnapshots := si.CreateRelationSnapshots()
	assert.Len(t, relSnapshots, 5) // All non-bundled relations
	
	// Create type snapshot and verify all relations are included
	typeSnapshots := si.CreateTypeSnapshots()
	require.Len(t, typeSnapshots, 1)
	
	typeSnapshot := typeSnapshots[0]
	details := typeSnapshot.Snapshot.Data.Details
	
	allRelIds := append(
		details.GetStringList("recommendedFeaturedRelations"),
		details.GetStringList("recommendedRelations")...,
	)
	
	// Should include all custom relations (5) plus type
	assert.Len(t, allRelIds, 6, "Type should include all custom relations plus type")
}