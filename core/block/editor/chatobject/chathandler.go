package chatobject

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

type ChatHandler struct {
	MyIdentity  string
	eventSender event.Sender
	chatId      string
}

func (d ChatHandler) CollectionName() string {
	return collectionName
}

func (d ChatHandler) Init(ctx context.Context, s *storestate.StoreState) (err error) {
	_, err = s.Collection(ctx, collectionName)
	return
}

func (d ChatHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) (err error) {
	// TODO Validate that creator from change equals to creator from message!
	ev := &pb.EventChatAdd{
		Id:     string(ch.Value.GetStringBytes("id")),
		Author: string(ch.Value.GetStringBytes("author")),
		Text:   string(ch.Value.GetStringBytes("text")),
	}
	d.eventSender.Broadcast(&pb.Event{
		ContextId: d.chatId,
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfChatAdd{
					ChatAdd: ev,
				},
			},
		},
	})
	return
}

func (d ChatHandler) BeforeModify(ctx context.Context, ch storestate.ChangeOp) (mode storestate.ModifyMode, err error) {
	return storestate.ModifyModeUpsert, nil
}

func (d ChatHandler) BeforeDelete(ctx context.Context, ch storestate.ChangeOp) (mode storestate.DeleteMode, err error) {
	return storestate.DeleteModeDelete, nil
}

func (d ChatHandler) UpgradeKeyModifier(ch storestate.ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return query.ModifyFunc(func(a *fastjson.Arena, v *fastjson.Value) (result *fastjson.Value, modified bool, err error) {
		if len(key.KeyPath) == 0 {
			return nil, false, fmt.Errorf("no key path")
		}
		if key.KeyPath[0] == "message" {
			author := v.GetStringBytes("author")
			if string(author) != ch.Change.Creator {
				return v, false, errors.Join(storestate.ErrValidation, fmt.Errorf("can't modify not own message"))
			}
		}
		result, modified, err = mod.Modify(a, v)
		if err != nil {
			return nil, false, err
		}
		ev := &pb.EventChatUpdate{
			Id:     string(result.GetStringBytes("id")),
			Author: string(result.GetStringBytes("author")),
			Text:   string(result.GetStringBytes("text")),
		}
		d.eventSender.Broadcast(&pb.Event{
			ContextId: d.chatId,
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfChatUpdate{
						ChatUpdate: ev,
					},
				},
			},
		})

		return result, modified, nil
	})
}
