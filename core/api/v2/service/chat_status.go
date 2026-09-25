package v2service

import (
	"context"
	"encoding/json"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/pubsub"
	"github.com/anyproto/anytype-heart/pb"
)

// PublishChatStatus sends an ephemeral activity update for a chat or discussion.
// Identity and Space encryption are supplied by pubsub, not by the request body.
func (s *Service) PublishChatStatus(ctx context.Context, spaceId, chatId string, req v2model.ChatStatusRequest, dryRun bool) (*v2model.ChatStatusResult, error) {
	if _, err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, v2model.ValidationFailed("invalid status data",
			v2model.Issue{Path: "/data", Message: "data must be a valid JSON value"})
	}
	if len(payload) > pubsub.MaxPayloadSize {
		return nil, v2model.RequestTooLarge(fmt.Sprintf("encoded chat status exceeds the %d-byte pubsub payload limit", pubsub.MaxPayloadSize))
	}
	if dryRun {
		return &v2model.ChatStatusResult{DryRun: true}, nil
	}
	resp := s.mw.PubsubPublish(ctx, &pb.RpcPubsubPublishRequest{
		SpaceId: spaceId,
		Topic:   chatId + "/status",
		Payload: payload,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcPubsubPublishResponseError_NULL {
		return nil, v2ChatRpcError("publish chat status", int32(resp.Error.Code), int32(pb.RpcPubsubPublishResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &v2model.ChatStatusResult{}, nil
}
