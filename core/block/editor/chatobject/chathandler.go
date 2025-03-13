package chatobject

import (
	"context"
	"errors"
	"fmt"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/timeid"
)

type ChatHandler struct {
	subscription    *subscription
	currentIdentity string
	// forceNotRead forces handler to mark all messages as not read. It's useful for unit testing
	forceNotRead bool
}

func (d *ChatHandler) CollectionName() string {
	return collectionName
}

func (d *ChatHandler) Init(ctx context.Context, s *storestate.StoreState) (err error) {
	coll, err := s.Collection(ctx, collectionName)
	if err != nil {
		return err
	}
	iErr := coll.EnsureIndex(ctx, anystore.IndexInfo{
		Fields: []string{"_o.id"},
	})
	if iErr != nil && !errors.Is(iErr, anystore.ErrIndexExists) {
		return iErr
	}
	return
}

func (d *ChatHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) (err error) {
	msg := newMessageWrapper(ch.Arena, ch.Value)
	msg.setCreatedAt(ch.Change.Timestamp)
	msg.setCreator(ch.Change.Creator)
	if d.forceNotRead {
		msg.setRead(false)
	} else {
		if ch.Change.Creator == d.currentIdentity {
			msg.setRead(true)
		} else {
			msg.setRead(false)
		}
	}

	msg.setAddedAt(timeid.NewNano())
	model := msg.toModel()
	model.OrderId = ch.Change.Order
	d.subscription.add(ch.Change.PrevOrderId, model)

	return
}

func (d *ChatHandler) BeforeModify(ctx context.Context, ch storestate.ChangeOp) (mode storestate.ModifyMode, err error) {
	return storestate.ModifyModeUpsert, nil
}

func (d *ChatHandler) BeforeDelete(ctx context.Context, ch storestate.ChangeOp) (mode storestate.DeleteMode, err error) {
	coll, err := ch.State.Collection(ctx, collectionName)
	if err != nil {
		return storestate.DeleteModeDelete, fmt.Errorf("get collection: %w", err)
	}

	messageId := ch.Change.Change.GetDelete().GetDocumentId()

	doc, err := coll.FindId(ctx, messageId)
	if err != nil {
		return storestate.DeleteModeDelete, fmt.Errorf("get message: %w", err)
	}

	message := newMessageWrapper(ch.Arena, doc.Value())
	if message.getCreator() != ch.Change.Creator {
		return storestate.DeleteModeDelete, errors.New("can't delete not own message")
	}

	d.subscription.delete(messageId)
	return storestate.DeleteModeDelete, nil
}

func (d *ChatHandler) UpgradeKeyModifier(ch storestate.ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		if len(key.KeyPath) == 0 {
			return nil, false, fmt.Errorf("no key path")
		}

		path := key.KeyPath[0]

		result, modified, err = mod.Modify(a, v)
		if err != nil {
			return nil, false, err
		}

		if modified {
			msg := newMessageWrapper(a, result)
			model := msg.toModel()

			switch path {
			case reactionsKey:
				// Do not parse json, just trim "
				identity := strings.Trim(key.ModifyValue, `"`)
				if identity != ch.Change.Creator {
					return v, false, errors.Join(storestate.ErrValidation, fmt.Errorf("can't toggle someone else's reactions"))
				}
				// TODO Count validation

				d.subscription.updateReactions(model)
			case contentKey:
				creator := model.Creator
				if creator != ch.Change.Creator {
					return v, false, errors.Join(storestate.ErrValidation, fmt.Errorf("can't modify someone else's message"))
				}
				result.Set(modifiedAtKey, a.NewNumberInt(int(ch.Change.Timestamp)))
				model.ModifiedAt = ch.Change.Timestamp
				d.subscription.updateFull(model)
			default:
				return nil, false, fmt.Errorf("invalid key path %s", key.KeyPath)
			}
		}

		return result, modified, nil
	})
}
