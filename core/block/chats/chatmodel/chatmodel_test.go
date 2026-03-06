package chatmodel

import (
	"strings"
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestValidate(t *testing.T) {
	t.Run("valid message", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: "1",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Emoji,
							Param: "😇",
							Range: &model.Range{
								From: 0,
								To:   1,
							},
						},
					},
				},
			},
		}

		assert.NoError(t, msg.Validate())
	})

	t.Run("invalid message: mark range from", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: "1",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Emoji,
							Param: "😇",
							Range: &model.Range{
								From: 1,
								To:   2,
							},
						},
					},
				},
			},
		}

		assert.Error(t, msg.Validate())
	})

	t.Run("invalid message: mark range to", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: "1",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Emoji,
							Param: "😇",
							Range: &model.Range{
								From: 0,
								To:   2,
							},
						},
					},
				},
			},
		}

		assert.Error(t, msg.Validate())
	})

	t.Run("message at max length is valid", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: strings.Repeat("a", MaxMessageLength),
				},
			},
		}

		assert.NoError(t, msg.Validate())
	})

	t.Run("message exceeds max length", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: strings.Repeat("a", MaxMessageLength+1),
				},
			},
		}

		assert.Error(t, msg.Validate())
	})
}

func TestAddUnreadReaction(t *testing.T) {
	t.Run("add to nil map", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{}}

		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})

		require.Len(t, msg.UnreadReactionIds, 1)
		require.Len(t, msg.UnreadReactionIds["👍"], 1)
		assert.Equal(t, "ch1", msg.UnreadReactionIds["👍"]["user1"].ChangeId)
	})

	t.Run("add multiple emojis and identities", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{}}

		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})
		msg.AddUnreadReaction("👍", "user2", ReactionChangeEntry{ChangeId: "ch2"})
		msg.AddUnreadReaction("❤️", "user3", ReactionChangeEntry{ChangeId: "ch3"})

		require.Len(t, msg.UnreadReactionIds, 2)
		require.Len(t, msg.UnreadReactionIds["👍"], 2)
		require.Len(t, msg.UnreadReactionIds["❤️"], 1)
		assert.Equal(t, "ch1", msg.UnreadReactionIds["👍"]["user1"].ChangeId)
		assert.Equal(t, "ch2", msg.UnreadReactionIds["👍"]["user2"].ChangeId)
		assert.Equal(t, "ch3", msg.UnreadReactionIds["❤️"]["user3"].ChangeId)
	})

	t.Run("overwrite existing entry", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{}}

		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})
		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch2"})

		require.Len(t, msg.UnreadReactionIds["👍"], 1)
		assert.Equal(t, "ch2", msg.UnreadReactionIds["👍"]["user1"].ChangeId)
	})
}

func TestRemoveUnreadReaction(t *testing.T) {
	t.Run("remove from nil map returns empty", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{}}

		empty := msg.RemoveUnreadReaction("👍", "user1")

		assert.True(t, empty)
	})

	t.Run("remove nonexistent emoji returns correct empty state", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{}}
		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})

		empty := msg.RemoveUnreadReaction("❤️", "user1")

		assert.False(t, empty)
		require.Len(t, msg.UnreadReactionIds, 1)
	})

	t.Run("remove last identity from emoji cleans up emoji key", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{}}
		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})

		empty := msg.RemoveUnreadReaction("👍", "user1")

		assert.True(t, empty)
		assert.Empty(t, msg.UnreadReactionIds)
	})

	t.Run("remove one identity keeps other", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{}}
		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})
		msg.AddUnreadReaction("👍", "user2", ReactionChangeEntry{ChangeId: "ch2"})

		empty := msg.RemoveUnreadReaction("👍", "user1")

		assert.False(t, empty)
		require.Len(t, msg.UnreadReactionIds["👍"], 1)
		assert.Equal(t, "ch2", msg.UnreadReactionIds["👍"]["user2"].ChangeId)
	})

	t.Run("remove last across multiple emojis", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{}}
		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})
		msg.AddUnreadReaction("❤️", "user2", ReactionChangeEntry{ChangeId: "ch2"})

		empty := msg.RemoveUnreadReaction("👍", "user1")
		assert.False(t, empty)

		empty = msg.RemoveUnreadReaction("❤️", "user2")
		assert.True(t, empty)
		assert.Empty(t, msg.UnreadReactionIds)
	})
}


