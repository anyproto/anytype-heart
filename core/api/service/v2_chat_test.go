package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	testChatId   = "chat1"
	testIdentity = "identityA"
)

// requireV2Code asserts err is a C6 error with the given code.
func requireV2Code(t *testing.T, err error, wantCode string) {
	t.Helper()
	assert.Equal(t, wantCode, v2Err(t, err).Code)
}

// addChat registers a chat object (chatDerived layout) in the store.
func (fx *v2Fixture) addChat(t *testing.T, id, name string, lastModified int64) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyName:             domain.String(name),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_chatDerived)),
		bundle.RelationKeyLastModifiedDate: domain.Int64(lastModified),
	}})
}

// addParticipant registers the participant object the author-name
// enrichment resolves (deterministic id, store-backed — no subscriptions).
func (fx *v2Fixture) addParticipant(t *testing.T, identity, name string) string {
	participantId := domain.NewParticipantId(testSpaceId, identity)
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(participantId),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
	}})
	return participantId
}

func chatProtoMessage() *model.ChatMessage {
	return &model.ChatMessage{
		Id:        "msg1",
		OrderId:   "00a1",
		Creator:   testIdentity,
		CreatedAt: 1717405200,
		Message: &model.ChatMessageMessageContent{
			Text:  "can you check the doc?",
			Style: model.BlockContentText_Paragraph,
			Marks: []*model.BlockContentTextMark{{
				Range: &model.Range{From: 8, To: 13},
				Type:  model.BlockContentTextMark_Bold,
			}},
		},
		Reactions: &model.ChatMessageReactions{
			Reactions: map[string]*model.ChatMessageReactionsIdentityList{
				"👍": {Ids: []string{testIdentity, "identityB"}},
			},
		},
	}
}

func TestV2ListChats(t *testing.T) {
	t.Run("C5 rows via the store — no chat opens, hidden and non-chat excluded", func(t *testing.T) {
		// given: the mock middleware has NO expectations — any RPC (a chat
		// open, a subscription) would fail the test, which is the phase's
		// no-chat-opens guarantee (GO-7302)
		fx := newV2Fixture(t)
		fx.addChat(t, "chatB", "Team chat", 2000)
		fx.addChat(t, "chatA", "Old chat", 1000)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("hiddenChat"),
				bundle.RelationKeyName:           domain.String("Hidden"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_chatDerived)),
				bundle.RelationKeyIsHidden:       domain.Bool(true),
			},
			{
				bundle.RelationKeyId:             domain.String("page1"),
				bundle.RelationKeyName:           domain.String("A page"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
		})
		want := []apimodel.V2ChatRow{
			{Id: "chatB", Name: "Team chat"},
			{Id: "chatA", Name: "Old chat"},
		}

		// when
		rows, total, hasMore, err := fx.ListChats(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows, "rows are {id,name}, newest-modified first — no type object, no counters (Q3)")
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
	})

	t.Run("pagination reports has_more with an honest total", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, "chatB", "B", 2000)
		fx.addChat(t, "chatA", "A", 1000)

		// when
		rows, total, hasMore, err := fx.ListChats(context.Background(), testSpaceId, 0, 1)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, 2, total)
		assert.True(t, hasMore)
	})

	t.Run("unknown space is a 404", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, _, _, err := fx.ListChats(context.Background(), "nope", 0, 25)
		requireV2Code(t, err, apimodel.V2CodeNotFound)
	})
}

