package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// tombstoneProto is a deleted discussion comment as the middleware serves it.
func tombstoneProto() *model.ChatMessage {
	msg := &chatmodel.Message{ChatMessage: chatProtoMessage()}
	msg.Id = "msgDeleted"
	msg.StateId = "state7"
	msg.ReplyToMessageId = "msgPost"
	msg.Attachments = []*model.ChatMessageAttachment{{Target: "file1"}}
	msg.MakeTombstone()
	return msg.ChatMessage
}

func (fx *v2Fixture) withExistingMessage(msg *model.ChatMessage) {
	fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
		Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{msg}})
}

func requireV2Status(t *testing.T, err error, status int) *v2model.Error {
	t.Helper()
	var v2Err *v2model.Error
	require.ErrorAs(t, err, &v2Err)
	require.Equal(t, status, v2Err.Status, v2Err.Message)
	return v2Err
}

func TestV2DeletedCommentIsServedAsTombstone(t *testing.T) {
	for _, resolved := range []bool{true, false} {
		t.Run(fmt.Sprintf("author resolved: %v", resolved), func(t *testing.T) {
			// given
			fx := newV2Fixture(t)
			fx.addChat(t, testChatId, "Team chat", 1000)
			if resolved {
				fx.addParticipant(t, testIdentity, "Alice")
			}
			fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesResponse{
				Messages: []*model.ChatMessage{tombstoneProto()},
			})
			author := ""
			if resolved {
				author = `"author": "Alice",`
			}
			want := fmt.Sprintf(`{
				"id": "msgDeleted",
				"state_id": "state7",
				"order": "00a1",
				%s
				"author_id": %q,
				"at": "2024-06-03T09:00:00Z",
				"text": "",
				"reply_to": "msgPost",
				"deleted": true
			}`, author, domain.NewParticipantId(testSpaceId, testIdentity))

			// when
			got, err := fx.GetChatMessages(context.Background(), testSpaceId, testChatId, ChatMessagesQuery{Limit: 25})

			// then
			require.NoError(t, err)
			require.Len(t, got.Messages, 1)
			raw, err := json.Marshal(got.Messages[0])
			require.NoError(t, err)
			assert.JSONEq(t, want, string(raw))
		})
	}
}

func TestV2DeletedCommentRefusesChanges(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		suffix := fmt.Sprintf(" (dry run: %v)", dryRun)

		t.Run("edit is a 404"+suffix, func(t *testing.T) {
			// given: no edit RPC expectation
			fx := newV2Fixture(t)
			fx.addChat(t, testChatId, "Team chat", 1000)
			fx.withExistingMessage(tombstoneProto())

			// when
			_, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "msgDeleted",
				v2model.EditChatMessageRequest{Text: "back"}, dryRun)

			// then
			v2Err := requireV2Status(t, err, 404)
			assert.Contains(t, v2Err.Message, "deleted")
		})

		t.Run("reaction is a 404"+suffix, func(t *testing.T) {
			// given: no toggle RPC expectation
			fx := newV2Fixture(t)
			fx.addChat(t, testChatId, "Team chat", 1000)
			fx.withExistingMessage(tombstoneProto())

			// when
			_, err := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msgDeleted",
				v2model.ChatReactionRequest{Emoji: "👍"}, dryRun)

			// then
			v2Err := requireV2Status(t, err, 404)
			assert.Contains(t, v2Err.Message, "deleted")
		})

		t.Run("a second delete is a 404"+suffix, func(t *testing.T) {
			// given: no delete RPC expectation
			fx := newV2Fixture(t)
			fx.addChat(t, testChatId, "Team chat", 1000)
			fx.withExistingMessage(tombstoneProto())

			// when
			_, err := fx.DeleteChatMessage(context.Background(), testSpaceId, testChatId, "msgDeleted", dryRun)

			// then
			v2Err := requireV2Status(t, err, 404)
			assert.Contains(t, v2Err.Message, "deleted")
		})

		t.Run("a reply to it is a 400 on reply_to"+suffix, func(t *testing.T) {
			// given: no add RPC expectation
			fx := newV2Fixture(t)
			fx.addChat(t, testChatId, "Team chat", 1000)
			fx.withExistingMessage(tombstoneProto())

			// when
			_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
				v2model.AddChatMessageRequest{Text: "reply", ReplyTo: "msgDeleted"}, dryRun)

			// then
			v2Err := requireV2Status(t, err, 400)
			require.NotEmpty(t, v2Err.Issues)
			assert.Equal(t, "/reply_to", v2Err.Issues[0].Path)
		})
	}

	t.Run("a reply to a message that is not stored here is sent as before", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).Return(&pb.RpcChatGetMessagesByIdsResponse{})
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.Anything).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})

		// when
		got, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			v2model.AddChatMessageRequest{Text: "reply", ReplyTo: "msgElsewhere"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "msgNew", got.Id)
	})
}

// A delete can land between the API's check and the RPC: the middleware's refusal must map to
// the same status the check gives.
func TestV2DeleteRacingTheCheck(t *testing.T) {
	badInput := func(err error) string { return "check message: " + err.Error() }

	t.Run("edit refused as deleted is a 404", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.withExistingMessage(chatProtoMessage())
		fx.mwMock.EXPECT().ChatEditMessageContent(mock.Anything, mock.Anything).Return(&pb.RpcChatEditMessageContentResponse{
			Error: &pb.RpcChatEditMessageContentResponseError{
				Code:        pb.RpcChatEditMessageContentResponseError_BAD_INPUT,
				Description: badInput(chatobject.ErrMessageDeleted),
			},
		})

		// when
		_, err := fx.EditChatMessage(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.EditChatMessageRequest{Text: "updated"}, false)

		// then
		requireV2Status(t, err, 404)
	})

	t.Run("reaction refused as missing is a 404", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.withExistingMessage(chatProtoMessage())
		fx.mwMock.EXPECT().ChatToggleMessageReaction(mock.Anything, mock.Anything).Return(&pb.RpcChatToggleMessageReactionResponse{
			Error: &pb.RpcChatToggleMessageReactionResponseError{
				Code:        pb.RpcChatToggleMessageReactionResponseError_BAD_INPUT,
				Description: badInput(chatobject.ErrMessageNotFound),
			},
		})

		// when
		_, err := fx.ToggleChatReaction(context.Background(), testSpaceId, testChatId, "msg1",
			v2model.ChatReactionRequest{Emoji: "👍"}, false)

		// then
		requireV2Status(t, err, 404)
	})

	t.Run("reply refused as deleted is a 400 on reply_to", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		fx.withExistingMessage(chatProtoMessage())
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.Anything).Return(&pb.RpcChatAddMessageResponse{
			Error: &pb.RpcChatAddMessageResponseError{
				Code:        pb.RpcChatAddMessageResponseError_BAD_INPUT,
				Description: chatobject.ErrReplyToDeletedMessage.Error(),
			},
		})

		// when
		_, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			v2model.AddChatMessageRequest{Text: "reply", ReplyTo: "msg1"}, false)

		// then
		v2Err := requireV2Status(t, err, 400)
		require.NotEmpty(t, v2Err.Issues)
		assert.Equal(t, "/reply_to", v2Err.Issues[0].Path)
	})
}
