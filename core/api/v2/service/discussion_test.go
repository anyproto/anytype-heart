package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const testDiscussionId = "discussion1"

// pageRead is a live read of a page, optionally carrying a discussion id
// the way ObjectAddDiscussion leaves it on the parent (a persisted detail).
func pageRead(sbType model.SmartBlockType, discussionId string) apicore.ObjectRead {
	read := testObjectRead()
	read.SbType = sbType
	if discussionId != "" {
		read.Snapshot.Details.Fields[bundle.RelationKeyDiscussionId.String()] = pbtypes.String(discussionId)
	}
	return read
}

// addDiscussion registers a discussion object (discussion layout) in the
// store — what the chat gate resolves a discussion id against.
func (fx *v2Fixture) addDiscussion(t *testing.T, id string) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_discussion)),
	}})
}

func TestV2CreateDiscussion(t *testing.T) {
	t.Run("an object without a discussion gets one minted", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(pageRead(model.SmartBlockType_Page, ""), nil)
		fx.mwMock.EXPECT().ObjectAddDiscussion(mock.Anything, &pb.RpcObjectDiscussionAddRequest{ObjectId: "obj1"}).
			Return(&pb.RpcObjectDiscussionAddResponse{DiscussionId: testDiscussionId}).Once()
		want := &v2model.DiscussionResult{Id: testDiscussionId, Created: true}

		// when
		got, err := fx.CreateDiscussion(context.Background(), testSpaceId, "obj1", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("an object that already has one answers the same id without minting", func(t *testing.T) {
		// given: no ObjectAddDiscussion expectation — a call would fail the test
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(pageRead(model.SmartBlockType_Page, testDiscussionId), nil)
		want := &v2model.DiscussionResult{Id: testDiscussionId}

		// when
		got, err := fx.CreateDiscussion(context.Background(), testSpaceId, "obj1", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got, "created stays false: the caller learns the call was a lookup")
	})

	t.Run("dry run reports the would-be outcome and mints nothing", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(pageRead(model.SmartBlockType_Page, ""), nil).Once()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj2").Return(pageRead(model.SmartBlockType_Page, testDiscussionId), nil).Once()

		// when
		fresh, err := fx.CreateDiscussion(context.Background(), testSpaceId, "obj1", true)
		require.NoError(t, err)
		existing, err := fx.CreateDiscussion(context.Background(), testSpaceId, "obj2", true)
		require.NoError(t, err)

		// then
		assert.Equal(t, &v2model.DiscussionResult{Created: true, DryRun: true}, fresh, "no id yet: nothing was minted")
		assert.Equal(t, &v2model.DiscussionResult{Id: testDiscussionId, DryRun: true}, existing)
	})

	t.Run("a file object can hold a discussion", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "file1").Return(pageRead(model.SmartBlockType_FileObject, ""), nil)
		fx.mwMock.EXPECT().ObjectAddDiscussion(mock.Anything, mock.Anything).Return(&pb.RpcObjectDiscussionAddResponse{DiscussionId: "d-file"}).Once()

		got, err := fx.CreateDiscussion(context.Background(), testSpaceId, "file1", false)

		require.NoError(t, err)
		assert.Equal(t, "d-file", got.Id)
	})

	t.Run("a chat is refused with the route that already serves its messages", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, testChatId).Return(pageRead(model.SmartBlockType_ChatDerivedObject, ""), nil)

		_, err := fx.CreateDiscussion(context.Background(), testSpaceId, testChatId, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "object_id", apiErr.Issues[0].Path)
		require.Len(t, apiErr.Issues[0].SeeAlso, 1)
		assert.Equal(t, v2model.RefGetChatMessages(testSpaceId, testChatId), apiErr.Issues[0].SeeAlso[0])
	})

	t.Run("a discussion itself is refused: the id already is the chat_id", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, testDiscussionId).Return(pageRead(model.SmartBlockType_DiscussionObject, ""), nil)

		_, err := fx.CreateDiscussion(context.Background(), testSpaceId, testDiscussionId, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, "chat_id")
	})

	t.Run("a system or schema object cannot hold one", func(t *testing.T) {
		for _, sbType := range []model.SmartBlockType{model.SmartBlockType_STType, model.SmartBlockType_Template, model.SmartBlockType_Participant, model.SmartBlockType_Workspace} {
			fx := newV2Fixture(t)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "sys1").Return(pageRead(sbType, ""), nil)

			_, err := fx.CreateDiscussion(context.Background(), testSpaceId, "sys1", false)

			apiErr := v2Err(t, err)
			assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code, sbType.String())
			assert.Contains(t, apiErr.Message, "cannot hold a discussion", sbType.String())
		}
	})

	t.Run("an unknown object is a 404, a deleted one too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "missing").Return(apicore.ObjectRead{}, treeUnknownErr())
		_, err := fx.CreateDiscussion(context.Background(), testSpaceId, "missing", false)
		requireV2Code(t, err, v2model.CodeNotFound)

		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "gone").Return(pageRead(model.SmartBlockType_Page, ""), nil)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:        domain.String("gone"),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		}})
		_, err = fx.CreateDiscussion(context.Background(), testSpaceId, "gone", false)
		requireV2Code(t, err, v2model.CodeNotFound)
	})

	t.Run("a read-only grant is refused before the object is read", func(t *testing.T) {
		// given: no ReadObject expectation — the verb gate runs first
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateDiscussion(grantCtx(util.GrantPermsRead, testSpaceId), testSpaceId, "obj1", false)

		// then
		requireV2Code(t, err, v2model.CodeWriteNotGranted)
	})

	t.Run("a mint failure surfaces — the RPC absorbs the concurrent-mint case itself, so nothing is masked here", func(t *testing.T) {
		// given: ObjectAddDiscussion recovers an already-existing derived
		// tree and finishes the parent link (core/block/service.go), so an
		// error from it is a real one; the service must not re-read and
		// answer a cached id that may never have been persisted
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(pageRead(model.SmartBlockType_Page, ""), nil).Once()
		fx.mwMock.EXPECT().ObjectAddDiscussion(mock.Anything, mock.Anything).Return(&pb.RpcObjectDiscussionAddResponse{
			Error: &pb.RpcObjectDiscussionAddResponseError{Code: pb.RpcObjectDiscussionAddResponseError_UNKNOWN_ERROR, Description: "get space: boom"},
		})

		// when
		_, err := fx.CreateDiscussion(context.Background(), testSpaceId, "obj1", false)

		// then
		requireV2Code(t, err, v2model.CodeInternalError)
	})

	t.Run("an archived object is refused before any mint, dry run included", func(t *testing.T) {
		// given: no ObjectAddDiscussion expectation
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "arch1").Return(pageRead(model.SmartBlockType_Page, ""), nil).Times(2)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:         domain.String("arch1"),
			bundle.RelationKeyIsArchived: domain.Bool(true),
		}})

		// when / then
		for _, dryRun := range []bool{false, true} {
			_, err := fx.CreateDiscussion(context.Background(), testSpaceId, "arch1", dryRun)
			apiErr := v2Err(t, err)
			assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
			assert.Contains(t, apiErr.Message, "archived")
		}
	})

	t.Run("refusals win over an existing id: an archived or deleted object that already has one is still refused", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "arch2").Return(pageRead(model.SmartBlockType_Page, testDiscussionId), nil).Once()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "gone2").Return(pageRead(model.SmartBlockType_Page, testDiscussionId), nil).Once()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type2").Return(pageRead(model.SmartBlockType_STType, testDiscussionId), nil).Once()
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("arch2"), bundle.RelationKeyIsArchived: domain.Bool(true)},
			{bundle.RelationKeyId: domain.String("gone2"), bundle.RelationKeyIsDeleted: domain.Bool(true)},
		})

		_, err := fx.CreateDiscussion(context.Background(), testSpaceId, "arch2", true)
		requireV2Code(t, err, v2model.CodeValidationFailed)
		_, err = fx.CreateDiscussion(context.Background(), testSpaceId, "gone2", false)
		requireV2Code(t, err, v2model.CodeNotFound)
		_, err = fx.CreateDiscussion(context.Background(), testSpaceId, "type2", false)
		requireV2Code(t, err, v2model.CodeValidationFailed)
	})

	t.Run("a grant for another space is refused before the object is read", func(t *testing.T) {
		fx := newV2Fixture(t)

		_, err := fx.CreateDiscussion(grantCtx(util.GrantPermsReadWrite, "spaceB"), testSpaceId, "obj1", false)

		requireV2Code(t, err, v2model.CodeSpaceNotGranted)
	})

	t.Run("a space whose ACL makes this account a reader is a 403, not a 500", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(pageRead(model.SmartBlockType_Page, ""), nil).Once()
		fx.mwMock.EXPECT().ObjectAddDiscussion(mock.Anything, mock.Anything).Return(&pb.RpcObjectDiscussionAddResponse{
			Error: &pb.RpcObjectDiscussionAddResponseError{Code: pb.RpcObjectDiscussionAddResponseError_UNKNOWN_ERROR, Description: "set discussionId on parent object: apply: " + list.ErrInsufficientPermissions.Error()},
		}).Once()

		_, err := fx.CreateDiscussion(context.Background(), testSpaceId, "obj1", false)

		requireV2Code(t, err, v2model.CodeForbidden)
	})

	t.Run("a permanently deleted object whose tree storage is gone is a 404, not a 500", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "purged").Return(apicore.ObjectRead{}, fmt.Errorf("load: %w", spacestorage.ErrTreeStorageAlreadyDeleted))

		_, err := fx.CreateDiscussion(context.Background(), testSpaceId, "purged", false)

		requireV2Code(t, err, v2model.CodeNotFound)
	})
}