func TestV2CreateChat(t *testing.T) {
	t.Run("thin ObjectCreate with the chatDerived type", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().ObjectCreate(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRequest) bool {
			return req.SpaceId == testSpaceId &&
				req.ObjectTypeUniqueKey == bundle.TypeKeyChatDerived.URL() &&
				req.Details.GetFields()["name"].GetStringValue() == "Project chat"
		})).Return(&pb.RpcObjectCreateResponse{ObjectId: "chatNew"})
		want := &apimodel.V2ChatResult{Id: "chatNew", Name: "Project chat"}

		// when
		got, err := fx.CreateChat(context.Background(), testSpaceId, apimodel.V2CreateChatRequest{Name: "Project chat"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("dry run commits nothing", func(t *testing.T) {
		// given: no ObjectCreate expectation — a call would fail the test
		fx := newV2Fixture(t)

		// when
		got, err := fx.CreateChat(context.Background(), testSpaceId, apimodel.V2CreateChatRequest{Name: "Project chat"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
		assert.Empty(t, got.Id)
	})

	t.Run("empty name is a 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, err := fx.CreateChat(context.Background(), testSpaceId, apimodel.V2CreateChatRequest{}, false)
		requireV2Code(t, err, apimodel.V2CodeValidationFailed)
	})
}

func TestV2GetChatMessages(t *testing.T) {
	t.Run("state and messageCount pass through — the fields v1 dropped", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.addParticipant(t, testIdentity, "Alice")
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatGetMessagesRequest) bool {
			return req.ChatObjectId == testChatId && req.AfterOrderId == "0090" && req.Limit == 25
		})).Return(&pb.RpcChatGetMessagesResponse{
			Messages: []*model.ChatMessage{chatProtoMessage()},
			ChatState: &model.ChatState{
				Messages:    &model.ChatStateUnreadState{OldestOrderId: "00a1", Counter: 3},
				Mentions:    &model.ChatStateUnreadState{OldestOrderId: "00a1", Counter: 1},
				LastStateId: "state42",
			},
			MessageCount: 812,
		})

		// when
		got, err := fx.GetChatMessages(context.Background(), testSpaceId, testChatId, V2ChatMessagesQuery{After: "0090", Limit: 25})

		// then
		require.NoError(t, err)
		assert.Equal(t, 812, got.MessageCount, "messageCount must pass through (the peek)")
		require.NotNil(t, got.State, "chatState must pass through")
		assert.Equal(t, 3, got.State.UnreadMessages)
		assert.Equal(t, 1, got.State.UnreadMentions)
		assert.Equal(t, "state42", got.State.LastStateId,
			"lastStateId must reach the client — without it the mark-read race guard is unreachable")

		require.Len(t, got.Messages, 1)
		msg := got.Messages[0]
		assert.Equal(t, "can you **check** the doc?", msg.Text, "text is §8 markup")
		assert.Equal(t, domain.NewParticipantId(testSpaceId, testIdentity), msg.AuthorId)
		assert.Equal(t, "Alice", msg.Author, "author name enriched from the store participant")
		assert.Equal(t, map[string]int{"👍": 2}, msg.Reactions, "counts by default (Q4)")
	})

	t.Run("reactions=full restores identity lists as participant ids", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesResponse{Messages: []*model.ChatMessage{chatProtoMessage()}})

		// when
		got, err := fx.GetChatMessages(context.Background(), testSpaceId, testChatId, V2ChatMessagesQuery{Limit: 25, FullReactions: true})

		// then
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
		want := map[string][]string{"👍": {
			domain.NewParticipantId(testSpaceId, testIdentity),
			domain.NewParticipantId(testSpaceId, "identityB"),
		}}
		assert.Equal(t, want, got.Messages[0].Reactions)
	})

	t.Run("a non-chat target is a 400 naming the layout", func(t *testing.T) {
		// given: no RPC expectation — the guard must fire before the RPC
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("page1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		// when
		_, err := fx.GetChatMessages(context.Background(), testSpaceId, "page1", V2ChatMessagesQuery{Limit: 25})

		// then
		requireV2Code(t, err, apimodel.V2CodeValidationFailed)
		assert.Contains(t, err.Error(), "not a chat")
	})

	t.Run("an unknown chat is a 404", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, err := fx.GetChatMessages(context.Background(), testSpaceId, "nope", V2ChatMessagesQuery{Limit: 25})
		requireV2Code(t, err, apimodel.V2CodeNotFound)
	})
}

