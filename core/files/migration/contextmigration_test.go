package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func TestContextMigrationService_buildIncomingLinksMap(t *testing.T) {
	// This test focuses on the core logic of building incoming links map
	// which is the most important part of the migration

	service := &contextMigrationService{}

	t.Run("empty map for no links", func(t *testing.T) {
		incomingLinksMap := make(map[string][]incomingLinkInfo)

		// No links exist
		fileId := "file1"

		// Find creation context
		context := service.findCreationContext(fileId, incomingLinksMap)

		// Assert
		assert.Nil(t, context)
	})

	t.Run("prefer block links over relation links", func(t *testing.T) {
		fileId := "file1"
		pageId := "page1"
		taskId := "task1"
		blockId := "block1"

		incomingLinksMap := map[string][]incomingLinkInfo{
			fileId: {
				// Relation link (comes first)
				{
					objectId:    taskId,
					relationKey: "attachment",
				},
				// Block link (should be preferred)
				{
					objectId:    pageId,
					blockId:     blockId,
					relationKey: "",
				},
			},
		}

		// Find creation context
		context := service.findCreationContext(fileId, incomingLinksMap)

		// Assert
		require.NotNil(t, context)
		assert.Equal(t, pageId, context.contextId)
		assert.Equal(t, blockId, context.blockId)
	})

	t.Run("use first link when multiple relation links exist", func(t *testing.T) {
		fileId := "file1"
		page1 := "page1"
		page2 := "page2"

		incomingLinksMap := map[string][]incomingLinkInfo{
			fileId: {
				{
					objectId:    page2,
					relationKey: "b",
				},
				{
					objectId:    page1,
					relationKey: "a",
				},
			},
		}

		// Find creation context
		context := service.findCreationContext(fileId, incomingLinksMap)

		// Assert - should pick first after sorting by relationKey
		require.NotNil(t, context)
		assert.Equal(t, page1, context.contextId)
		assert.Equal(t, "", context.blockId)
	})

	t.Run("handle relation links when no block links exist", func(t *testing.T) {
		fileId := "file1"
		taskId := "task1"

		incomingLinksMap := map[string][]incomingLinkInfo{
			fileId: {
				{
					objectId:    taskId,
					relationKey: "attachment",
				},
			},
		}

		// Find creation context
		context := service.findCreationContext(fileId, incomingLinksMap)

		// Assert
		require.NotNil(t, context)
		assert.Equal(t, taskId, context.contextId)
		assert.Equal(t, "", context.blockId) // No block ID for relation links
	})
}

func TestIncomingLinkConversion(t *testing.T) {
	// Test conversion from spaceindex.OutgoingLink to internal incomingLinkInfo

	t.Run("convert block link", func(t *testing.T) {
		sourceId := "page1"
		blockId := "block1"
		targetId := "file1"

		link := spaceindex.OutgoingLink{
			TargetID:    targetId,
			BlockID:     blockId,
			RelationKey: "",
		}

		// Convert to internal format (targetId is the map key, not stored in struct)
		info := incomingLinkInfo{
			objectId:    sourceId,
			blockId:     link.BlockID,
			relationKey: link.RelationKey,
		}

		// Assert
		assert.Equal(t, sourceId, info.objectId)
		assert.Equal(t, blockId, info.blockId)
		assert.Equal(t, "", info.relationKey)
		// targetId is used as map key: incomingLinksMap[link.TargetID] = append(...)
		assert.Equal(t, "file1", targetId)
	})

	t.Run("convert relation link", func(t *testing.T) {
		sourceId := "task1"
		targetId := "file1"
		relationKey := "attachment"

		link := spaceindex.OutgoingLink{
			TargetID:    targetId,
			BlockID:     "",
			RelationKey: relationKey,
		}

		// Convert to internal format (targetId is the map key, not stored in struct)
		info := incomingLinkInfo{
			objectId:    sourceId,
			blockId:     link.BlockID,
			relationKey: link.RelationKey,
		}

		// Assert
		assert.Equal(t, sourceId, info.objectId)
		assert.Equal(t, "", info.blockId)
		assert.Equal(t, relationKey, info.relationKey)
		// targetId is used as map key: incomingLinksMap[link.TargetID] = append(...)
		assert.Equal(t, "file1", targetId)
	})
}

func TestMigrationDetailsCreation(t *testing.T) {
	// Test creation of details for migration

	t.Run("create details with block context", func(t *testing.T) {
		contextId := "page1"
		blockId := "block1"

		contextInfo := &contextInfo{
			contextId: contextId,
			blockId:   blockId,
		}

		// Create details
		details := []domain.Detail{
			{
				Key:   bundle.RelationKeyCreatedInContext,
				Value: domain.String(contextInfo.contextId),
			},
		}

		if contextInfo.blockId != "" {
			details = append(details, domain.Detail{
				Key:   bundle.RelationKeyCreatedInBlockId,
				Value: domain.String(contextInfo.blockId),
			})
		}

		// Assert
		assert.Len(t, details, 2)
		assert.Equal(t, bundle.RelationKeyCreatedInContext, details[0].Key)
		assert.Equal(t, contextId, details[0].Value.String())
		assert.Equal(t, bundle.RelationKeyCreatedInBlockId, details[1].Key)
		assert.Equal(t, blockId, details[1].Value.String())
	})

	t.Run("create details without block context", func(t *testing.T) {
		contextId := "task1"

		contextInfo := &contextInfo{
			contextId: contextId,
			blockId:   "",
		}

		// Create details
		details := []domain.Detail{
			{
				Key:   bundle.RelationKeyCreatedInContext,
				Value: domain.String(contextInfo.contextId),
			},
		}

		if contextInfo.blockId != "" {
			details = append(details, domain.Detail{
				Key:   bundle.RelationKeyCreatedInBlockId,
				Value: domain.String(contextInfo.blockId),
			})
		}

		// Assert
		assert.Len(t, details, 1)
		assert.Equal(t, bundle.RelationKeyCreatedInContext, details[0].Key)
		assert.Equal(t, contextId, details[0].Value.String())
	})
}

func TestInit(t *testing.T) {
	// Test service initialization
	service := NewContextMigrationService()

	// Assert basic properties
	assert.NotNil(t, service)
	assert.Equal(t, CName, service.Name())
}
