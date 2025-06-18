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
	
	allRelIds := append(
		details.GetStringList("recommendedFeaturedRelations"),
		details.GetStringList("recommendedRelations")...,
	)
	
	// Should include all relations defined in the schema
	assert.Greater(t, len(allRelIds), 5, "Type should include all relations from schema")
}