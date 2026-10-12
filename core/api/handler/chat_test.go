package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/pb"
)

func TestToggleChatReactionHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("a reaction to a deleted comment is a 400, not a 500", func(t *testing.T) {
		// given
		mwMock := mock_apicore.NewMockClientCommands(t)
		crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
		svc := service.NewService(mwMock, nil, "http://127.0.0.1:31009", "techspace", crossSpaceSubService)
		mwMock.On("ChatToggleMessageReaction", mock.Anything, mock.Anything).Return(&pb.RpcChatToggleMessageReactionResponse{
			Error: &pb.RpcChatToggleMessageReactionResponseError{
				Code:        pb.RpcChatToggleMessageReactionResponseError_BAD_INPUT,
				Description: "check message: message is deleted",
			},
		}).Once()
		req := httptest.NewRequest(http.MethodPost, "/v1/spaces/space1/chats/chat1/messages/msg1/reactions",
			bytes.NewBufferString(`{"emoji":"👍"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router := gin.New()
		router.POST("/v1/spaces/:space_id/chats/:chat_id/messages/:message_id/reactions", ToggleChatReactionHandler(svc))

		// when
		router.ServeHTTP(w, req)

		// then
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "message is deleted")
	})
}
