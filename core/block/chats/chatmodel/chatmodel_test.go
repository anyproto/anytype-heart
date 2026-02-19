package chatmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
}