func TestV2AddChatMessage(t *testing.T) {
	t.Run("text parses as §8 markup — the RPC receives text plus marks", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			msg := req.Message
			return req.ChatObjectId == testChatId &&
				msg.Message.Text == "can you check the doc?" &&
				len(msg.Message.Marks) == 1 &&
				msg.Message.Marks[0].Type == model.BlockContentTextMark_Bold &&
				msg.Message.Marks[0].Range.From == 8 && msg.Message.Marks[0].Range.To == 13 &&
				msg.ReplyToMessageId == "msg0"
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

		// when
		got, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			apimodel.V2AddChatMessageRequest{Text: "can you **check** the doc?", ReplyTo: "msg0"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "msgNew", got.Id)
	})

	t.Run("attachment kinds infer from the target layout", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("img1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_image)),
			},
			{
				bundle.RelationKeyId:             domain.String("pdf1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_pdf)),
			},
			{
				bundle.RelationKeyId:             domain.String("page1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
		})
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			atts := req.Message.Attachments
			return len(atts) == 3 &&
				atts[0].Target == "img1" && atts[0].Type == model.ChatMessageAttachment_IMAGE &&
				atts[1].Target == "pdf1" && atts[1].Type == model.ChatMessageAttachment_FILE &&
				atts[2].Target == "page1" && atts[2].Type == model.ChatMessageAttachment_LINK
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			apimodel.V2AddChatMessageRequest{Text: "see these", Attachments: []string{"img1", "pdf1", "page1"}}, false)

		// then
		require.NoError(t, err)
	})

	t.Run("an unknown attachment id is a path-addressed 400", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			apimodel.V2AddChatMessageRequest{Text: "see this", Attachments: []string{"missing1"}}, false)

		// then
		requireV2Code(t, err, apimodel.V2CodeValidationFailed)
		var v2Err *apimodel.V2Error
		require.ErrorAs(t, err, &v2Err)
		require.Len(t, v2Err.Issues, 1)
		assert.Equal(t, "/attachments/0", v2Err.Issues[0].Path)
	})

	t.Run("dry run validates everything and sends nothing", func(t *testing.T) {
		// given: no ChatAddMessage expectation — a send would fail the test
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when
		got, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			apimodel.V2AddChatMessageRequest{Text: "hello"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
	})

	t.Run("empty text with no attachments is a 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId, apimodel.V2AddChatMessageRequest{}, false)
		requireV2Code(t, err, apimodel.V2CodeValidationFailed)
	})
}

func TestV2EditChatMessage(t *testing.T) {
	t.Run("text-only merge preserves attachments, style and blocks", func(t *testing.T) {
		// given: the middleware edit replaces the whole content, so the
		// service must carry the existing attachments through — dropping
		// this read-merge would wipe them on every text edit
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		existing := chatProtoMessage()
		existing.Message.Style = model.BlockContentText_Quote
		existing.Attachments = []*model.ChatMessageAttachment{{Target: "file1", Type: model.ChatMessageAttachment_IMAGE}}
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, &pb.RpcChatGetMessagesByIdsRequest{
			ChatObjectId: testChatId, MessageIds: []string{"msg1"},
		}).Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{existing}})
		fx.mwMock.EXPECT().ChatEditMessageContent(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatEditMessageContentRequest) bool {
			msg := req.EditedMessage
			return req.MessageId == "msg1" &&
				msg.Message.Text == "updated text" &&
				msg.Message.Style == model.BlockContentText_Quote &&
				len(msg.Attachments) == 1 && msg.Attachments[0].Target == "file1"
		})).Return(&pb.RpcChatEditMessageContentResponse{})

		// when
		got, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "msg1",
			apimodel.V2EditChatMessageRequest{Text: "updated text"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "msg1", got.Id)
	})

	t.Run("editing a missing message is a 404", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{})

		// when
		_, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "nope",
			apimodel.V2EditChatMessageRequest{Text: "updated"}, false)

		// then
		requireV2Code(t, err, apimodel.V2CodeNotFound)
	})

	t.Run("dry run stops after the existence check", func(t *testing.T) {
		// given: no ChatEditMessageContent expectation
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatProtoMessage()}})

		// when
		got, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "msg1",
			apimodel.V2EditChatMessageRequest{Text: "updated"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
	})
}

func TestV2DeleteChatMessage(t *testing.T) {
	t.Run("delete passes through", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatDeleteMessage(mock.Anything, &pb.RpcChatDeleteMessageRequest{
			ChatObjectId: testChatId, MessageId: "msg1",
		}).Return(&pb.RpcChatDeleteMessageResponse{})

		// when
		got, err := fx.DeleteChatMessage(context.Background(), testSpaceId, testChatId, "msg1", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "msg1", got.Id)
	})

	t.Run("dry run is an existence check — a missing message 404s", func(t *testing.T) {
		// given: no ChatDeleteMessage expectation
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{})

		// when
		_, err := fx.DeleteChatMessage(context.Background(), testSpaceId, testChatId, "nope", true)

		// then
		requireV2Code(t, err, apimodel.V2CodeNotFound)
	})
}

