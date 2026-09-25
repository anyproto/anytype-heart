package v2handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

func TestChatStatusHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, body, payload string
		status              int
	}{
		{"no body", "", `{}`, 200},
		{"empty object", `{}`, `{}`, 200},
		{"empty text", `{"text":""}`, `{}`, 200},
		{"data only", `{"text":"","data":{"tool_call":"search","id":9007199254740993}}`, `{"data":{"tool_call":"search","id":9007199254740993}}`, 200},
		{"custom text", `{"text":"Searching","data":[true,2,null]}`, `{"text":"Searching","data":[true,2,null]}`, 200},
		{"unknown field", `{"status_text":"x"}`, "", 400},
		{"invalid text type", `{"text":{"x":1}}`, "", 400},
		{"malformed", `{"data":`, "", 400},
		{"array envelope", `[]`, "", 400},
		{"null envelope", `null`, "", 400},
		{"extra object", `{} {}`, "", 400},
		{"trailing garbage", `{} garbage`, "", 400},
		{"oversized encoded payload", `{"text":"` + strings.Repeat("x", 65508) + `"}`, "", 413},
		{"oversized HTTP body", strings.Repeat(" ", maxChatRequestBody+1), "", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := chatRouterFixture(t)
			fx.router.POST("/v2/spaces/:space_id/chats/:chat_id/status", PublishChatStatusHandler(fx.svc))
			if tc.status == http.StatusOK {
				fx.mwMock.EXPECT().PubsubPublish(mock.Anything, mock.Anything).Run(
					func(_ context.Context, req *pb.RpcPubsubPublishRequest) {
						require.Equal(t, "space1", req.SpaceId)
						require.Equal(t, "chat1/status", req.Topic)
						require.Equal(t, tc.payload, string(req.Payload))
					}).Return(&pb.RpcPubsubPublishResponse{}).Once()
			}
			w := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/status", tc.body)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == http.StatusOK {
				require.JSONEq(t, `{}`, w.Body.String())
			}
		})
	}
}

func TestChatStatusHTTPDryRun(t *testing.T) {
	fx := chatRouterFixture(t)
	fx.router.POST("/v2/spaces/:space_id/chats/:chat_id/status", PublishChatStatusHandler(fx.svc))
	// No middleware publication expectation: even the default status is not sent.
	w := serveChat(fx, "POST", "/v2/spaces/space1/chats/chat1/status?dry_run=true", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"dry_run":true}`, w.Body.String())
}
