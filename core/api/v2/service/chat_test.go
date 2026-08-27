package v2service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
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
		want := []v2model.ChatRow{
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
		// given: THREE chats and limit=1 — the fetch reads limit+1 = 2
		// records, so the banned v1 `total = len(fetched)` pattern would
		// report 2 while the honest QueryAndCount total is 3; two chats
		// could not tell the implementations apart
		fx := newV2Fixture(t)
		fx.addChat(t, "chatC", "C", 3000)
		fx.addChat(t, "chatB", "B", 2000)
		fx.addChat(t, "chatA", "A", 1000)

		// when
		rows, total, hasMore, err := fx.ListChats(context.Background(), testSpaceId, 0, 1)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, 3, total, "total must be the store count, not len(fetched) — the Phase-4-banned pattern")
		assert.True(t, hasMore)
	})

	t.Run("unknown space is a 404", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, _, _, err := fx.ListChats(context.Background(), "nope", 0, 25)
		requireV2Code(t, err, v2model.CodeNotFound)
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
		want := &v2model.ChatResult{Id: "chatNew", Name: "Project chat"}

		// when
		got, err := fx.CreateChat(context.Background(), testSpaceId, v2model.CreateChatRequest{Name: "Project chat"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("dry run commits nothing", func(t *testing.T) {
		// given: no ObjectCreate expectation — a call would fail the test
		fx := newV2Fixture(t)

		// when
		got, err := fx.CreateChat(context.Background(), testSpaceId, v2model.CreateChatRequest{Name: "Project chat"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
		assert.Empty(t, got.Id)
	})

	t.Run("empty name is a 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, err := fx.CreateChat(context.Background(), testSpaceId, v2model.CreateChatRequest{}, false)
		requireV2Code(t, err, v2model.CodeValidationFailed)
	})
}

func TestV2GetChatMessages(t *testing.T) {
	t.Run("state and message_count pass through — the fields v1 dropped", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.addParticipant(t, testIdentity, "Alice")
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatGetMessagesRequest) bool {
			// limit+1: the extra record detects has_more without guessing
			return req.ChatObjectId == testChatId && req.AfterOrderId == "0090" && req.Limit == 26
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
		got, err := fx.GetChatMessages(context.Background(), testSpaceId, testChatId, ChatMessagesQuery{After: "0090", Limit: 25})

		// then
		require.NoError(t, err)
		assert.Equal(t, 812, got.MessageCount, "message_count must pass through (the peek)")
		require.NotNil(t, got.State, "chatState must pass through")
		assert.Equal(t, 3, got.State.UnreadMessages)
		assert.Equal(t, 1, got.State.UnreadMentions)
		assert.Equal(t, "state42", got.State.LastStateId,
			"last_state_id must reach the client — without it the mark-read race guard is unreachable")

		require.Len(t, got.Messages, 1)
		msg := got.Messages[0]
		assert.Equal(t, "can you **check** the doc?", msg.Text, "text is §8 markup")
		assert.Equal(t, domain.NewParticipantId(testSpaceId, testIdentity), msg.AuthorId)
		assert.Equal(t, "Alice", msg.Author, "author name enriched from the store participant")
		assert.Equal(t, map[string]int{"👍": 2}, msg.Reactions, "counts by default (Q4)")
		assert.Nil(t, msg.ReactedBy, "identity lists only under ?reactions=full")
		assert.False(t, got.HasMore)
	})

	t.Run("reactions=full adds reacted_by participant ids — counts keep their slot (C2)", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesResponse{Messages: []*model.ChatMessage{chatProtoMessage()}})

		// when
		got, err := fx.GetChatMessages(context.Background(), testSpaceId, testChatId, ChatMessagesQuery{Limit: 25, FullReactions: true})

		// then
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
		want := map[string][]string{"👍": {
			domain.NewParticipantId(testSpaceId, testIdentity),
			domain.NewParticipantId(testSpaceId, "identityB"),
		}}
		assert.Equal(t, want, got.Messages[0].ReactedBy)
		assert.Equal(t, map[string]int{"👍": 2}, got.Messages[0].Reactions,
			"reactions stays the counts map in full mode — one slot, one type")
	})

	t.Run("forward paging trims the newest extra and continues with next_after", func(t *testing.T) {
		// given: ?after alone is the one ASC query — the RPC is asked for
		// limit+1 and returns 3 ascending messages for limit=2
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		protos := make([]*model.ChatMessage, 0, 3)
		for _, order := range []string{"00a1", "00a2", "00a3"} {
			m := chatProtoMessage()
			m.Id = "msg-" + order
			m.OrderId = order
			protos = append(protos, m)
		}
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatGetMessagesRequest) bool {
			return req.AfterOrderId == "0090" && req.Limit == 3
		})).Return(&pb.RpcChatGetMessagesResponse{Messages: protos})

		// when
		got, err := fx.GetChatMessages(context.Background(), testSpaceId, testChatId, ChatMessagesQuery{After: "0090", Limit: 2})

		// then
		require.NoError(t, err)
		require.Len(t, got.Messages, 2)
		assert.Equal(t, "00a1", got.Messages[0].Order)
		assert.Equal(t, "00a2", got.Messages[1].Order, "the newest extra is trimmed on a forward walk")
		assert.True(t, got.HasMore)
		assert.Equal(t, "00a2", got.NextAfter, "the cursor advances past the last shown message")
		assert.Empty(t, got.NextBefore)
	})

	t.Run("newest-anchored paging trims the oldest extra and continues with next_before", func(t *testing.T) {
		// given: no ?after — the repository sorts DESC (newest N) and the
		// service receives them ascending; the OLDEST message is the extra
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		protos := make([]*model.ChatMessage, 0, 3)
		for _, order := range []string{"00a1", "00a2", "00a3"} {
			m := chatProtoMessage()
			m.Id = "msg-" + order
			m.OrderId = order
			protos = append(protos, m)
		}
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesResponse{Messages: protos})

		// when
		got, err := fx.GetChatMessages(context.Background(), testSpaceId, testChatId, ChatMessagesQuery{Limit: 2})

		// then
		require.NoError(t, err)
		require.Len(t, got.Messages, 2)
		assert.Equal(t, "00a2", got.Messages[0].Order, "the oldest extra is trimmed on a newest-anchored read")
		assert.Equal(t, "00a3", got.Messages[1].Order)
		assert.True(t, got.HasMore)
		assert.Equal(t, "00a2", got.NextBefore, "older messages continue with ?before=")
		assert.Empty(t, got.NextAfter)
	})

	t.Run("a non-chat target is a 400 naming the layout", func(t *testing.T) {
		// given: no RPC expectation — the guard must fire before the RPC
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("page1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		// when
		_, err := fx.GetChatMessages(context.Background(), testSpaceId, "page1", ChatMessagesQuery{Limit: 25})

		// then
		requireV2Code(t, err, v2model.CodeValidationFailed)
		assert.Contains(t, err.Error(), "not a chat")
	})

	t.Run("an unknown chat is a 404", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, err := fx.GetChatMessages(context.Background(), testSpaceId, "nope", ChatMessagesQuery{Limit: 25})
		requireV2Code(t, err, v2model.CodeNotFound)
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
			v2model.AddChatMessageRequest{Text: "can you **check** the doc?", ReplyTo: "msg0"}, false)

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
			v2model.AddChatMessageRequest{Text: "see these", Attachments: []string{"img1", "pdf1", "page1"}}, false)

		// then
		require.NoError(t, err)
	})

	t.Run("an unknown attachment id is a path-addressed 400", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			v2model.AddChatMessageRequest{Text: "see this", Attachments: []string{"missing1"}}, false)

		// then
		requireV2Code(t, err, v2model.CodeValidationFailed)
		var v2Err *v2model.Error
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
			v2model.AddChatMessageRequest{Text: "hello"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
	})

	t.Run("empty text with no attachments is a 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId, v2model.AddChatMessageRequest{}, false)
		requireV2Code(t, err, v2model.CodeValidationFailed)
	})

	t.Run("text over the UTF-16 cap is a path-addressed 400 BEFORE the RPC", func(t *testing.T) {
		// given: no RPC expectation — the chat RPC enums carry no usable
		// error code, so an over-long message reaching the middleware would
		// come back as a retry-looping 500; the cap must fire in v2
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			v2model.AddChatMessageRequest{Text: strings.Repeat("a", chatmodel.MaxMessageLength+1)}, false)

		// then
		requireV2Code(t, err, v2model.CodeValidationFailed)
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		require.NotEmpty(t, v2Err.Issues)
		assert.Equal(t, "/text", v2Err.Issues[0].Path)
	})

	t.Run("more than 32 attachments is a path-addressed 400 before any lookup", func(t *testing.T) {
		// given: the cap the chatMessage schema advertises (maxItems 32)
		// must be enforced, or the strict schema lies about the contract
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		ids := make([]string, maxChatAttachments+1)
		for i := range ids {
			ids[i] = fmt.Sprintf("obj%d", i)
		}

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			v2model.AddChatMessageRequest{Text: "see these", Attachments: ids}, false)

		// then
		requireV2Code(t, err, v2model.CodeValidationFailed)
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		require.NotEmpty(t, v2Err.Issues)
		assert.Equal(t, "/attachments", v2Err.Issues[0].Path)
	})

	t.Run("an RPC validate failure maps to 400, not a retry-looping 500", func(t *testing.T) {
		// given: the RPC answers UNKNOWN_ERROR (core mapErrorCode has no
		// chat mappings) with the middleware's "validate: …" description —
		// the description, not the dead code, must drive the status
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.Anything).Return(&pb.RpcChatAddMessageResponse{
			Error: &pb.RpcChatAddMessageResponseError{
				Code:        pb.RpcChatAddMessageResponseError_UNKNOWN_ERROR,
				Description: "validate: mark range out of bounds",
			},
		})

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			v2model.AddChatMessageRequest{Text: "hello"}, false)

		// then
		requireV2Code(t, err, v2model.CodeValidationFailed)
		assert.Contains(t, err.Error(), "rejected")
	})

	t.Run("editing another member's message is a 403 forbidden, not a 500", func(t *testing.T) {
		// given: the description the EDIT path really produces —
		// chathandler.go joins storestate.ErrValidation with
		// ErrModifyForeignMessage and storeObject.EditMessage wraps the push
		// as "push change: …". Built from the same producers so a rewording
		// cannot leave this test green against a dead string (the original
		// fed the DELETE wording into the EDIT path and passed against
		// behavior that did not exist — surface review M2b).
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatProtoMessage()}})
		fx.mwMock.EXPECT().ChatEditMessageContent(mock.Anything, mock.Anything).Return(&pb.RpcChatEditMessageContentResponse{
			Error: &pb.RpcChatEditMessageContentResponseError{
				Code:        pb.RpcChatEditMessageContentResponseError_UNKNOWN_ERROR,
				Description: "push change: " + errors.Join(storestate.ErrValidation, chatobject.ErrModifyForeignMessage).Error(),
			},
		})

		// when
		_, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.EditChatMessageRequest{Text: "updated"}, false)

		// then
		requireV2Code(t, err, v2model.CodeForbidden)
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 403, v2Err.Status)
	})

	t.Run("deleting another member's message is a 403 forbidden, not a 500", func(t *testing.T) {
		// given: the DELETE path's refusal (chathandler BeforeDelete),
		// wrapped like the store wraps it
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatProtoMessage()}})
		fx.mwMock.EXPECT().ChatDeleteMessage(mock.Anything, mock.Anything).Return(&pb.RpcChatDeleteMessageResponse{
			Error: &pb.RpcChatDeleteMessageResponseError{
				Code:        pb.RpcChatDeleteMessageResponseError_UNKNOWN_ERROR,
				Description: "push change: " + chatobject.ErrDeleteForeignMessage.Error(),
			},
		})

		// when
		_, err := fx.DeleteChatMessage(context.Background(), testSpaceId, testChatId, "msg1", false)

		// then
		requireV2Code(t, err, v2model.CodeForbidden)
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 403, v2Err.Status)
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
			v2model.EditChatMessageRequest{Text: "updated text"}, false)

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
			v2model.EditChatMessageRequest{Text: "updated"}, false)

		// then
		requireV2Code(t, err, v2model.CodeNotFound)
	})

	t.Run("dry run stops after the existence check", func(t *testing.T) {
		// given: no ChatEditMessageContent expectation
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatProtoMessage()}})

		// when
		got, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.EditChatMessageRequest{Text: "updated"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
	})
}

