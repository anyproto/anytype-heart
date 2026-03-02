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
		require.Len(t, got.ChatMessage.Blocks, 4)

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
