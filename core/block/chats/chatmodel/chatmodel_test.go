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

	t.Run("valid text block", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{},
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{
							Text:  "hello",
							Style: model.BlockContentText_Paragraph,
							Marks: []*model.BlockContentTextMark{
								{
									Type: model.BlockContentTextMark_Bold,
									Range: &model.Range{
										From: 0,
										To:   5,
									},
								},
							},
						},
					}},
				},
			},
		}

		assert.NoError(t, msg.Validate())
	})

	t.Run("valid link block", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{},
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfLink{
						Link: &model.ChatMessageMessageBlockLink{
							TargetObjectId: "obj123",
							Type:           model.ChatMessageMessageBlockLink_File,
						},
					}},
				},
			},
		}

		assert.NoError(t, msg.Validate())
	})

	t.Run("mixed text and link blocks", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{Text: "legacy"},
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{Text: "block1"},
					}},
					{Content: &model.ChatMessageMessageBlockContentOfLink{
						Link: &model.ChatMessageMessageBlockLink{TargetObjectId: "obj1", Type: model.ChatMessageMessageBlockLink_Image},
					}},
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{Text: "block2"},
					}},
				},
			},
		}

		assert.NoError(t, msg.Validate())
	})

	t.Run("text block exceeds max length", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{},
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{
							Text: strings.Repeat("a", MaxMessageLength+1),
						},
					}},
				},
			},
		}

		assert.Error(t, msg.Validate())
	})

	t.Run("text block invalid mark range", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{},
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{
							Text: "hi",
							Marks: []*model.BlockContentTextMark{
								{
									Type:  model.BlockContentTextMark_Bold,
									Range: &model.Range{From: 0, To: 10},
								},
							},
						},
					}},
				},
			},
		}

		assert.Error(t, msg.Validate())
	})

	t.Run("link block empty target", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{},
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfLink{
						Link: &model.ChatMessageMessageBlockLink{
							TargetObjectId: "",
						},
					}},
				},
			},
		}

		assert.Error(t, msg.Validate())
	})

	t.Run("block with nil content", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{},
				Blocks:  []*model.ChatMessageMessageBlock{{}},
			},
		}

		assert.Error(t, msg.Validate())
	})

	t.Run("nil message field with attachments is valid", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Attachments: []*model.ChatMessageAttachment{
					{Target: "file1", Type: model.ChatMessageAttachment_FILE},
				},
			},
		}

		assert.NoError(t, msg.Validate())
	})

	t.Run("nil message field with blocks is valid", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{Text: "hello"},
					}},
				},
			},
		}

		assert.NoError(t, msg.Validate())
	})

	t.Run("no content at all is invalid", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{},
		}

		assert.Error(t, msg.Validate())
	})

	t.Run("empty message with no attachments or blocks is invalid", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{},
			},
		}

		assert.Error(t, msg.Validate())
	})

	t.Run("mark with nil range is invalid", func(t *testing.T) {
		msg := &Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: "hello",
					Marks: []*model.BlockContentTextMark{
						{
							Type:  model.BlockContentTextMark_Bold,
							Range: nil,
						},
					},
				},
			},
		}

		assert.Error(t, msg.Validate())
	})
}