func TestV2DeleteChatMessage(t *testing.T) {
	t.Run("delete passes through — and warns about the attachment file GC", func(t *testing.T) {
		// given: deleting a message permanently deletes (skipBin) any
		// attachment orphaned by it, asynchronously — the receipt must name
		// the ids at risk instead of hiding the irreversible part
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		msg := chatProtoMessage()
		msg.Attachments = []*model.ChatMessageAttachment{{Target: "file1", Type: model.ChatMessageAttachment_IMAGE}}
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{msg}})
		fx.mwMock.EXPECT().ChatDeleteMessage(mock.Anything, &pb.RpcChatDeleteMessageRequest{
			ChatObjectId: testChatId, MessageId: "msg1",
		}).Return(&pb.RpcChatDeleteMessageResponse{})

		// when
		got, err := fx.DeleteChatMessage(context.Background(), testSpaceId, testChatId, "msg1", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "msg1", got.Id)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "file1")
		assert.Contains(t, got.Warnings[0].Message, "PERMANENTLY")
	})

	t.Run("the real delete 404s for a missing message exactly like its dry run (C9)", func(t *testing.T) {
		// given: no ChatDeleteMessage expectation — the store handler
		// treats deleting a missing document as success, so skipping the
		// check would answer 200 for a deletion that never happened (and
		// still push a junk delete change into the CRDT tree)
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{}).Times(2)

		// when
		_, errReal := fx.DeleteChatMessage(context.Background(), testSpaceId, testChatId, "nope", false)
		_, errDry := fx.DeleteChatMessage(context.Background(), testSpaceId, testChatId, "nope", true)

		// then
		requireV2Code(t, errReal, v2model.CodeNotFound)
		requireV2Code(t, errDry, v2model.CodeNotFound)
	})

	t.Run("dry run reports the same file-GC warnings and deletes nothing", func(t *testing.T) {
		// given: no ChatDeleteMessage expectation
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		msg := chatProtoMessage()
		msg.Attachments = []*model.ChatMessageAttachment{{Target: "file1", Type: model.ChatMessageAttachment_FILE}}
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{msg}})

		// when
		got, err := fx.DeleteChatMessage(context.Background(), testSpaceId, testChatId, "msg1", true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "file1")
	})
}

