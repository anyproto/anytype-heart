package v2handler

// chat_test.go covers the Phase-6 chat HTTP layer — the review
// found the whole layer untested: ?reactions= and dry_run could be broken
// with every suite green, and dry_run=true regressing silently would SEND A
// REAL MESSAGE into the user's chat. Each case here pins one behavior the
// swagger text and §8.7 assert.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"

	"github.com/anyproto/anytype-heart/core/domain"
)

// chatRouterFixture mounts every chat route with the C10 pagination
// middleware (the handlers read the parsed limit from the context) and
// registers one chat object in space1.
func chatRouterFixture(t *testing.T) *v2HandlerFixture {
	fx := newV2HandlerFixture(t)
	fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("chat1"),
		bundle.RelationKeyName:           domain.String("Team chat"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_chatDerived)),
	}})
	fx.router.Use(pagination.New(pagination.Config{DefaultPage: 0, DefaultPageSize: 25, MinPageSize: 1, MaxPageSize: 1000}))
	fx.router.Use(withDryRunFlag())
	fx.router.GET("/v2/spaces/:space_id/chats/:chat_id/messages", GetChatMessagesHandler(fx.svc))
	fx.router.POST("/v2/spaces/:space_id/chats", CreateChatHandler(fx.svc))
	fx.router.POST("/v2/spaces/:space_id/chats/:chat_id/messages", AddChatMessageHandler(fx.svc))
	fx.router.PATCH("/v2/spaces/:space_id/chats/:chat_id/messages/:message_id", EditChatMessageHandler(fx.svc))
	fx.router.DELETE("/v2/spaces/:space_id/chats/:chat_id/messages/:message_id", DeleteChatMessageHandler(fx.svc))
	fx.router.POST("/v2/spaces/:space_id/chats/:chat_id/messages/:message_id/reactions", ToggleChatReactionHandler(fx.svc))
	fx.router.POST("/v2/spaces/:space_id/chats/:chat_id/read", ReadChatHandler(fx.svc))
	return fx
}

func chatHandlerTestMessage() *model.ChatMessage {
	return &model.ChatMessage{
		Id:      "msg1",
		OrderId: "00a1",
		Creator: "identityA",
		Message: &model.ChatMessageMessageContent{Text: "hello"},
		Reactions: &model.ChatMessageReactions{
			Reactions: map[string]*model.ChatMessageReactionsIdentityList{
				"👍": {Ids: []string{"identityA", "identityB"}},
			},
		},
	}
}

func serveChat(fx *v2HandlerFixture, method, target, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}

func TestGetChatMessagesV2HandlerQueryPlumbing(t *testing.T) {
	t.Run("?reactions=full reaches the service — reacted_by appears, counts keep their slot", func(t *testing.T) {
		// given
		fx := chatRouterFixture(t)
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesResponse{Messages: []*model.ChatMessage{chatHandlerTestMessage()}}).Times(2)

		// when
		wFull := serveChat(fx, "GET", "/v2/spaces/space1/chats/chat1/messages?reactions=full", "")
		wDefault := serveChat(fx, "GET", "/v2/spaces/space1/chats/chat1/messages", "")

		// then
		require.Equal(t, http.StatusOK, wFull.Code)
		assert.Contains(t, wFull.Body.String(), `"reacted_by"`,
			"?reactions=full must plumb through to the DTO — the whole Q4 feature dies silently otherwise")
		require.Equal(t, http.StatusOK, wDefault.Code)
		assert.NotContains(t, wDefault.Body.String(), `"reacted_by"`)
		assert.Contains(t, wDefault.Body.String(), `"reactions":{"👍":2}`)
	})

	t.Run("?reactions=garbage is a 400 naming the allowed values", func(t *testing.T) {
		// given: no RPC expectation
		fx := chatRouterFixture(t)

		// when
		w := serveChat(fx, "GET", "/v2/spaces/space1/chats/chat1/messages?reactions=wat", "")

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "counts, full")
	})

	t.Run("?after and ?limit reach the RPC — with the has_more +1", func(t *testing.T) {
		// given
		fx := chatRouterFixture(t)
		fx.mwMock.EXPECT().ChatGetMessages(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatGetMessagesRequest) bool {
			return req.AfterOrderId == "0090" && req.Limit == 3
		})).Return(&pb.RpcChatGetMessagesResponse{})

		// when
		w := serveChat(fx, "GET", "/v2/spaces/space1/chats/chat1/messages?after=0090&limit=2", "")

		// then
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("?offset is rejected with cursor steering", func(t *testing.T) {
		fx := chatRouterFixture(t)
		w := serveChat(fx, "GET", "/v2/spaces/space1/chats/chat1/messages?offset=5", "")
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "cursor")
	})
}

