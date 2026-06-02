package api

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type chatSubAdapter struct {
	svc chatsubscription.Service
}

func (a *chatSubAdapter) SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string, sink chan<- *pb.Event) ([]*model.ChatMessage, error) {
	resp, err := a.svc.SubscribeLastMessages(ctx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId:     chatObjectId,
		SubId:            subId,
		Limit:            limit,
		WithDependencies: false,
		SseSink:          sink,
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe last messages: %w", err)
	}

	msgs := make([]*model.ChatMessage, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		msgs = append(msgs, m.ChatMessage)
	}

	return msgs, nil
}

func (a *chatSubAdapter) Unsubscribe(chatObjectId string, subId string) error {
	return a.svc.Unsubscribe(chatObjectId, subId)
}