func TestV2ToggleChatReaction(t *testing.T) {
	t.Run("toggle passes the outcome through", func(t *testing.T) {
		// given: the real path reads the message first — the RPC surfaces a
		// missing message as an opaque UNKNOWN_ERROR, the check makes it a 404
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatProtoMessage()}})
		fx.mwMock.EXPECT().ChatToggleMessageReaction(mock.Anything, &pb.RpcChatToggleMessageReactionRequest{
			ChatObjectId: testChatId, MessageId: "msg1", Emoji: "👍",
		}).Return(&pb.RpcChatToggleMessageReactionResponse{Added: true})

		// when
		got, err := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.ChatReactionRequest{Emoji: "👍"}, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, got.Added)
		assert.True(t, *got.Added)
	})

	t.Run("a reaction on a missing message is a 404, not a 500", func(t *testing.T) {
		// given: no toggle expectation — the RPC must never run
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{})

		// when
		_, err := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "nope",
			v2model.ChatReactionRequest{Emoji: "👍"}, false)

		// then
		requireV2Code(t, err, v2model.CodeNotFound)
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
			v2model.ChatReactionRequest{Emoji: "👍"}, true)
		removeOutcome, err2 := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.ChatReactionRequest{Emoji: "🎉"}, true)

		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		require.NotNil(t, addOutcome.Added)
		assert.True(t, *addOutcome.Added, "not reacted yet — the toggle would add")
		assert.True(t, addOutcome.DryRun)
		require.NotNil(t, removeOutcome.Added)
		assert.False(t, *removeOutcome.Added, "already reacted — the toggle would remove")
	})

	t.Run("dry run without an account identity omits added and warns", func(t *testing.T) {
		// given: Service documents accountId as possibly empty — with no
		// identity NOTHING matches the stored reactions, so asserting
		// added=true would be wrong whenever the caller already reacted
		mwMock := mock_apicore.NewMockClientCommands(t)
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("spaceView1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(testSpaceId),
		}})
		store.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String(testChatId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_chatDerived)),
		}})
		svc := NewService(mwMock, nil, nil, nil, nil, store, objectstore.TestTechSpaceId, "" /* no accountId */)
		msg := chatProtoMessage()
		msg.Reactions = &model.ChatMessageReactions{Reactions: map[string]*model.ChatMessageReactionsIdentityList{
			"👍": {Ids: []string{"someoneElse"}},
		}}
		mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{msg}})

		// when
		got, err := svc.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.ChatReactionRequest{Emoji: "👍"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
		assert.Nil(t, got.Added, "no identity to predict with — added must be omitted, not asserted")
		require.NotEmpty(t, got.Warnings)
		assert.Contains(t, got.Warnings[0].Message, "could not be predicted")
	})

	t.Run("empty emoji is a 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		_, err := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.ChatReactionRequest{}, false)
		requireV2Code(t, err, v2model.CodeValidationFailed)
	})
}

