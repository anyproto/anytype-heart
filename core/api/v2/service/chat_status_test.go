package v2service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/pubsub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestPublishChatStatusPayload(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  v2model.ChatStatusRequest
		want string
	}{
		{"localized default", v2model.ChatStatusRequest{}, `{}`},
		{"empty text with data", v2model.ChatStatusRequest{Data: json.RawMessage(`{"tool_call":"search"}`)}, `{"data":{"tool_call":"search"}}`},
		{"custom text", v2model.ChatStatusRequest{Text: "Searching documentation", Data: json.RawMessage(`{"tool_call":"search","nested":[true,null]}`)}, `{"text":"Searching documentation","data":{"tool_call":"search","nested":[true,null]}}`},
		{"array", v2model.ChatStatusRequest{Data: json.RawMessage(`[1,"a",false]`)}, `{"data":[1,"a",false]}`},
		{"large integer", v2model.ChatStatusRequest{Data: json.RawMessage(`9007199254740993`)}, `{"data":9007199254740993}`},
		{"string", v2model.ChatStatusRequest{Data: json.RawMessage(`"running"`)}, `{"data":"running"}`},
		{"boolean", v2model.ChatStatusRequest{Data: json.RawMessage(`false`)}, `{"data":false}`},
		{"null", v2model.ChatStatusRequest{Data: json.RawMessage(`null`)}, `{"data":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			fx.addChat(t, testChatId, "Chat", 0)
			fx.mwMock.EXPECT().PubsubPublish(mock.Anything, mock.Anything).Run(
				func(_ context.Context, req *pb.RpcPubsubPublishRequest) {
					require.Equal(t, testSpaceId, req.SpaceId)
					require.Equal(t, testChatId+"/status", req.Topic)
					require.Equal(t, tc.want, string(req.Payload))
				}).Return(&pb.RpcPubsubPublishResponse{}).Once()
			result, err := fx.PublishChatStatus(context.Background(), testSpaceId, testChatId, tc.req, false)
			require.NoError(t, err)
			require.False(t, result.DryRun)
		})
	}
}

func TestPublishChatStatusValidationAndDryRun(t *testing.T) {
	fx := newV2Fixture(t)
	fx.addChat(t, testChatId, "Chat", 0)
	// No publication expectation: validation and dry runs must not broadcast.
	result, err := fx.PublishChatStatus(context.Background(), testSpaceId, testChatId, v2model.ChatStatusRequest{}, true)
	require.NoError(t, err)
	require.True(t, result.DryRun)

	_, err = fx.PublishChatStatus(context.Background(), testSpaceId, testChatId, v2model.ChatStatusRequest{Data: json.RawMessage(`{`)}, false)
	requireV2Code(t, err, v2model.CodeValidationFailed)
	for _, dryRun := range []bool{false, true} {
		_, err = fx.PublishChatStatus(context.Background(), testSpaceId, testChatId, v2model.ChatStatusRequest{Text: strings.Repeat("x", pubsub.MaxPayloadSize)}, dryRun)
		requireV2Code(t, err, v2model.CodeRequestTooLarge)
	}
	// The limit includes JSON framing and escaping, not just the text length.
	text := strings.Repeat("x", pubsub.MaxPayloadSize-len(`{"text":""}`))
	fx.mwMock.EXPECT().PubsubPublish(mock.Anything, mock.Anything).Run(
		func(_ context.Context, req *pb.RpcPubsubPublishRequest) {
			require.Len(t, req.Payload, pubsub.MaxPayloadSize)
		},
	).Return(&pb.RpcPubsubPublishResponse{}).Once()
	_, err = fx.PublishChatStatus(context.Background(), testSpaceId, testChatId, v2model.ChatStatusRequest{Text: text}, false)
	require.NoError(t, err)
	_, err = fx.PublishChatStatus(context.Background(), testSpaceId, testChatId, v2model.ChatStatusRequest{Text: text + "x"}, false)
	requireV2Code(t, err, v2model.CodeRequestTooLarge)
}

func TestPublishChatStatusScope(t *testing.T) {
	fx := newV2Fixture(t)
	fx.addChat(t, testChatId, "Chat", 0)
	for _, tc := range []struct {
		spaceId, chatId string
		grant           *util.ApiGrant
		code            string
	}{
		{"unknown-space", testChatId, nil, v2model.CodeNotFound},
		{testSpaceId, "unknown-chat", nil, v2model.CodeNotFound},
		{testSpaceId, testChatId, &util.ApiGrant{Spaces: []string{testSpaceId}, Perms: util.GrantPermsRead}, v2model.CodeWriteNotGranted},
		{testSpaceId, testChatId, &util.ApiGrant{Spaces: []string{"other-space"}, Perms: util.GrantPermsReadWrite}, v2model.CodeSpaceNotGranted},
	} {
		ctx := util.CtxWithApiGrant(context.Background(), tc.grant)
		_, err := fx.PublishChatStatus(ctx, tc.spaceId, tc.chatId, v2model.ChatStatusRequest{}, false)
		requireV2Code(t, err, tc.code)
	}

	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("discussion1"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_discussion)),
	}, {
		bundle.RelationKeyId:             domain.String("page1"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
	}})
	_, err := fx.PublishChatStatus(context.Background(), testSpaceId, "page1", v2model.ChatStatusRequest{}, false)
	requireV2Code(t, err, v2model.CodeValidationFailed)
	fx.mwMock.EXPECT().PubsubPublish(mock.Anything, mock.MatchedBy(func(req *pb.RpcPubsubPublishRequest) bool {
		return req.Topic == "discussion1/status"
	})).Return(&pb.RpcPubsubPublishResponse{}).Once()
	_, err = fx.PublishChatStatus(context.Background(), testSpaceId, "discussion1", v2model.ChatStatusRequest{}, false)
	require.NoError(t, err)
}

func TestPublishChatStatusRPCFailures(t *testing.T) {
	for _, tc := range []struct {
		code pb.RpcPubsubPublishResponseErrorCode
		want string
	}{
		{pb.RpcPubsubPublishResponseError_BAD_INPUT, v2model.CodeValidationFailed},
		{pb.RpcPubsubPublishResponseError_UNKNOWN_ERROR, v2model.CodeInternalError},
	} {
		t.Run(tc.want, func(t *testing.T) {
			fx := newV2Fixture(t)
			fx.addChat(t, testChatId, "Chat", 0)
			fx.mwMock.EXPECT().PubsubPublish(mock.Anything, mock.Anything).Return(&pb.RpcPubsubPublishResponse{
				Error: &pb.RpcPubsubPublishResponseError{Code: tc.code, Description: "publication rejected"},
			}).Once()
			_, err := fx.PublishChatStatus(context.Background(), testSpaceId, testChatId, v2model.ChatStatusRequest{}, false)
			requireV2Code(t, err, tc.want)
		})
	}
}
