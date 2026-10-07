package api

import (
	"context"
	"errors"
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type chatSubAdapter struct {
	svc chatsubscription.Service
	// spaceChats backs OpenSpaceChats; nil refuses it.
	spaceChats *spaceChatHubs
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

func (a *chatSubAdapter) ChatState(spaceId, chatObjectId string) (*model.ChatState, error) {
	mngr, err := a.svc.GetManager(spaceId, chatObjectId)
	if err != nil {
		return nil, fmt.Errorf("get chat manager: %w", err)
	}
	mngr.Lock()
	defer mngr.Unlock()
	return mngr.GetChatState(), nil
}

func (a *chatSubAdapter) OpenSpaceChats(ctx context.Context, req apicore.SpaceChatOpen) (apicore.SpaceChatSubscription, error) {
	if a.spaceChats == nil {
		return nil, errors.New("open space chats: the space chat hub is not configured")
	}
	sub, err := a.spaceChats.Open(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("open space chats: %w", err)
	}
	return sub, nil
}