func TestV2ReadChat(t *testing.T) {
	t.Run("messages scope forwards up_to AND last_state_id — the race guard v1 made unreachable", func(t *testing.T) {
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
			v2model.ChatReadRequest{UpTo: "00a5", LastStateId: "state42"}, false)

		// then
		require.NoError(t, err)
		assert.False(t, got.DryRun)
	})

	t.Run("mentions scope maps to the mentions counter", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatReadMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatReadMessagesRequest) bool {
			return req.Type == pb.RpcChatReadMessages_Mentions && req.BeforeOrderId == "00a5" && req.LastStateId == "state42"
		})).Return(&pb.RpcChatReadMessagesResponse{})

		// when
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			v2model.ChatReadRequest{UpTo: "00a5", LastStateId: "state42", Scope: "mentions"}, false)

		// then
		require.NoError(t, err)
	})

	t.Run("up_to AND last_state_id are required — an empty bound OR guard silently marks nothing", func(t *testing.T) {
		// given: no RPC expectation — the request must never reach the RPC.
		// The range query ANDs `orderId <= up_to` with `stateId <= last_state_id`
		// and every stored message carries a non-empty state id, so EITHER
		// empty value is the same silent no-op (markedCount 0, HTTP 200)
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when: both missing
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId, v2model.ChatReadRequest{}, false)

		// then: both named, path-addressed
		requireV2Code(t, err, v2model.CodeValidationFailed)
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		require.Len(t, v2Err.Issues, 2)
		assert.Equal(t, "/up_to", v2Err.Issues[0].Path)
		assert.Equal(t, "/last_state_id", v2Err.Issues[1].Path)
	})

	t.Run("up_to alone is NOT enough — the omitted race guard is the v1 trap one field over", func(t *testing.T) {
		// given: no RPC expectation. Forwarding LastStateId:"" would make
		// MarkReadMessages mark ZERO messages and still answer 200 — the
		// exact silent no-op requiring up_to was meant to close
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			v2model.ChatReadRequest{UpTo: "00a5"}, false)

		// then
		requireV2Code(t, err, v2model.CodeValidationFailed)
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		require.Len(t, v2Err.Issues, 1)
		assert.Equal(t, "/last_state_id", v2Err.Issues[0].Path)
		assert.Contains(t, v2Err.Issues[0].Hint, "state.last_state_id")
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
			v2model.ChatReadRequest{Scope: "reactions"}, false)

		// then
		require.NoError(t, err)
	})

	t.Run("reactions scope rejects up_to — the backend takes no bound", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			v2model.ChatReadRequest{Scope: "reactions", UpTo: "00a5"}, false)
		requireV2Code(t, err, v2model.CodeValidationFailed)
	})

	t.Run("unknown scope is a 400 naming the allowed values", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			v2model.ChatReadRequest{Scope: "everything", UpTo: "00a5"}, false)
		requireV2Code(t, err, v2model.CodeValidationFailed)
		assert.Contains(t, err.Error(), "scope")
	})

	t.Run("dry run validates and forwards nothing", func(t *testing.T) {
		// given: no RPC expectations
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)

		// when
		got, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			v2model.ChatReadRequest{UpTo: "00a5", LastStateId: "state42"}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
	})

	t.Run("the forwarded RPC request never carries an empty last_state_id", func(t *testing.T) {
		// given: the regression pin for the silent no-op — whatever shape
		// reaches the RPC must carry a non-empty guard
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatReadMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatReadMessagesRequest) bool {
			return req.LastStateId != "" && req.BeforeOrderId != ""
		})).Return(&pb.RpcChatReadMessagesResponse{})

		// when
		_, err := fx.ReadChat(context.Background(), testSpaceId, testChatId,
			v2model.ChatReadRequest{UpTo: "00a5", LastStateId: "state42"}, false)

		// then
		require.NoError(t, err)
	})
}
