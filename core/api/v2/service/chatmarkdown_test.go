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

// postToDiscussion posts text into a discussion and returns the blocks the
// middleware received with the result.
func postToDiscussion(t *testing.T, text string) ([]*model.ChatMessageMessageBlock, *v2model.ChatMessageResult) {
	t.Helper()
	fx := newV2Fixture(t)
	fx.addDiscussion(t, testDiscussionId)
	var sent *model.ChatMessage
	fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
		sent = req.Message
		return true
	})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

	got, err := fx.AddChatMessage(context.Background(), testSpaceId, testDiscussionId, v2model.AddChatMessageRequest{Text: text}, false)

	require.NoError(t, err)
	require.NotNil(t, sent)
	require.NotNil(t, sent.Message, "an EMPTY content object rides beside the blocks")
	assert.Empty(t, sent.Message.Text)
	return sent.Blocks, got
}

func styledBlock(style model.BlockContentTextStyle, text string, marks ...*model.BlockContentTextMark) *model.ChatMessageMessageBlock {
	return &model.ChatMessageMessageBlock{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{
		Text: text, Style: style, Marks: marks,
	}}}
}

func TestV2DiscussionMarkdown(t *testing.T) {
	t.Run("block markdown becomes styled blocks, nesting flattened", func(t *testing.T) {
		// given
		text := "# Title\n- one\n  - nested\n1. first\n- [x] done\n> quoted\n```go\nx\ny\n```\n---\npara **b**"
		checked := styledBlock(model.BlockContentText_Checkbox, "done")
		checked.GetText().Checked = true
		code := styledBlock(model.BlockContentText_Code, "x\ny")
		code.GetText().Lang = "go"
		want := []*model.ChatMessageMessageBlock{
			styledBlock(model.BlockContentText_Header1, "Title"),
			styledBlock(model.BlockContentText_Marked, "one"),
			styledBlock(model.BlockContentText_Marked, "nested"),
			styledBlock(model.BlockContentText_Numbered, "first"),
			checked,
			styledBlock(model.BlockContentText_Quote, "quoted"),
			code,
			styledBlock(model.BlockContentText_Paragraph, "---"),
			styledBlock(model.BlockContentText_Paragraph, "para b", chatMark(model.BlockContentTextMark_Bold, 5, 6, "")),
		}

		// when
		got, result := postToDiscussion(t, text)

		// then
		assert.Equal(t, want, got)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a multi-line quote becomes one quote block per line", func(t *testing.T) {
		// when
		got, _ := postToDiscussion(t, "> a\n> b")

		// then
		assert.Equal(t, []*model.ChatMessageMessageBlock{
			styledBlock(model.BlockContentText_Quote, "a"),
			styledBlock(model.BlockContentText_Quote, "b"),
		}, got)
	})

	t.Run("a table becomes one line per row, with a warning", func(t *testing.T) {
		// when
		got, result := postToDiscussion(t, "| a | **b** |\n|---|---|\n| 1 | 2 |")

		// then
		assert.Equal(t, []*model.ChatMessageMessageBlock{
			styledBlock(model.BlockContentText_Paragraph, "a | b", chatMark(model.BlockContentTextMark_Bold, 4, 5, "")),
			styledBlock(model.BlockContentText_Paragraph, "1 | 2"),
		}, got)
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "/text", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, "table")
	})

	t.Run("an empty heading and an empty fence leave no block", func(t *testing.T) {
		// when
		got, _ := postToDiscussion(t, "# \n```\n```\nok")

		// then
		assert.Equal(t, []*model.ChatMessageMessageBlock{styledBlock(model.BlockContentText_Paragraph, "ok")}, got)
	})

	t.Run("an edit keeps headings and code, so only inexpressible blocks are warned about", func(t *testing.T) {
		// given
		existing := &model.ChatMessage{Id: "msg1", Message: &model.ChatMessageMessageContent{}, Blocks: []*model.ChatMessageMessageBlock{
			styledBlock(model.BlockContentText_Header1, "Heading"),
			styledBlock(model.BlockContentText_Code, "code"),
			styledBlock(model.BlockContentText_Callout, "note"),
		}}
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{existing}}).Once()
		var edited *model.ChatMessage
		fx.mwMock.EXPECT().ChatEditMessageContent(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatEditMessageContentRequest) bool {
			edited = req.EditedMessage
			return true
		})).Return(&pb.RpcChatEditMessageContentResponse{}).Once()

		// when
		got, err := fx.EditChatMessage(context.Background(), testSpaceId, testDiscussionId, "msg1", v2model.EditChatMessageRequest{Text: "## Update\n- item"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []*model.ChatMessageMessageBlock{
			styledBlock(model.BlockContentText_Header2, "Update"),
			styledBlock(model.BlockContentText_Marked, "item"),
		}, edited.Blocks)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "1 styled text block(s)")
	})

	t.Run("what the read serves posts back as the same blocks", func(t *testing.T) {
		// given
		first, _ := postToDiscussion(t, "# Title\n- one\n1. a\n2. b\n- [ ] todo\n> q\n```sh\nmake\n```\n---\nsee **this**\nand that")
		served := v2model.ChatMessageFromProto(&model.ChatMessage{Id: "msg1", Message: &model.ChatMessageMessageContent{}, Blocks: first},
			v2model.ChatMessageOptions{SpaceId: testSpaceId})

		// when
		second, _ := postToDiscussion(t, served.Text)

		// then
		assert.Equal(t, first, second)
	})
}