func TestCloneUnreadReactionIds(t *testing.T) {
	t.Run("clone with nil UnreadReactionIds", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{Id: "msg1"}}

		cloned := msg.Clone()

		assert.Nil(t, cloned.UnreadReactionIds)
		assert.Equal(t, "msg1", cloned.Id)
	})

	t.Run("clone is deep copy", func(t *testing.T) {
		msg := &Message{ChatMessage: &model.ChatMessage{Id: "msg1"}}
		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})

		cloned := msg.Clone()

		// Verify data is equal
		require.Len(t, cloned.UnreadReactionIds, 1)
		assert.Equal(t, "ch1", cloned.UnreadReactionIds["👍"]["user1"].ChangeId)

		// Mutate original — clone should be unaffected
		msg.AddUnreadReaction("👍", "user2", ReactionChangeEntry{ChangeId: "ch2"})

		assert.Len(t, cloned.UnreadReactionIds["👍"], 1)
	})
}

func TestMarshalUnmarshalUnreadReactionIds(t *testing.T) {
	t.Run("round trip with reactions", func(t *testing.T) {
		// given
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Id:      "msg1",
				OrderId: "ord1",
				Creator: "creator1",
				Message: &model.ChatMessageMessageContent{Text: "hello"},
			},
		}
		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1", OrderId: "ord1"})
		msg.AddUnreadReaction("👍", "user2", ReactionChangeEntry{ChangeId: "ch2", OrderId: "ord1"})
		msg.AddUnreadReaction("❤️", "user3", ReactionChangeEntry{ChangeId: "ch3", OrderId: "ord1"})

		// when
		arena := &anyenc.Arena{}
		v := arena.NewObject()
		msg.MarshalAnyenc(v, arena)
		got, err := UnmarshalMessage(v)

		// then
		require.NoError(t, err)
		require.Len(t, got.UnreadReactionIds, 2)
		require.Len(t, got.UnreadReactionIds["👍"], 2)
		assert.Equal(t, "ch1", got.UnreadReactionIds["👍"]["user1"].ChangeId)
		assert.Equal(t, "ord1", got.UnreadReactionIds["👍"]["user1"].OrderId)
		assert.Equal(t, "ch2", got.UnreadReactionIds["👍"]["user2"].ChangeId)
		assert.Equal(t, "ord1", got.UnreadReactionIds["👍"]["user2"].OrderId)
		require.Len(t, got.UnreadReactionIds["❤️"], 1)
		assert.Equal(t, "ch3", got.UnreadReactionIds["❤️"]["user3"].ChangeId)
		assert.Equal(t, "ord1", got.UnreadReactionIds["❤️"]["user3"].OrderId)
		assert.True(t, got.UnreadReaction)
	})

	t.Run("round trip with empty map clears fields", func(t *testing.T) {
		// given: marshal with reactions first
		arena := &anyenc.Arena{}
		v := arena.NewObject()
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Id:      "msg1",
				Creator: "creator1",
				Message: &model.ChatMessageMessageContent{Text: "hello"},
			},
		}
		msg.AddUnreadReaction("👍", "user1", ReactionChangeEntry{ChangeId: "ch1"})
		msg.MarshalAnyenc(v, arena)

		// when: marshal again with empty map (simulating all reactions removed)
		msg.RemoveUnreadReaction("👍", "user1")
		msg.MarshalUnreadReactionIds(v, arena)
		got, err := UnmarshalMessage(v)

		// then
		require.NoError(t, err)
		assert.Nil(t, got.UnreadReactionIds)
		assert.False(t, got.UnreadReaction)
	})

	t.Run("round trip without unread reactions", func(t *testing.T) {
		// given
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Id:      "msg1",
				Creator: "creator1",
				Message: &model.ChatMessageMessageContent{Text: "hello"},
			},
		}

		// when
		arena := &anyenc.Arena{}
		v := arena.NewObject()
		msg.MarshalAnyenc(v, arena)
		got, err := UnmarshalMessage(v)

		// then
		require.NoError(t, err)
		assert.Nil(t, got.UnreadReactionIds)
		assert.False(t, got.UnreadReaction)
	})

	t.Run("UnreadReaction true from rUnreadOrdId without rUnreadChIds", func(t *testing.T) {
		// given: manually set rUnreadOrdId without rUnreadChIds (e.g. data inconsistency)
		arena := &anyenc.Arena{}
		v := arena.NewObject()
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Id:      "msg1",
				Creator: "creator1",
				Message: &model.ChatMessageMessageContent{Text: "hello"},
			},
		}
		msg.MarshalAnyenc(v, arena)
		v.Set(ReactionUnreadOrderIdKey, arena.NewString("order1"))

		// when
		got, err := UnmarshalMessage(v)

		// then
		require.NoError(t, err)
		assert.Nil(t, got.UnreadReactionIds)
		assert.True(t, got.UnreadReaction)
	})
}