func TestV2DiscussionIsAChatForTheMessageOperations(t *testing.T) {
	t.Run("the messages read accepts a discussion id as chat_id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatGetMessagesRequest) bool {
			return req.ChatObjectId == testDiscussionId
		})).Return(&pb.RpcChatGetMessagesResponse{Messages: []*model.ChatMessage{chatProtoMessage()}, MessageCount: 1})

		// when
		got, err := fx.GetChatMessages(context.Background(), testSpaceId, testDiscussionId, ChatMessagesQuery{Limit: 25})

		// then
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
	})

	t.Run("a post into a discussion is stored as one text block, not as content", func(t *testing.T) {
		// given: the desktop's discussion composer writes blocks only and its
		// renderer reads blocks first — a content-only post would be the
		// odd one out in the thread
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		var sent *model.ChatMessage
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			sent = req.Message
			return req.ChatObjectId == testDiscussionId
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

		// when
		got, err := fx.AddChatMessage(context.Background(), testSpaceId, testDiscussionId, v2model.AddChatMessageRequest{Text: "**agreed**", ReplyTo: "msg1"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "msgNew", got.Id)
		require.NotNil(t, sent)
		require.NotNil(t, sent.Message, "an EMPTY content object rides beside the blocks: the store serializer dereferences it, and the desktop composer writes the same")
		assert.Empty(t, sent.Message.Text)
		assert.Empty(t, sent.Message.Marks)
		assert.Equal(t, "msg1", sent.ReplyToMessageId, "reply_to is the thread reply")
		require.Len(t, sent.Blocks, 1)
		block := sent.Blocks[0].GetText()
		require.NotNil(t, block)
		assert.Equal(t, "agreed", block.Text)
		assert.Equal(t, model.BlockContentText_Paragraph, block.Style)
		require.Len(t, block.Marks, 1, "marks parse into the block, as they do into content")
		assert.Equal(t, model.BlockContentTextMark_Bold, block.Marks[0].Type)
	})

	t.Run("a multi-line post becomes one paragraph block per line, marks clipped and rebased in UTF-16", func(t *testing.T) {
		// given: the desktop discussion renderer shows no break for a
		// newline inside a block, so lines have to be blocks; the emoji is
		// two UTF-16 units, which is what the ranges count
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		var sent *model.ChatMessage
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			sent = req.Message
			return true
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testDiscussionId,
			v2model.AddChatMessageRequest{Text: "**bold\nline** two\n\n🎉 <mention object_id=\"obj1\">m</mention>"}, false)

		// then
		require.NoError(t, err)
		require.Len(t, sent.Blocks, 3, "the blank line is a paragraph break, not an empty block")
		first, second, third := sent.Blocks[0].GetText(), sent.Blocks[1].GetText(), sent.Blocks[2].GetText()
		assert.Equal(t, "bold", first.Text)
		require.Len(t, first.Marks, 1)
		assert.Equal(t, &model.Range{From: 0, To: 4}, first.Marks[0].Range, "the bold mark spanning the newline is clipped to this line")
		assert.Equal(t, "line two", second.Text)
		require.Len(t, second.Marks, 1)
		assert.Equal(t, &model.Range{From: 0, To: 4}, second.Marks[0].Range, "…and rebased on the next")
		assert.Equal(t, "🎉 m", third.Text)
		require.Len(t, third.Marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Mention, third.Marks[0].Type)
		assert.Equal(t, &model.Range{From: 3, To: 4}, third.Marks[0].Range, "the emoji counts two units")
	})

	t.Run("an attachments-only post into a discussion carries no block", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("file1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_image)),
		}})
		var sent *model.ChatMessage
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			sent = req.Message
			return true
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testDiscussionId, v2model.AddChatMessageRequest{Attachments: []string{"file1"}}, false)

		require.NoError(t, err)
		assert.Empty(t, sent.Blocks, "an empty text block would render as an empty paragraph")
		require.Len(t, sent.Attachments, 1)
	})

	t.Run("a post into a space chat stays content — the desktop chat UI reads content today", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		var sent *model.ChatMessage
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			sent = req.Message
			return true
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId, v2model.AddChatMessageRequest{Text: "hi"}, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, sent.Message)
		assert.Equal(t, "hi", sent.Message.Text)
		assert.Empty(t, sent.Blocks)
	})

	t.Run("an edit in a discussion replaces the blocks with the text — dry run first, then exactly one write", func(t *testing.T) {
		// given: a desktop post with a text block and a quote block
		existingPost := func() *model.ChatMessage {
			return &model.ChatMessage{Id: "msg1", Message: &model.ChatMessageMessageContent{}, Blocks: []*model.ChatMessageMessageBlock{
				{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{Text: "old"}}},
				{Content: &model.ChatMessageMessageBlockContentOfEditorQuote{EditorQuote: &model.ChatMessageMessageBlockEditorQuote{BlockId: "b1", Content: &model.ChatMessageMessageBlockText{Text: "quoted"}}}},
			}, Attachments: []*model.ChatMessageAttachment{{Target: "file1", Type: model.ChatMessageAttachment_FILE}}}
		}

		// the dry run: its OWN fixture, with no edit expectation at all — a
		// write during a preview would fail the strict mock
		dryFx := newV2Fixture(t)
		dryFx.addDiscussion(t, testDiscussionId)
		dryFx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{existingPost()}}).Once()
		dry, err := dryFx.EditChatMessage(context.Background(), testSpaceId, testDiscussionId, "msg1", v2model.EditChatMessageRequest{Text: "**new**"}, true)
		require.NoError(t, err)
		assert.True(t, dry.DryRun)

		// the real edit: exactly one write
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{existingPost()}}).Once()
		var sent *model.ChatMessage
		fx.mwMock.EXPECT().ChatEditMessageContent(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatEditMessageContentRequest) bool {
			sent = req.EditedMessage
			return req.ChatObjectId == testDiscussionId && req.MessageId == "msg1"
		})).Return(&pb.RpcChatEditMessageContentResponse{}).Once()
		got, err := fx.EditChatMessage(context.Background(), testSpaceId, testDiscussionId, "msg1", v2model.EditChatMessageRequest{Text: "**new**"}, false)

		// then
		require.NoError(t, err)
		assert.False(t, got.DryRun)
		require.NotNil(t, sent)
		require.NotNil(t, sent.Message)
		assert.Empty(t, sent.Message.Text, "an empty content object beside the blocks, never nil")
		require.Len(t, sent.Blocks, 1, "the quote block the new text does not spell out is gone")
		assert.Equal(t, "new", sent.Blocks[0].GetText().Text)
		require.Len(t, sent.Blocks[0].GetText().Marks, 1, "marks land in the replacement block")
		assert.Equal(t, existingPost().Attachments, sent.Attachments, "attachments survive")
		for _, result := range []*v2model.ChatMessageResult{dry, got} {
			require.Len(t, result.Warnings, 1, "the dropped quote is warned about on the dry run and the receipt alike")
			assert.Equal(t, "/text", result.Warnings[0].Path)
			assert.Contains(t, result.Warnings[0].Message, "1 quote block(s)")
		}
	})

	t.Run("the loss warning counts every kind the text cannot express, exactly", func(t *testing.T) {
		// given: two quotes (editor + message), a link, an embed, two styled
		// text blocks, one paragraph and a nil entry
		existing := &model.ChatMessage{Id: "msg1", Message: &model.ChatMessageMessageContent{}, Blocks: []*model.ChatMessageMessageBlock{
			{Content: &model.ChatMessageMessageBlockContentOfEditorQuote{EditorQuote: &model.ChatMessageMessageBlockEditorQuote{BlockId: "b1", Content: &model.ChatMessageMessageBlockText{Text: "q1"}}}},
			{Content: &model.ChatMessageMessageBlockContentOfMessageQuote{MessageQuote: &model.ChatMessageMessageBlockMessageQuote{MessageId: "m0", ParticipantId: "p", Content: &model.ChatMessageMessageBlockText{Text: "q2"}}}},
			{Content: &model.ChatMessageMessageBlockContentOfLink{Link: &model.ChatMessageMessageBlockLink{TargetObjectId: "file1"}}},
			{Content: &model.ChatMessageMessageBlockContentOfEmbed{Embed: &model.ChatMessageMessageBlockEmbed{Text: "x^2"}}},
			{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{Text: "Heading", Style: model.BlockContentText_Header1}}},
			{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{Text: "code", Style: model.BlockContentText_Code}}},
			{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{Text: "plain"}}},
			nil,
		}}
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{existing}}).Once()

		// when
		got, err := fx.EditChatMessage(context.Background(), testSpaceId, testDiscussionId, "msg1", v2model.EditChatMessageRequest{Text: "new"}, true)

		// then
		require.NoError(t, err)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "2 quote block(s), 1 link block(s), 1 embed block(s), 2 styled text block(s)")
	})

	t.Run("a space-chat edit keeps its blocks and warns about nothing", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		existing := chatProtoMessage()
		existing.Blocks = []*model.ChatMessageMessageBlock{
			{Content: &model.ChatMessageMessageBlockContentOfEditorQuote{EditorQuote: &model.ChatMessageMessageBlockEditorQuote{BlockId: "b1", Content: &model.ChatMessageMessageBlockText{Text: "quoted"}}}},
		}
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{existing}}).Once()

		got, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "msg1", v2model.EditChatMessageRequest{Text: "new"}, true)

		require.NoError(t, err)
		assert.Empty(t, got.Warnings)
	})

	t.Run("a text of newlines only is refused on the dry run as on the real call (C9)", func(t *testing.T) {
		// given: it parses fine and makes no block, and a message with no
		// block and no attachment is what the store refuses — so the dry
		// run must not predict a 201
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{
			{Id: "msg1", Message: &model.ChatMessageMessageContent{}, Blocks: []*model.ChatMessageMessageBlock{{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{Text: "old"}}}}},
		}}).Times(2)

		// when / then: no ChatAddMessage / ChatEditMessageContent expectation
		for _, dryRun := range []bool{true, false} {
			_, err := fx.AddChatMessage(context.Background(), testSpaceId, testDiscussionId, v2model.AddChatMessageRequest{Text: "\n\n"}, dryRun)
			requireV2Code(t, err, v2model.CodeValidationFailed)
			_, err = fx.EditChatMessage(context.Background(), testSpaceId, testDiscussionId, "msg1", v2model.EditChatMessageRequest{Text: "\n"}, dryRun)
			requireV2Code(t, err, v2model.CodeValidationFailed)
		}
		// …while a space chat, which stores content, still takes it
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.Anything).Return(&pb.RpcChatAddMessageResponse{MessageId: "m"}).Once()
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId, v2model.AddChatMessageRequest{Text: "\n"}, false)
		require.NoError(t, err)
	})

	t.Run("line boundaries: a one-unit mark, a trailing newline, and mark parameters survive the split", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		var sent *model.ChatMessage
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			sent = req.Message
			return true
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "m"}).Once()

		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testDiscussionId,
			v2model.AddChatMessageRequest{Text: "**a**\n[b](https://example.com)\n"}, false)

		require.NoError(t, err)
		require.Len(t, sent.Blocks, 2, "a trailing newline makes no empty block")
		first, second := sent.Blocks[0].GetText(), sent.Blocks[1].GetText()
		require.Len(t, first.Marks, 1, "a mark of exactly one unit at the line end is kept")
		assert.Equal(t, &model.Range{From: 0, To: 1}, first.Marks[0].Range)
		require.Len(t, second.Marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Link, second.Marks[0].Type)
		assert.Equal(t, "https://example.com", second.Marks[0].Param, "the mark's parameter survives the clip")
		assert.Equal(t, &model.Range{From: 0, To: 1}, second.Marks[0].Range)
	})

	t.Run("deleting a discussion post warns about link-block targets too", func(t *testing.T) {
		// given: a desktop post carries its file in a link block, not in
		// attachments — and the middleware garbage-collects both
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{
			{Id: "msg1", Message: &model.ChatMessageMessageContent{}, Blocks: []*model.ChatMessageMessageBlock{
				{Content: &model.ChatMessageMessageBlockContentOfLink{Link: &model.ChatMessageMessageBlockLink{TargetObjectId: "file9"}}},
			}},
		}}).Once()

		got, err := fx.DeleteChatMessage(context.Background(), testSpaceId, testDiscussionId, "msg1", true)

		require.NoError(t, err)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "file9")
	})

	t.Run("an edit that drops nothing warns about nothing", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		existing := &model.ChatMessage{Id: "msg1", Message: &model.ChatMessageMessageContent{}, Blocks: []*model.ChatMessageMessageBlock{
			{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{Text: "old"}}},
		}}
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{existing}})

		got, err := fx.EditChatMessage(context.Background(), testSpaceId, testDiscussionId, "msg1", v2model.EditChatMessageRequest{Text: "new"}, true)

		require.NoError(t, err)
		assert.Empty(t, got.Warnings)
	})

	t.Run("a discussion in another space is not reachable under this space's path", func(t *testing.T) {
		// given: the discussion is indexed in space B only; a key granted
		// space A asks for it under A
		fx := newV2Fixture(t)
		fx.registerSpace(t, "spaceB")
		fx.objectStore.AddObjects(t, "spaceB", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String(testDiscussionId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_discussion)),
		}})
		ctx := grantCtx(util.GrantPermsReadWrite, testSpaceId)

		// when: no RPC expectation — a lookup that crossed spaces would call one
		_, readErr := fx.GetChatMessages(ctx, testSpaceId, testDiscussionId, ChatMessagesQuery{})
		_, writeErr := fx.AddChatMessage(ctx, testSpaceId, testDiscussionId, v2model.AddChatMessageRequest{Text: "hi"}, false)
		_, otherSpaceErr := fx.AddChatMessage(ctx, "spaceB", testDiscussionId, v2model.AddChatMessageRequest{Text: "hi"}, false)

		// then
		requireV2Code(t, readErr, v2model.CodeNotFound)
		requireV2Code(t, writeErr, v2model.CodeNotFound)
		requireV2Code(t, otherSpaceErr, v2model.CodeSpaceNotGranted)
	})

	t.Run("a discussion post reads back as text", func(t *testing.T) {
		// given: the store holds what the post above wrote
		fx := newV2Fixture(t)
		fx.addDiscussion(t, testDiscussionId)
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesResponse{Messages: []*model.ChatMessage{{
			Id: "msg1", OrderId: "00a1",
			Blocks: []*model.ChatMessageMessageBlock{{Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{Text: "agreed"}}}},
		}}})

		// when
		got, err := fx.GetChatMessages(context.Background(), testSpaceId, testDiscussionId, ChatMessagesQuery{})

		// then
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
		assert.Equal(t, "agreed", got.Messages[0].Text)
		assert.Empty(t, got.Messages[0].BlocksText)
	})

	t.Run("a discussion is not listed among the space chats", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.addDiscussion(t, testDiscussionId)

		// when
		rows, total, _, err := fx.ListChats(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, testChatId, rows[0].Id)
	})

	t.Run("a page id sent as chat_id is steered to create_discussion", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("page1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		// when
		_, err := fx.GetChatMessages(context.Background(), testSpaceId, "page1", ChatMessagesQuery{})

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].SeeAlso, v2model.RefCreateDiscussion(testSpaceId, "page1"))
	})

	t.Run("a layout that cannot hold a discussion is not steered to create_discussion", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})

		_, err := fx.GetChatMessages(context.Background(), testSpaceId, "type1", ChatMessagesQuery{})

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.NotContains(t, apiErr.Issues[0].SeeAlso, v2model.RefCreateDiscussion(testSpaceId, "type1"), "following that hint would 400 again")
		assert.Contains(t, apiErr.Issues[0].SeeAlso, v2model.RefListChats(testSpaceId))
	})

	t.Run("a template or an archived page is not steered to create_discussion either", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("tpl1"), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)), bundle.RelationKeyTargetObjectType: domain.String("ot-page")},
			{bundle.RelationKeyId: domain.String("arch3"), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)), bundle.RelationKeyIsArchived: domain.Bool(true)},
		})

		for _, id := range []string{"tpl1", "arch3"} {
			_, err := fx.GetChatMessages(context.Background(), testSpaceId, id, ChatMessagesQuery{})
			apiErr := v2Err(t, err)
			require.Len(t, apiErr.Issues, 1, id)
			assert.NotContains(t, apiErr.Issues[0].SeeAlso, v2model.RefCreateDiscussion(testSpaceId, id), id)
		}
	})

	t.Run("an unknown chat_id names both ways to find one", func(t *testing.T) {
		fx := newV2Fixture(t)

		_, err := fx.GetChatMessages(context.Background(), testSpaceId, "nope", ChatMessagesQuery{})

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].SeeAlso, v2model.RefListChats(testSpaceId))
		assert.Contains(t, apiErr.Issues[0].SeeAlso, v2model.RefCreateDiscussion(testSpaceId, ""))
	})
}

