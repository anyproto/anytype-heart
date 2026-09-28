package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// postToSpaceChat posts text into a space chat and returns the content the
// middleware received.
func postToSpaceChat(t *testing.T, text string) *model.ChatMessageMessageContent {
	t.Helper()
	fx := newV2Fixture(t)
	fx.addChat(t, testChatId, "Team chat", 1000)
	var sent *model.ChatMessage
	fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
		sent = req.Message
		return true
	})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

	_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId, v2model.AddChatMessageRequest{Text: text}, false)

	require.NoError(t, err)
	require.NotNil(t, sent)
	assert.Empty(t, sent.Blocks, "a space chat message stays content")
	return sent.Message
}

func chatMark(markType model.BlockContentTextMarkType, from, to int32, param string) *model.BlockContentTextMark {
	return &model.BlockContentTextMark{Range: &model.Range{From: from, To: to}, Type: markType, Param: param}
}

func TestV2SpaceChatMarkdown(t *testing.T) {
	t.Run("a heading line becomes a bold line", func(t *testing.T) {
		// given
		text := "# Plan\nsee **this**"
		want := &model.ChatMessageMessageContent{
			Text:  "Plan\nsee this",
			Style: model.BlockContentText_Paragraph,
			Marks: []*model.BlockContentTextMark{
				chatMark(model.BlockContentTextMark_Bold, 0, 4, ""),
				chatMark(model.BlockContentTextMark_Bold, 9, 13, ""),
			},
		}

		// when
		got := postToSpaceChat(t, text)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a heading keeps its inline marks, and one bold spans the whole line", func(t *testing.T) {
		// given: a bold inside the heading would overlap the line's own
		text := "### **Big** [site](https://example.com)"
		want := &model.ChatMessageMessageContent{
			Text:  "Big site",
			Style: model.BlockContentText_Paragraph,
			Marks: []*model.BlockContentTextMark{
				chatMark(model.BlockContentTextMark_Bold, 0, 8, ""),
				chatMark(model.BlockContentTextMark_Link, 4, 8, "https://example.com"),
			},
		}

		// when
		got := postToSpaceChat(t, text)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a hash without a space is not a heading", func(t *testing.T) {
		// when
		got := postToSpaceChat(t, "#hashtag")

		// then
		assert.Equal(t, "#hashtag", got.Text)
		assert.Empty(t, got.Marks)
	})

	t.Run("a code fence becomes a code mark over its lines, the language dropped", func(t *testing.T) {
		// given: the desktop composer stores ``` fences exactly so; the code
		// itself is literal, not markup
		text := "Run:\n```go\nx := **1**\ny := 2\n```\ndone"
		want := &model.ChatMessageMessageContent{
			Text:  "Run:\nx := **1**\ny := 2\ndone",
			Style: model.BlockContentText_Paragraph,
			Marks: []*model.BlockContentTextMark{
				chatMark(model.BlockContentTextMark_Keyboard, 5, 22, ""),
			},
		}

		// when
		got := postToSpaceChat(t, text)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a fence closes only on a marker run at least as long as its opener", func(t *testing.T) {
		// given
		text := "~~~~\n~~~\nx\n~~~~~"

		// when
		got := postToSpaceChat(t, text)

		// then
		assert.Equal(t, "~~~\nx", got.Text)
		assert.Equal(t, []*model.BlockContentTextMark{chatMark(model.BlockContentTextMark_Keyboard, 0, 5, "")}, got.Marks)
	})

	t.Run("an unterminated fence runs to the end", func(t *testing.T) {
		// when
		got := postToSpaceChat(t, "```\na\nb")

		// then
		assert.Equal(t, "a\nb", got.Text)
		assert.Equal(t, []*model.BlockContentTextMark{chatMark(model.BlockContentTextMark_Keyboard, 0, 3, "")}, got.Marks)
	})

	t.Run("an empty fence leaves no line behind", func(t *testing.T) {
		// when
		got := postToSpaceChat(t, "a\n```\n```\nb")

		// then
		assert.Equal(t, "a\nb", got.Text)
		assert.Empty(t, got.Marks)
	})

	t.Run("a heading inside a fence is code", func(t *testing.T) {
		// when
		got := postToSpaceChat(t, "```\n# not a heading\n```")

		// then
		assert.Equal(t, "# not a heading", got.Text)
		assert.Equal(t, []*model.BlockContentTextMark{chatMark(model.BlockContentTextMark_Keyboard, 0, 15, "")}, got.Marks)
	})

	t.Run("lists, quotes and blank lines pass through as today", func(t *testing.T) {
		// when
		got := postToSpaceChat(t, "- one\n\n\n> two")

		// then
		assert.Equal(t, "- one\n\n\n> two", got.Text)
		assert.Empty(t, got.Marks)
	})

	t.Run("an edit parses headings and fences the same way", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatProtoMessage()}})
		var edited *model.ChatMessage
		fx.mwMock.EXPECT().ChatEditMessageContent(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatEditMessageContentRequest) bool {
			edited = req.EditedMessage
			return true
		})).Return(&pb.RpcChatEditMessageContentResponse{})

		// when
		_, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.EditChatMessageRequest{Text: "# Done\n```\nok\n```"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "Done\nok", edited.Message.Text)
		assert.Equal(t, []*model.BlockContentTextMark{
			chatMark(model.BlockContentTextMark_Bold, 0, 4, ""),
			chatMark(model.BlockContentTextMark_Keyboard, 5, 7, ""),
		}, edited.Message.Marks)
	})

	t.Run("what the read serves posts back as the same message", func(t *testing.T) {
		// given
		first := postToSpaceChat(t, "# Plan\n```sh\nmake\nmake test\n```\nthen **ship**")
		served := v2model.ChatMessageFromProto(&model.ChatMessage{Id: "msg1", Message: first}, v2model.ChatMessageOptions{SpaceId: testSpaceId})

		// when
		second := postToSpaceChat(t, served.Text)

		// then
		assert.Equal(t, first, second)
	})
}