func TestChatMutationDryRunPlumbing(t *testing.T) {
	// Every subtest registers NO mutation RPC expectation: were dry_run
	// dropped from the handler → service call, the strict mock would fail
	// the test AND the status would flip — the exact break the review
	// performed with every suite staying green.

	t.Run("POST message with dry_run sends NOTHING and answers 200, without dry_run commits with 201", func(t *testing.T) {
		// given
		fx := chatRouterFixture(t)

		// when: dry run — no ChatAddMessage expectation, a send would fail
		wDry := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/messages?dry_run=true", `{"text":"hi"}`)

		// then
		require.Equal(t, http.StatusOK, wDry.Code, "a dry run is not a create — 200, not 201")
		assert.Contains(t, wDry.Body.String(), `"dry_run":true`)

		// and when: the real send
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.Anything).
			Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"})
		wReal := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/messages", `{"text":"hi"}`)

		// then
		require.Equal(t, http.StatusCreated, wReal.Code)
		assert.Contains(t, wReal.Body.String(), `"id":"msgNew"`)
	})

	t.Run("POST chat with dry_run creates nothing and answers 200", func(t *testing.T) {
		fx := chatRouterFixture(t)
		w := serveChat(fx, "POST", "/v2/spaces/space1/chats?dry_run=true", `{"name":"New chat"}`)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"dry_run":true`)
	})

	t.Run("PATCH message with dry_run stops after the existence check", func(t *testing.T) {
		fx := chatRouterFixture(t)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatHandlerTestMessage()}})
		w := serveChat(fx, "PATCH", "/v2/spaces/space1/chats/chat1/messages/msg1?dry_run=true", `{"text":"updated"}`)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"dry_run":true`)
	})

	t.Run("DELETE message with dry_run deletes nothing", func(t *testing.T) {
		fx := chatRouterFixture(t)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatHandlerTestMessage()}})
		w := serveChat(fx, "DELETE", "/v2/spaces/space1/chats/chat1/messages/msg1?dry_run=true", "")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"dry_run":true`)
	})

	t.Run("POST reaction with dry_run toggles nothing and predicts the outcome", func(t *testing.T) {
		fx := chatRouterFixture(t)
		fx.mwMock.EXPECT().ChatGetMessagesByIds(mock.Anything, mock.Anything).
			Return(&pb.RpcChatGetMessagesByIdsResponse{Messages: []*model.ChatMessage{chatHandlerTestMessage()}})
		w := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/messages/msg1/reactions?dry_run=true", `{"emoji":"🎉"}`)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"added":true`,
			"the fixture account carries no 🎉 — the predicted outcome is added")
		assert.Contains(t, w.Body.String(), `"dry_run":true`)
	})

	t.Run("POST read with dry_run forwards nothing", func(t *testing.T) {
		fx := chatRouterFixture(t)
		w := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/read?dry_run=true", `{"up_to":"00a1","last_state_id":"state42"}`)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"dry_run":true`)
	})
}

func TestChatBodyDecoding(t *testing.T) {
	t.Run("an unknown field is a 400 naming the field", func(t *testing.T) {
		// given: strict decoding (C13's spirit at the request layer)
		fx := chatRouterFixture(t)

		// when
		w := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/messages", `{"message":"hi"}`)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeValidationFailed, got.Code)
		require.NotEmpty(t, got.Issues)
		assert.Equal(t, "/message", got.Issues[0].Path, "the unknown field must be named, path-addressed")
	})

	t.Run("an oversized body is a 413 request_too_large", func(t *testing.T) {
		fx := chatRouterFixture(t)
		w := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/messages", strings.Repeat("x", 1<<20+1))
		require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		assert.Contains(t, w.Body.String(), v2model.CodeRequestTooLarge)
	})

	t.Run("an empty body is a 400 carrying the shape hint", func(t *testing.T) {
		fx := chatRouterFixture(t)
		w := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/messages", "")
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "text, reply_to, attachments")
	})
}