func TestV2ObjectReadServesTheDiscussionMember(t *testing.T) {
	t.Run("present on every read shape when the object has one, absent otherwise", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(pageRead(model.SmartBlockType_Page, testDiscussionId), nil).Times(3)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj2").Return(pageRead(model.SmartBlockType_Page, ""), nil).Once()

		// when / then
		for _, q := range []ObjectQuery{{}, {Outline: true}, {Include: "properties"}} {
			body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", q)
			require.NoError(t, err)
			assert.Equal(t, testDiscussionId, decodeBody(t, body)["discussion"], "shape %+v", q)
		}
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj2", ObjectQuery{})
		require.NoError(t, err)
		assert.NotContains(t, decodeBody(t, body), "discussion")
	})

	t.Run("the markdown shape carries it too", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(pageRead(model.SmartBlockType_Page, testDiscussionId), nil)
		fx.mwMock.EXPECT().ObjectExport(mock.Anything, mock.Anything).Return(&pb.RpcObjectExportResponse{Result: "# Doc"})

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", ObjectQuery{Format: "md"})

		// then
		require.NoError(t, err)
		assert.Equal(t, testDiscussionId, decodeBody(t, body)["discussion"])
	})

	t.Run("a read body writes back: the create path strips the member", func(t *testing.T) {
		// given
		body := []byte(`{"formatVersion":"2.0","type":"page","discussion":"` + testDiscussionId + `","etag":"e","warnings":[]}`)

		// when
		normalized, err := normalizeCreateBody(body)

		// then
		require.NoError(t, err)
		assert.JSONEq(t, `{"formatVersion":"2.0","type":"page"}`, string(normalized))
	})

	t.Run("the served document schema declares it output-only", func(t *testing.T) {
		// given
		var served map[string]any
		require.NoError(t, json.Unmarshal(apiV2DocumentSchema(), &served))

		// then
		member, ok := served["properties"].(map[string]any)["discussion"].(map[string]any)
		require.True(t, ok, "the served schema must declare the member a read carries")
		assert.Equal(t, true, member["x-output-only"])
		assert.Equal(t, "string", member["type"])
	})
}

// treeUnknownErr is the reader's answer for an id no tree has.
func treeUnknownErr() error { return treestorage.ErrUnknownTreeId }