func TestV2ToggleChatReaction(t *testing.T) {
	t.Run("toggle passes the outcome through", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatToggleMessageReaction(mock.Anything, &pb.RpcChatToggleMessageReactionRequest{
			ChatObjectId: testChatId, MessageId: "msg1", Emoji: "👍",
		}).Return(&pb.RpcChatToggleMessageReactionResponse{Added: true})

		// when
		got, err := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			apimodel.V2ChatReactionRequest{Emoji: "👍"}, false)

		// then
		require.NoError(t, err)
		assert.True(t, got.Added)
	})

	t.Run("dry run reports the would-be outcome without toggling", func(t *testing.T) {
		// given: the fixture's account (testAccountId) does NOT carry 👍 on
		// the message, so the would-be outcome is added=true; no toggle RPC
		// expectation — a real toggle would fail the test
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		msg := chatProtoMessage()
		msg.Reactions = &model.ChatMessageReactions{Reactions: map[string]*model.ChatMessageReactionsIdentityList{
			"👍": {Ids: []string{"identityB"}},
			"🎉": {Ids: []string{testAccountId}},
		}}
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{msg}}).Times(2)

		// when
		addOutcome, err1 := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			apimodel.V2ChatReactionRequest{Emoji: "👍"}, true)
		removeOutcome, err2 := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			apimodel.V2ChatReactionRequest{Emoji: "🎉"}, true)

		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.True(t, addOutcome.Added, "not reacted yet — the toggle would add")
		assert.True(t, addOutcome.DryRun)
		assert.False(t, removeOutcome.Added, "already reacted — the toggle would remove")
	})

	t.Run("empty emoji is a 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		_, err := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			apimodel.V2ChatReactionRequest{}, false)
		requireV2Code(t, err, apimodel.V2CodeValidationFailed)
	})
}

func TestV2ReadChat(t *testing.T) {
	t.Run("messages scope forwards upTo AND lastStateId — the race guard v1 made unreachable", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatReadMessages(mock.Anything, &pb.RpcChatReadMessagesRequest{
			ChatObjectId:  testChatId,
			Type:          pb.RpcChatReadMessages_Messages,
			BeforeOrderId: "00a5",
			LastStateId:   "state42",
		}).Return(&pb.RpcChatReadMessagesResponse{})

		// when
		got, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			apimodel.V2ChatReadRequest{UpTo: "00a5", LastStateId: "state42"}, false)

		// then
		require.NoError(t, err)
		assert.False(t, got.DryRun)
	})

	t.Run("mentions scope maps to the mentions counter", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatReadMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatReadMessagesRequest) bool {
			return req.Type == pb.RpcChatReadMessages_Mentions && req.BeforeOrderId == "00a5"
		})).Return(&pb.RpcChatReadMessagesResponse{})

		// when
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			apimodel.V2ChatReadRequest{UpTo: "00a5", Scope: "mentions"}, false)

		// then
		require.NoError(t, err)
	})

	t.Run("upTo is required — an empty bound would silently mark nothing", func(t *testing.T) {
		// given: no RPC expectation — the request must never reach the RPC
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId, apimodel.V2ChatReadRequest{}, false)

		// then
		requireV2Code(t, err, apimodel.V2CodeValidationFailed)
		var v2Err *apimodel.V2Error
		require.ErrorAs(t, err, &v2Err)
		require.NotEmpty(t, v2Err.Issues)
		assert.Equal(t, "/upTo", v2Err.Issues[0].Path)
	})

	t.Run("reactions scope marks all unread reactions", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatReadReactions(mock.Anything, &pb.RpcChatReadReactionsRequest{
			ChatObjectId: testChatId,
		}).Return(&pb.RpcChatReadReactionsResponse{})

		// when
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			apimodel.V2ChatReadRequest{Scope: "reactions"}, false)

		// then
		require.NoError(t, err)
	})

	t.Run("reactions scope rejects upTo — the backend takes no bound", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			apimodel.V2ChatReadRequest{Scope: "reactions", UpTo: "00a5"}, false)
		requireV2Code(t, err, apimodel.V2CodeValidationFailed)
	})

	t.Run("unknown scope is a 400 naming the allowed values", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			apimodel.V2ChatReadRequest{Scope: "everything", UpTo: "00a5"}, false)
		requireV2Code(t, err, apimodel.V2CodeValidationFailed)
		assert.Contains(t, err.Error(), "scope")
	})

	t.Run("dry run validates and forwards nothing", func(t *testing.T) {
		// given: no RPC expectations
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when
		got, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			apimodel.V2ChatReadRequest{UpTo: "00a5"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
	})
}