func TestBlocksRoundTrip(t *testing.T) {
	t.Run("text and link blocks marshal and unmarshal", func(t *testing.T) {
		// given
		original := &Message{
			ChatMessage: &model.ChatMessage{
				Id:      "msg1",
				Creator: "user1",
				Message: &model.ChatMessageMessageContent{
					Text: "legacy text",
				},
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{
							Text:  "hello world",
							Style: model.BlockContentText_Header1,
							Marks: []*model.BlockContentTextMark{
								{
									Type:  model.BlockContentTextMark_Bold,
									Range: &model.Range{From: 0, To: 5},
								},
								{
									Type:  model.BlockContentTextMark_Mention,
									Param: "participant_abc",
									Range: &model.Range{From: 6, To: 11},
								},
							},
							Checked: true,
							Lang:    "go",
						},
					}},
					{Content: &model.ChatMessageMessageBlockContentOfLink{
						Link: &model.ChatMessageMessageBlockLink{
							TargetObjectId: "fileObj1",
							Type:           model.ChatMessageMessageBlockLink_File,
						},
					}},
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{
							Text: "second paragraph",
						},
					}},
					{Content: &model.ChatMessageMessageBlockContentOfLink{
						Link: &model.ChatMessageMessageBlockLink{
							TargetObjectId: "imgObj1",
							Type:           model.ChatMessageMessageBlockLink_Image,
						},
					}},
					{Content: &model.ChatMessageMessageBlockContentOfEmbed{
						Embed: &model.ChatMessageMessageBlockEmbed{
							Text:      "graph TD; A-->B;",
							Processor: model.BlockContentLatex_Mermaid,
						},
					}},
				},
			},
		}

		arena := &anyenc.Arena{}
		val := arena.NewObject()

		// when
		original.MarshalAnyenc(val, arena)
		got, err := UnmarshalMessage(val)

		// then
		require.NoError(t, err)
		require.Len(t, got.ChatMessage.Blocks, 5)

		// Text block 0
		tb0 := got.ChatMessage.Blocks[0].GetText()
		require.NotNil(t, tb0)
		assert.Equal(t, "hello world", tb0.Text)
		assert.Equal(t, model.BlockContentText_Header1, tb0.Style)
		require.Len(t, tb0.Marks, 2)
		assert.Equal(t, model.BlockContentTextMark_Bold, tb0.Marks[0].Type)
		assert.Equal(t, int32(0), tb0.Marks[0].Range.From)
		assert.Equal(t, int32(5), tb0.Marks[0].Range.To)
		assert.Equal(t, model.BlockContentTextMark_Mention, tb0.Marks[1].Type)
		assert.Equal(t, "participant_abc", tb0.Marks[1].Param)
		assert.True(t, tb0.Checked)
		assert.Equal(t, "go", tb0.Lang)

		// Link block 1
		lb1 := got.ChatMessage.Blocks[1].GetLink()
		require.NotNil(t, lb1)
		assert.Equal(t, "fileObj1", lb1.TargetObjectId)
		assert.Equal(t, model.ChatMessageMessageBlockLink_File, lb1.Type)

		// Text block 2
		tb2 := got.ChatMessage.Blocks[2].GetText()
		require.NotNil(t, tb2)
		assert.Equal(t, "second paragraph", tb2.Text)

		// Link block 3
		lb3 := got.ChatMessage.Blocks[3].GetLink()
		require.NotNil(t, lb3)
		assert.Equal(t, "imgObj1", lb3.TargetObjectId)
		assert.Equal(t, model.ChatMessageMessageBlockLink_Image, lb3.Type)

		// Embed block 4
		eb4 := got.ChatMessage.Blocks[4].GetEmbed()
		require.NotNil(t, eb4)
		assert.Equal(t, "graph TD; A-->B;", eb4.Text)
		assert.Equal(t, model.BlockContentLatex_Mermaid, eb4.Processor)
	})

	t.Run("empty blocks round-trip", func(t *testing.T) {
		original := &Message{
			ChatMessage: &model.ChatMessage{
				Id:      "msg2",
				Message: &model.ChatMessageMessageContent{Text: "just text"},
			},
		}

		arena := &anyenc.Arena{}
		val := arena.NewObject()
		original.MarshalAnyenc(val, arena)
		got, err := UnmarshalMessage(val)

		require.NoError(t, err)
		assert.Nil(t, got.ChatMessage.Blocks)
		assert.Equal(t, "just text", got.Message.Text)
	})

	t.Run("zero-value checked and lang are omitted from encoding", func(t *testing.T) {
		// given
		original := &Message{
			ChatMessage: &model.ChatMessage{
				Id: "msg3",
				Message: &model.ChatMessageMessageContent{Text: "t"},
				Blocks: []*model.ChatMessageMessageBlock{
					{Content: &model.ChatMessageMessageBlockContentOfText{
						Text: &model.ChatMessageMessageBlockText{
							Text:    "plain",
							Checked: false,
							Lang:    "",
						},
					}},
				},
			},
		}

		arena := &anyenc.Arena{}
		val := arena.NewObject()

		// when
		original.MarshalAnyenc(val, arena)

		// then — raw encoding must not contain "checked" or "lang" keys
		raw := val.String()
		assert.NotContains(t, raw, "checked")
		assert.NotContains(t, raw, "lang")

		// unmarshal and verify defaults
		got, err := UnmarshalMessage(val)
		require.NoError(t, err)
		require.Len(t, got.ChatMessage.Blocks, 1)
		tb := got.ChatMessage.Blocks[0].GetText()
		require.NotNil(t, tb)
		assert.False(t, tb.Checked)
		assert.Empty(t, tb.Lang)
	})
}

func TestBlocksText(t *testing.T) {
	msg := &Message{
		ChatMessage: &model.ChatMessage{
			Message: &model.ChatMessageMessageContent{Text: "legacy"},
			Blocks: []*model.ChatMessageMessageBlock{
				{Content: &model.ChatMessageMessageBlockContentOfText{
					Text: &model.ChatMessageMessageBlockText{Text: "first"},
				}},
				{Content: &model.ChatMessageMessageBlockContentOfLink{
					Link: &model.ChatMessageMessageBlockLink{TargetObjectId: "obj1"},
				}},
				{Content: &model.ChatMessageMessageBlockContentOfText{
					Text: &model.ChatMessageMessageBlockText{Text: "second"},
				}},
			},
		},
	}

	assert.Equal(t, "first\nsecond", msg.BlocksText())
}

func TestLinkBlockTargetIds(t *testing.T) {
	msg := &Message{
		ChatMessage: &model.ChatMessage{
			Message: &model.ChatMessageMessageContent{},
			Blocks: []*model.ChatMessageMessageBlock{
				{Content: &model.ChatMessageMessageBlockContentOfText{
					Text: &model.ChatMessageMessageBlockText{Text: "text"},
				}},
				{Content: &model.ChatMessageMessageBlockContentOfLink{
					Link: &model.ChatMessageMessageBlockLink{TargetObjectId: "file1"},
				}},
				{Content: &model.ChatMessageMessageBlockContentOfLink{
					Link: &model.ChatMessageMessageBlockLink{TargetObjectId: "img1"},
				}},
			},
		},
	}

	assert.Equal(t, []string{"file1", "img1"}, msg.LinkBlockTargetIds())
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
