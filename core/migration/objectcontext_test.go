package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func TestService_findBestContext(t *testing.T) {
	s := &service{}
	var fileTs int64 = 9999999999

	t.Run("empty links and no chat returns nil", func(t *testing.T) {
		result := s.findBestContext(fileTs, "file1", nil, nil)
		assert.Nil(t, result)
	})

	t.Run("block link only", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
		}

		result := s.findBestContext(fileTs, "file1", links, nil)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
		assert.Equal(t, "507f1f77bcf86cd799439011", result.blockId)
		assert.Empty(t, result.messageId)
	})

	t.Run("chat only", func(t *testing.T) {
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    1000,
		}

		result := s.findBestContext(fileTs, "file1", nil, chatCtx)

		require.NotNil(t, result)
		assert.Equal(t, "chat1", result.objectId)
		assert.Equal(t, "msg1", result.messageId)
		assert.Empty(t, result.blockId)
	})

	t.Run("relation link only (fallback)", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", RelationKey: "attachment"},
		}

		result := s.findBestContext(fileTs, "file1", links, nil)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
		assert.Empty(t, result.blockId)
		assert.Empty(t, result.messageId)
		assert.Equal(t, int64(0), result.timestamp)
	})

	t.Run("block earlier than chat returns block", func(t *testing.T) {
		// 507f1f77 = timestamp 1350844279
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
		}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    2000000000, // Later than block
		}

		result := s.findBestContext(fileTs, "file1", links, chatCtx)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
		assert.Equal(t, "507f1f77bcf86cd799439011", result.blockId)
	})

	t.Run("chat earlier than block returns chat", func(t *testing.T) {
		// 507f1f77 = timestamp 1350844279
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
		}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    1000, // Earlier than block
		}

		result := s.findBestContext(fileTs, "file1", links, chatCtx)

		require.NotNil(t, result)
		assert.Equal(t, "chat1", result.objectId)
		assert.Equal(t, "msg1", result.messageId)
	})

	t.Run("relation link and chat prefers chat (chat has timestamp)", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", RelationKey: "attachment"},
		}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    1000,
		}

		result := s.findBestContext(fileTs, "file1", links, chatCtx)

		require.NotNil(t, result)
		assert.Equal(t, "chat1", result.objectId)
		assert.Equal(t, "msg1", result.messageId)
	})

	t.Run("chat newer than file is ignored", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", RelationKey: "attachment"},
		}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    fileTs + 1000, // Newer than file
		}

		result := s.findBestContext(fileTs, "file1", links, chatCtx)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId) // Falls back to relation
	})

	t.Run("block newer than file is ignored", func(t *testing.T) {
		// Create a block ID with timestamp newer than fileTs
		// fileTs = 9999999999, so we need a block from the future
		// Actually, let's use a smaller fileTs
		var smallFileTs int64 = 1000
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"}, // timestamp 1350844279 > 1000
			{SourceID: "page2", RelationKey: "attachment"},
		}

		result := s.findBestContext(smallFileTs, "file1", links, nil)

		require.NotNil(t, result)
		assert.Equal(t, "page2", result.objectId) // Falls back to relation
	})
}

func TestService_filterLinks(t *testing.T) {
	s := &service{}

	t.Run("filters self-references", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "file1", RelationKey: "some"},
			{SourceID: "page1", RelationKey: "attachment"},
		}

		result := s.filterLinks("file1", links)

		require.Len(t, result, 1)
		assert.Equal(t, "page1", result[0].SourceID)
	})

	t.Run("filters system relations", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "creator1", RelationKey: string(bundle.RelationKeyCreator)},
			{SourceID: "page1", RelationKey: "attachment"},
		}

		result := s.filterLinks("file1", links)

		require.Len(t, result, 1)
		assert.Equal(t, "page1", result[0].SourceID)
	})
}

func TestService_findBlockContext(t *testing.T) {
	s := &service{}
	var fileTs int64 = 9999999999

	t.Run("finds earliest block", func(t *testing.T) {
		// 507f1f77 = 1350844279, 600000000 = 1610612736
		links := []spaceindex.IncomingLink{
			{SourceID: "page2", BlockID: "60000000bcf86cd799439011"}, // Later
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"}, // Earlier
		}

		result := s.findBlockContext(fileTs, links)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
	})

	t.Run("skips blocks newer than file", func(t *testing.T) {
		var smallFileTs int64 = 1000
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"}, // timestamp > 1000
		}

		result := s.findBlockContext(smallFileTs, links)

		assert.Nil(t, result)
	})
}

func TestService_findRelationContext(t *testing.T) {
	s := &service{}

	t.Run("returns first relation by key", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page2", RelationKey: "b"},
			{SourceID: "page1", RelationKey: "a"},
		}

		result := s.findRelationContext(links)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
	})

	t.Run("skips block links", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
			{SourceID: "page2", RelationKey: "attachment"},
		}

		result := s.findRelationContext(links)

		require.NotNil(t, result)
		assert.Equal(t, "page2", result.objectId)
	})

	t.Run("returns nil if only block links", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
		}

		result := s.findRelationContext(links)

		assert.Nil(t, result)
	})
}

func TestMigrationDetailsCreation(t *testing.T) {
	t.Run("create details with block context", func(t *testing.T) {
		ci := &contextInfo{
			objectId: "page1",
			blockId:  "block1",
		}

		details := []domain.Detail{
			{Key: bundle.RelationKeyCreatedInContext, Value: domain.String(ci.objectId)},
		}
		if ci.blockId != "" {
			details = append(details, domain.Detail{
				Key: bundle.RelationKeyCreatedInContextRef, Value: domain.String(ci.blockId),
			})
		}

		assert.Len(t, details, 2)
		assert.Equal(t, "page1", details[0].Value.String())
		assert.Equal(t, "block1", details[1].Value.String())
	})

	t.Run("create details with chat context", func(t *testing.T) {
		ci := &contextInfo{
			objectId:  "chat1",
			messageId: "msg1",
		}

		details := []domain.Detail{
			{Key: bundle.RelationKeyCreatedInContext, Value: domain.String(ci.objectId)},
		}
		if ci.blockId != "" {
			details = append(details, domain.Detail{
				Key: bundle.RelationKeyCreatedInContextRef, Value: domain.String(ci.blockId),
			})
		} else if ci.messageId != "" {
			details = append(details, domain.Detail{
				Key: bundle.RelationKeyCreatedInContextRef, Value: domain.String(ci.messageId),
			})
		}

		assert.Len(t, details, 2)
		assert.Equal(t, "chat1", details[0].Value.String())
		assert.Equal(t, "msg1", details[1].Value.String())
	})
}

func TestNew(t *testing.T) {
	service := New()

	assert.NotNil(t, service)
	assert.Equal(t, CName, service.Name())
}
