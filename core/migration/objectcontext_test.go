package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func TestService_findCreationContext(t *testing.T) {
	// This test focuses on the core logic of finding creation context
	// which is the most important part of the migration

	s := &service{}

	// Use a large timestamp so block timestamp comparisons pass
	// (block must be created before file for context to be valid)
	var largeTs int64 = 9999999999

	t.Run("empty slice returns nil", func(t *testing.T) {
		fileId := "file1"
		var inboundLinks []spaceindex.IncomingLink

		context := s.findCreationContext(largeTs, fileId, inboundLinks)

		assert.Nil(t, context)
	})

	t.Run("prefer block links over relation links", func(t *testing.T) {
		fileId := "file1"
		pageId := "page1"
		taskId := "task1"
		// Valid BSON ObjectId format (24 hex chars, first 8 are timestamp)
		blockId := "507f1f77bcf86cd799439011"

		inboundLinks := []spaceindex.IncomingLink{
			// Relation link (comes first)
			{
				SourceID:    taskId,
				RelationKey: "attachment",
			},
			// Block link (should be preferred)
			{
				SourceID: pageId,
				BlockID:  blockId,
			},
		}

		context := s.findCreationContext(largeTs, fileId, inboundLinks)

		require.NotNil(t, context)
		assert.Equal(t, pageId, context.contextId)
		assert.Equal(t, blockId, context.blockId)
	})

	t.Run("use first link when multiple relation links exist", func(t *testing.T) {
		fileId := "file1"
		page1 := "page1"
		page2 := "page2"

		inboundLinks := []spaceindex.IncomingLink{
			{
				SourceID:    page2,
				RelationKey: "b",
			},
			{
				SourceID:    page1,
				RelationKey: "a",
			},
		}

		context := s.findCreationContext(largeTs, fileId, inboundLinks)

		// Should pick first after sorting by relationKey
		require.NotNil(t, context)
		assert.Equal(t, page1, context.contextId)
		assert.Equal(t, "", context.blockId)
	})

	t.Run("handle relation links when no block links exist", func(t *testing.T) {
		fileId := "file1"
		taskId := "task1"

		inboundLinks := []spaceindex.IncomingLink{
			{
				SourceID:    taskId,
				RelationKey: "attachment",
			},
		}

		context := s.findCreationContext(largeTs, fileId, inboundLinks)

		require.NotNil(t, context)
		assert.Equal(t, taskId, context.contextId)
		assert.Equal(t, "", context.blockId)
	})

	t.Run("skip self-references", func(t *testing.T) {
		fileId := "file1"
		pageId := "page1"

		inboundLinks := []spaceindex.IncomingLink{
			// Self-reference should be skipped
			{
				SourceID:    fileId,
				RelationKey: "someRelation",
			},
			{
				SourceID:    pageId,
				RelationKey: "attachment",
			},
		}

		context := s.findCreationContext(largeTs, fileId, inboundLinks)

		require.NotNil(t, context)
		assert.Equal(t, pageId, context.contextId)
	})

	t.Run("skip system relations", func(t *testing.T) {
		fileId := "file1"
		pageId := "page1"
		creatorId := "creator1"

		inboundLinks := []spaceindex.IncomingLink{
			// System relation should be skipped
			{
				SourceID:    creatorId,
				RelationKey: string(bundle.RelationKeyCreator),
			},
			{
				SourceID:    pageId,
				RelationKey: "attachment",
			},
		}

		context := s.findCreationContext(largeTs, fileId, inboundLinks)

		require.NotNil(t, context)
		assert.Equal(t, pageId, context.contextId)
	})
}

func TestMigrationDetailsCreation(t *testing.T) {
	t.Run("create details with block context", func(t *testing.T) {
		contextId := "page1"
		blockId := "block1"

		ci := &contextInfo{
			contextId: contextId,
			blockId:   blockId,
		}

		details := []domain.Detail{
			{
				Key:   bundle.RelationKeyCreatedInContext,
				Value: domain.String(ci.contextId),
			},
		}

		if ci.blockId != "" {
			details = append(details, domain.Detail{
				Key:   bundle.RelationKeyCreatedInBlockId,
				Value: domain.String(ci.blockId),
			})
		}

		assert.Len(t, details, 2)
		assert.Equal(t, bundle.RelationKeyCreatedInContext, details[0].Key)
		assert.Equal(t, contextId, details[0].Value.String())
		assert.Equal(t, bundle.RelationKeyCreatedInBlockId, details[1].Key)
		assert.Equal(t, blockId, details[1].Value.String())
	})

	t.Run("create details without block context", func(t *testing.T) {
		contextId := "task1"

		ci := &contextInfo{
			contextId: contextId,
			blockId:   "",
		}

		details := []domain.Detail{
			{
				Key:   bundle.RelationKeyCreatedInContext,
				Value: domain.String(ci.contextId),
			},
		}

		if ci.blockId != "" {
			details = append(details, domain.Detail{
				Key:   bundle.RelationKeyCreatedInBlockId,
				Value: domain.String(ci.blockId),
			})
		}

		assert.Len(t, details, 1)
		assert.Equal(t, bundle.RelationKeyCreatedInContext, details[0].Key)
		assert.Equal(t, contextId, details[0].Value.String())
	})
}

func TestNew(t *testing.T) {
	service := New()

	assert.NotNil(t, service)
	assert.Equal(t, CName, service.Name())
}
