package apimodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func chatTestMessage() *model.ChatMessage {
	return &model.ChatMessage{
		Id:               "msg1",
		OrderId:          "00a1",
		Creator:          "identityA",
		CreatedAt:        1717405200,
		ModifiedAt:       1717405200,
		ReplyToMessageId: "msg0",
		Message: &model.ChatMessageMessageContent{
			Text:  "can you check the doc?",
			Style: model.BlockContentText_Paragraph,
			Marks: []*model.BlockContentTextMark{{
				Range: &model.Range{From: 8, To: 13},
				Type:  model.BlockContentTextMark_Bold,
			}},
		},
		Attachments: []*model.ChatMessageAttachment{
			{Target: "file1", Type: model.ChatMessageAttachment_IMAGE},
		},
		Reactions: &model.ChatMessageReactions{
			Reactions: map[string]*model.ChatMessageReactionsIdentityList{
				"👍": {Ids: []string{"identityA", "identityB"}},
			},
		},
	}
}

func TestV2ChatMessageFromProto(t *testing.T) {
	t.Run("marks render into §8 markup text — offset arrays never cross the API", func(t *testing.T) {
		// given
		msg := chatTestMessage()

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})

		// then
		assert.Equal(t, "can you **check** the doc?", got.Text,
			"the bold mark must render as §8 markup, not ride as an offset array")
		assert.Equal(t, "msg1", got.Id)
		assert.Equal(t, "00a1", got.Order)
		assert.Equal(t, "msg0", got.ReplyTo)
		assert.Equal(t, "2024-06-03T09:00:00Z", got.At,
			"dates are RFC 3339 UTC — the one date shape v2 uses everywhere (C2), not a unix epoch")
		assert.Zero(t, got.EditedAt, "modifiedAt == createdAt means never edited")
	})

	t.Run("markup bridge round-trips: rendered text parses back to the same marks", func(t *testing.T) {
		// given
		msg := chatTestMessage()
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})

		// when: the write path parses the same §8 source the read path rendered
		text, marks, err := anyblockjson.ParseInlineText(got.Text)

		// then
		require.NoError(t, err)
		assert.Equal(t, msg.Message.Text, text)
		require.Len(t, marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Bold, marks[0].Type)
		assert.Equal(t, msg.Message.Marks[0].Range.From, marks[0].Range.From)
		assert.Equal(t, msg.Message.Marks[0].Range.To, marks[0].Range.To)
	})

	t.Run("mention marks render as §8 mention tags", func(t *testing.T) {
		// given
		msg := chatTestMessage()
		msg.Message = &model.ChatMessageMessageContent{
			Text: "hi Alice",
			Marks: []*model.BlockContentTextMark{{
				Range: &model.Range{From: 3, To: 8},
				Type:  model.BlockContentTextMark_Mention,
				Param: "participantObj1",
			}},
		}

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})

		// then
		assert.Equal(t, `hi <mention objectId="participantObj1">Alice</mention>`, got.Text)
	})

	t.Run("author becomes the participant id plus the enriched name", func(t *testing.T) {
		// given
		msg := chatTestMessage()
		wantId := domain.NewParticipantId("space1", "identityA")

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{
			SpaceId: "space1",
			ParticipantName: func(participantId string) string {
				require.Equal(t, wantId, participantId)
				return "Alice"
			},
		})

		// then
		assert.Equal(t, wantId, got.AuthorId, "the raw identity never crosses the API")
		assert.Equal(t, "Alice", got.Author)
	})

	t.Run("reactions default to counts (Q4)", func(t *testing.T) {
		// given
		msg := chatTestMessage()

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})

		// then
		want := map[string]int{"👍": 2}
		assert.Equal(t, want, got.Reactions)
	})

	t.Run("reactions=full adds reactedBy — reactions keeps the counts type (C2)", func(t *testing.T) {
		// given
		msg := chatTestMessage()

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1", FullReactions: true})

		// then: two slots, each with ONE stable type — a model that saw
		// {"👍":2} must still be able to index reactions under ?reactions=full
		want := map[string][]string{"👍": {
			domain.NewParticipantId("space1", "identityA"),
			domain.NewParticipantId("space1", "identityB"),
		}}
		assert.Equal(t, want, got.ReactedBy)
		assert.Equal(t, map[string]int{"👍": 2}, got.Reactions)
	})

	t.Run("empty reactions are omitted, not rendered as {}", func(t *testing.T) {
		// given
		msg := chatTestMessage()
		msg.Reactions = &model.ChatMessageReactions{}

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})

		// then
		assert.Nil(t, got.Reactions)
		assert.Nil(t, got.ReactedBy)
	})

	t.Run("attachments carry id and inferred kind", func(t *testing.T) {
		// given
		msg := chatTestMessage()

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})

		// then
		want := []V2ChatAttachment{{Id: "file1", Type: "image"}}
		assert.Equal(t, want, got.Attachments)
	})

	t.Run("editedAt appears only when the message was edited", func(t *testing.T) {
		// given
		msg := chatTestMessage()
		msg.ModifiedAt = 1717405300

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})

		// then
		assert.Equal(t, "2024-06-03T09:01:40Z", got.EditedAt)
	})

	t.Run("block-composed content surfaces as blocksText — a blocks-only message is not empty", func(t *testing.T) {
		// given: chatmodel.Validate accepts a message whose ONLY content is
		// blocks (desktop quotes, rich pastes) — dropping them on read makes
		// real content invisible to an agent
		msg := chatTestMessage()
		msg.Message = nil
		msg.Blocks = []*model.ChatMessageMessageBlock{
			{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{
				Text: "quoted line",
				Marks: []*model.BlockContentTextMark{{
					Range: &model.Range{From: 0, To: 6},
					Type:  model.BlockContentTextMark_Bold,
				}},
			}}},
			{Content: &model.ChatMessageMessageBlockContentOfEditorQuote{EditorQuote: &model.ChatMessageMessageBlockEditorQuote{
				BlockId: "b1",
				Content: &model.ChatMessageMessageBlockText{Text: "the quoted editor text"},
			}}},
		}

		// when
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})

		// then
		assert.Empty(t, got.Text)
		assert.Equal(t, "**quoted** line\nthe quoted editor text", got.BlocksText,
			"text-bearing blocks render as §8 markup, newline-joined")
	})

	t.Run("non-BMP text round-trips with UTF-16 offsets across the bridge", func(t *testing.T) {
		// given: an astral emoji is TWO UTF-16 units — a codec disagreement
		// on the unit would shift every mark after it
		msg := chatTestMessage()
		msg.Message = &model.ChatMessageMessageContent{
			Text: "🎉 party time",
			Marks: []*model.BlockContentTextMark{{
				Range: &model.Range{From: 3, To: 8}, // "party" after the 2-unit emoji
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		// when: read renders, the write path re-parses the rendered source
		got := V2ChatMessageFromProto(msg, V2ChatMessageOptions{SpaceId: "space1"})
		text, marks, err := anyblockjson.ParseInlineText(got.Text)

		// then
		require.NoError(t, err)
		assert.Equal(t, "🎉 **party** time", got.Text)
		assert.Equal(t, "🎉 party time", text)
		require.Len(t, marks, 1)
		assert.Equal(t, int32(3), marks[0].Range.From, "offsets are UTF-16 units on both sides")
		assert.Equal(t, int32(8), marks[0].Range.To)
	})
}

func TestV2ChatStateFromProto(t *testing.T) {
	t.Run("full passthrough incl. lastStateId — the field v1 dropped", func(t *testing.T) {
		// given
		state := &model.ChatState{
			Messages:              &model.ChatStateUnreadState{OldestOrderId: "00a1", Counter: 3},
			Mentions:              &model.ChatStateUnreadState{OldestOrderId: "00a2", Counter: 1},
			LastStateId:           "state42",
			UnreadReactionOrderId: "00a3",
		}
		want := &V2ChatState{
			UnreadMessages:           3,
			UnreadMentions:           1,
			OldestUnreadOrder:        "00a1",
			OldestUnreadMentionOrder: "00a2",
			UnreadReactionOrder:      "00a3",
			LastStateId:              "state42",
		}

		// when
		got := V2ChatStateFromProto(state)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("nil state stays nil", func(t *testing.T) {
		assert.Nil(t, V2ChatStateFromProto(nil))
	})
}
