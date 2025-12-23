package chatobject

import (
	"context"
	"errors"
	"fmt"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ChatHandler struct {
	repository      chatrepository.Repository
	subscription    chatsubscription.Manager
	indexerStore    objectstore.IndexerStore
	chatFullId      domain.FullID
	currentIdentity string
	myParticipantId string
	// forceNotRead forces handler to mark all messages as not read. It's useful for unit testing
	forceNotRead bool
}

func (d *ChatHandler) CollectionName() string {
	return CollectionName
}

func (d *ChatHandler) Init(ctx context.Context, s *storestate.StoreState) (err error) {
	coll, err := s.Collection(ctx, CollectionName)
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

func (d *ChatHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) error {
	msg, err := chatmodel.UnmarshalMessage(ch.Value)
	if err != nil {
		return fmt.Errorf("unmarshal message: %w", err)
	}
	msg.CreatedAt = ch.Change.Timestamp
	msg.Creator = ch.Change.Creator
	if d.forceNotRead {
		msg.Read = false
		msg.MentionRead = false
	} else {
		if ch.Change.Creator == d.currentIdentity {
			msg.Read = true
			msg.MentionRead = true
		} else {
			msg.Read = false
			msg.MentionRead = false
		}
	}

	if ch.Change.Creator == d.currentIdentity {
		msg.Synced = false
	} else {
		msg.Synced = true
	}

	msg.StateId = bson.NewObjectId().Hex()

	isMentioned, err := msg.IsCurrentUserMentioned(ctx, d.myParticipantId, d.currentIdentity, d.repository)
	if err != nil {
		return fmt.Errorf("check if current user is mentioned: %w", err)
	}
	msg.HasMention = isMentioned
	msg.OrderId = ch.Change.Order

	prevOrderId, err := d.repository.GetPrevOrderId(ctx, ch.Change.Order)
	if err != nil {
		return fmt.Errorf("get prev order id: %w", err)
	}

	d.subscription.Lock()
	defer d.subscription.Unlock()
	d.subscription.UpdateChatState(func(state *model.ChatState) *model.ChatState {
		if !msg.Read {
			if msg.OrderId < state.Messages.OldestOrderId || state.Messages.OldestOrderId == "" {
				state.Messages.OldestOrderId = msg.OrderId
			}
			state.Messages.Counter++

			if isMentioned {
				state.Mentions.Counter++
				if msg.OrderId < state.Mentions.OldestOrderId || state.Mentions.OldestOrderId == "" {
					state.Mentions.OldestOrderId = msg.OrderId
				}
			}

		}
		if msg.StateId > state.LastStateId {
			state.LastStateId = msg.StateId
		}
		return state
	})

	d.subscription.Add(prevOrderId, msg)

	if err = d.indexerStore.AddChatMessageToIndexQueue(ctx, d.chatFullId, msg.OrderId); err != nil {
		return fmt.Errorf("add chat message to full text index queue: %w", err)
	}

	msg.MarshalAnyenc(ch.Value, ch.Arena)

	return nil
}

func (d *ChatHandler) BeforeModify(ctx context.Context, ch storestate.ChangeOp) (mode storestate.ModifyMode, err error) {
	if err = d.indexerStore.AddChatMessageToIndexQueue(ctx, d.chatFullId, ch.Change.Order); err != nil {
		return 0, fmt.Errorf("add chat message to full text index queue: %w", err)
	}
	return storestate.ModifyModeUpsert, nil
}

func (d *ChatHandler) BeforeDelete(ctx context.Context, ch storestate.ChangeOp) (mode storestate.DeleteMode, err error) {
	coll, err := ch.State.Collection(ctx, CollectionName)
	if err != nil {
		return storestate.DeleteModeDelete, fmt.Errorf("get collection: %w", err)
	}

	messageId := ch.Change.Change.GetDelete().GetDocumentId()

	doc, err := coll.FindId(ctx, messageId)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return storestate.DeleteModeDelete, nil
	}
	if err != nil {
		return storestate.DeleteModeDelete, fmt.Errorf("get message: %w", err)
	}

	message, err := chatmodel.UnmarshalMessage(doc.Value())
	if err != nil {
		return storestate.DeleteModeDelete, fmt.Errorf("unmarshal message: %w", err)
	}
	if message.Creator != ch.Change.Creator {
		return storestate.DeleteModeDelete, errors.New("can't delete not own message")
	}

	d.subscription.Lock()
	defer d.subscription.Unlock()
	d.subscription.Delete(messageId)

	// TODO: add msg on deletion to fulltext queue, so it would be deleted from tantivy

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
			msg, err := chatmodel.UnmarshalMessage(result)
			if err != nil {
				return nil, false, fmt.Errorf("unmarshal message: %w", err)
			}

			d.subscription.Lock()
			defer d.subscription.Unlock()

			switch path {
			case chatmodel.ReactionsKey:
				// Do not parse json, just trim "
				identity := strings.Trim(key.ModifyValue, `"`)
				if identity != ch.Change.Creator {
					return v, false, errors.Join(storestate.ErrValidation, fmt.Errorf("can't toggle someone else's reactions"))
				}
				// TODO Count validation

				d.subscription.UpdateReactions(msg)
			case chatmodel.ContentKey:
				creator := msg.Creator
				if creator != ch.Change.Creator {
					return v, false, errors.Join(storestate.ErrValidation, fmt.Errorf("can't modify someone else's message"))
				}
				msg.ModifiedAt = ch.Change.Timestamp
				msg.MarshalAnyenc(result, a)
				d.subscription.UpdateFull(msg)
			default:
				return nil, false, fmt.Errorf("invalid key path %s", key.KeyPath)
			}
		}

		return result, modified, nil
	})
}
