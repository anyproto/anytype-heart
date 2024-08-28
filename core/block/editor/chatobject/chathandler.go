package chatobject

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

type ChatHandler struct {
	chatId       string
	subscription *subscription
}

func (d ChatHandler) CollectionName() string {
	return collectionName
}

func (d ChatHandler) Init(ctx context.Context, s *storestate.StoreState) (err error) {
	_, err = s.Collection(ctx, collectionName)
	return
}

func (d ChatHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) (err error) {
	ch.Value.Set("createdAt", ch.Arena.NewNumberInt(int(ch.Change.Timestamp)))
	ch.Value.Set("creator", ch.Arena.NewString(ch.Change.Creator))

	message := unmarshalMessage(ch.Value)
	message.OrderId = ch.Change.Order
	d.subscription.add(message)

	return
}

func (d ChatHandler) BeforeModify(ctx context.Context, ch storestate.ChangeOp) (mode storestate.ModifyMode, err error) {
	return storestate.ModifyModeUpsert, nil
}

func (d ChatHandler) BeforeDelete(ctx context.Context, ch storestate.ChangeOp) (mode storestate.DeleteMode, err error) {
	d.subscription.delete(ch.Change.Change.GetDelete().GetDocumentId())
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

		if modified {
			message := unmarshalMessage(result)
			if key.KeyPath[0] == "reactions" {
				d.subscription.updateReactions(message)
			} else {
				d.subscription.updateFull(message)
			}
		}

		return result, modified, nil
	})
}
