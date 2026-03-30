package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatAttachmentContext(t *testing.T) {
	t.Run("struct fields", func(t *testing.T) {
		ctx := &ChatAttachmentContext{
			ChatObjectId: "chat123",
			MessageId:    "msg456",
			CreatedAt:    1234567890,
		}

		assert.Equal(t, "chat123", ctx.ChatObjectId)
		assert.Equal(t, "msg456", ctx.MessageId)
		assert.Equal(t, int64(1234567890), ctx.CreatedAt)
	})
}

func TestChatAttachmentIndex(t *testing.T) {
	t.Run("index lookup", func(t *testing.T) {
		index := ChatAttachmentIndex{
			"file1": &ChatAttachmentContext{
				ChatObjectId: "chat1",
				MessageId:    "msg1",
				CreatedAt:    1000,
			},
			"file2": &ChatAttachmentContext{
				ChatObjectId: "chat2",
				MessageId:    "msg2",
				CreatedAt:    2000,
			},
		}

		// Lookup existing file
		ctx := index["file1"]
		require.NotNil(t, ctx)
		assert.Equal(t, "chat1", ctx.ChatObjectId)

		// Lookup non-existing file
		ctx = index["file3"]
		assert.Nil(t, ctx)
	})

	t.Run("keeps earliest context", func(t *testing.T) {
		index := make(ChatAttachmentIndex)

		// Add first context
		index["file1"] = &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    2000,
		}

		// Simulate what buildChatAttachmentIndex does - only update if earlier
		newCtx := &ChatAttachmentContext{
			ChatObjectId: "chat2",
			MessageId:    "msg2",
			CreatedAt:    1000, // Earlier
		}
		existing := index["file1"]
		if existing == nil || newCtx.CreatedAt < existing.CreatedAt {
			index["file1"] = newCtx
		}

		assert.Equal(t, "chat2", index["file1"].ChatObjectId)
		assert.Equal(t, int64(1000), index["file1"].CreatedAt)
	})

	t.Run("keeps existing when new is later", func(t *testing.T) {
		index := make(ChatAttachmentIndex)

		// Add first context
		index["file1"] = &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    1000,
		}

		// Simulate what buildChatAttachmentIndex does - only update if earlier
		newCtx := &ChatAttachmentContext{
			ChatObjectId: "chat2",
			MessageId:    "msg2",
			CreatedAt:    2000, // Later - should not replace
		}
		existing := index["file1"]
		if existing == nil || newCtx.CreatedAt < existing.CreatedAt {
			index["file1"] = newCtx
		}

		assert.Equal(t, "chat1", index["file1"].ChatObjectId)
		assert.Equal(t, int64(1000), index["file1"].CreatedAt)
	})
}
