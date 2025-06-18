package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSchemaImporter_IntegrationWithCustomRelations(t *testing.T) {
	// This test uses custom relation keys that are NOT bundled
	schemaContent := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
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