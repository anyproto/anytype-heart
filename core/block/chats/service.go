package chats

import (
	"context"
	"sort"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "core.block.chats"

type Service interface {
	AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *model.ChatMessage) (string, error)
	EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *model.ChatMessage) error
	ToggleMessageReaction(ctx context.Context, chatObjectId string, messageId string, emoji string) error
	DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error
	GetMessages(ctx context.Context, chatObjectId string, beforeOrderId string, limit int) ([]*model.ChatMessage, error)
	GetMessagesByIds(ctx context.Context, chatObjectId string, messageIds []string) ([]*model.ChatMessage, error)
	SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int) ([]*model.ChatMessage, int, error)
	Unsubscribe(chatObjectId string) error
	DebugChanges(ctx context.Context, chatObjectId string, orderBy pb.RpcDebugChatChangesRequestOrderBy) ([]*pb.RpcDebugChatChangesResponseChange, error)

	app.Component
}

var _ Service = (*service)(nil)

type service struct {
	objectGetter cache.ObjectGetter
}

func New() Service {
	return &service{}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)

	return nil
}

func (s *service) AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *model.ChatMessage) (string, error) {
	var messageId string
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		messageId, err = sb.AddMessage(ctx, sessionCtx, message)
		return err
	})
	return messageId, err
}

func (s *service) EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *model.ChatMessage) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.EditMessage(ctx, messageId, newMessage)
	})
}

func (s *service) ToggleMessageReaction(ctx context.Context, chatObjectId string, messageId string, emoji string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.ToggleMessageReaction(ctx, messageId, emoji)
	})
}

func (s *service) DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.DeleteMessage(ctx, messageId)
	})
}

func (s *service) GetMessages(ctx context.Context, chatObjectId string, beforeOrderId string, limit int) ([]*model.ChatMessage, error) {
	var res []*model.ChatMessage
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		msgs, err := sb.GetMessages(ctx, beforeOrderId, limit)
		if err != nil {
			return err
		}
		res = msgs
		return nil
	})
	return res, err
}

func (s *service) GetMessagesByIds(ctx context.Context, chatObjectId string, messageIds []string) ([]*model.ChatMessage, error) {
	var res []*model.ChatMessage
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		msg, err := sb.GetMessagesByIds(ctx, messageIds)
		if err != nil {
			return err
		}
		res = msg
		return nil
	})
	return res, err
}

func (s *service) SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int) ([]*model.ChatMessage, int, error) {
	var (
		msgs      []*model.ChatMessage
		numBefore int
	)
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		msgs, numBefore, err = sb.SubscribeLastMessages(ctx, limit)
		if err != nil {
			return err
		}
		return nil
	})
	return msgs, numBefore, err
}

func (s *service) Unsubscribe(chatObjectId string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.Unsubscribe()
	})
}

func (s *service) DebugChanges(ctx context.Context, chatObjectId string, orderBy pb.RpcDebugChatChangesRequestOrderBy) ([]*pb.RpcDebugChatChangesResponseChange, error) {
	var changesOut []*pb.RpcDebugChatChangesResponseChange
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		changes, err := sb.DebugChanges(ctx)
		if err != nil {
			return err
		}
		for _, ch := range changes {
			var errString string
			if ch.Error != nil {
				errString = ch.Error.Error()
			}
			changesOut = append(changesOut, &pb.RpcDebugChatChangesResponseChange{
				ChangeId: ch.ChangeId,
				OrderId:  ch.OrderId,
				Error:    errString,
				Change:   ch.Change,
			})
		}
		return nil
	})

	if orderBy == pb.RpcDebugChatChangesRequest_ORDER_ID {
		sort.Slice(changesOut, func(i, j int) bool { return changesOut[i].OrderId < changesOut[j].OrderId })
	}

	return changesOut, err
}
