package chatobject

import (
	"context"
	"errors"
	"fmt"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/globalsign/mgo/bson"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The two foreign-message refusals, exported because the API layer
// classifies them by matching the RPC error DESCRIPTION (the sentinel is
// flattened to a string at the RPC boundary): core/api/v2 matches
// Contains(description, Err….Error()), so referencing the sentinel on both
// sides makes a rewording here update the classifier at compile time
// instead of silently degrading the 403 back to a retry-looping 500
// (API v2 surface review M2b).
var (
	// ErrModifyForeignMessage refuses editing another member's message
	// (the EDIT path, UpgradeKeyModifier on the content key).
	ErrModifyForeignMessage = errors.New("can't modify someone else's message")
	// ErrDeleteForeignMessage refuses deleting another member's message
	// without moderation rights (the DELETE path, BeforeDelete).
	ErrDeleteForeignMessage = errors.New("can't delete not own message")
)

type ChatHandler struct {
	repository      chatrepository.Repository
	subscription    chatsubscription.Manager
	indexerStore    objectstore.IndexerStore
	chatFullId      domain.FullID
	currentIdentity string
	myParticipantId string
	// aclList resolves permissions for the change author at the change's AclHeadId.
	// May be nil in unit tests that exercise pre-Admin behavior.
	aclList list.AclList
	// reactionsCounterEpoch is the unix timestamp after which reactions counters are tracked
	reactionsCounterEpoch int64
	// forceNotRead forces handler to mark all messages as not read. It's useful for unit testing
	forceNotRead bool
}

// canModerateAt reports whether the change author had Owner or Admin permission
// at the ACL head referenced by the change. Falls back to false when the AclList
// is unavailable or the head/identity cannot be resolved — preserving the
// historical creator-only restriction in those cases.
func (d *ChatHandler) canModerateAt(authorAccount, aclHeadId string) bool {
	if d.aclList == nil || aclHeadId == "" {
		return false
	}
	authorKey, err := crypto.DecodeAccountAddress(authorAccount)
	if err != nil {
		return false
	}
	d.aclList.RLock()
	defer d.aclList.RUnlock()
	perms, err := d.aclList.AclState().PermissionsAtRecord(aclHeadId, authorKey)
	if err != nil {
		return false
	}
	return perms.IsOwner() || perms.IsAdmin()
}

func (d *ChatHandler) CollectionName() string {
	return CollectionName
}

func (d *ChatHandler) Init(ctx context.Context, s *storestate.StoreState) (err error) {
	_, err = s.Collection(ctx, CollectionName)
	return
}

func (d *ChatHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) error {
	coll, err := ch.State.Collection(ctx, CollectionName)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	_, err = coll.FindId(ctx, ch.Value.GetString("id"))
	if err == nil {
		// The message is already stored: this create is being replayed over an existing
		// store (e.g. the one-time full-tree replay storeApply runs for a store without
		// the fullyReplayed marker, which a reindex triggers for every chat). The insert
		// would hit ErrDocExists anyway; bail out before mutating the subscription
		// counters, so already-read messages are not resurrected as unread.
		return storestate.ErrIgnore
	}
	if !errors.Is(err, anystore.ErrDocNotFound) {
		return fmt.Errorf("check for existing message: %w", err)
	}
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

	if err = d.indexerStore.AddChatMessageToIndexQueue(context.Background(), d.chatFullId, msg.OrderId); err != nil {
		return fmt.Errorf("add chat message to full text index queue: %w", err)
	}
	if _, err = d.repository.RecordMessage(ctx, msg.Id); err != nil {
		return errors.Join(storestate.ErrCritical, fmt.Errorf("record message history: %w", err))
	}

	d.subscription.Lock()
	defer d.subscription.Unlock()
	d.subscription.UpdateMessageCount(1)
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

	msg.MarshalAnyenc(ch.Value, ch.Arena)

	return nil
}

func (d *ChatHandler) BeforeModify(ctx context.Context, ch storestate.ChangeOp) (mode storestate.ModifyMode, err error) {
	if err = d.indexerStore.AddChatMessageToIndexQueue(context.Background(), d.chatFullId, ch.Change.Order); err != nil {
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
	if message.Creator != ch.Change.Creator && !d.canModerateAt(ch.Change.Creator, ch.Change.AclHeadId) {
		return storestate.DeleteModeDelete, ErrDeleteForeignMessage
	}

	d.subscription.Lock()
	defer d.subscription.Unlock()
	d.subscription.Delete(messageId)

	if err = d.indexerStore.AddChatMessageDeleteToIndexQueue(context.Background(), d.chatFullId, messageId); err != nil {
		log.With(zap.String("chatId", d.chatFullId.ObjectID), zap.String("messageId", messageId), zap.Error(err)).
			Error("failed to add message to fulltext delete queue")
	}

	return storestate.DeleteModeDelete, nil
}

func (d *ChatHandler) UpgradeKeyModifier(ch storestate.ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		if len(key.KeyPath) == 0 {
			return nil, false, fmt.Errorf("no key path")
		}

		path := key.KeyPath[0]

		// Capture old reactions state BEFORE modification (for new-reaction detection).
		var oldReactions *model.ChatMessageReactions
		if path == chatmodel.ReactionsKey {
			if oldMsg, unmarshalErr := chatmodel.UnmarshalMessage(v); unmarshalErr == nil {
				oldReactions = oldMsg.GetReactions()
			}
		}

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
				if err := d.handleReactionsModify(ch, key, oldReactions, msg, result, a); err != nil {
					return v, false, err
				}
			case chatmodel.ContentKey:
				creator := msg.Creator
				if creator != ch.Change.Creator {
					return v, false, errors.Join(storestate.ErrValidation, ErrModifyForeignMessage)
				}
				msg.ModifiedAt = ch.Change.Timestamp
				msg.MarshalAnyenc(result, a)
				d.subscription.UpdateFull(msg)
			case chatmodel.PinnedKey:
				d.subscription.UpdatePinned(msg)
			default:
				return nil, false, fmt.Errorf("invalid key path %s", key.KeyPath)
			}
		}

		return result, modified, nil
	})
}

func (d *ChatHandler) handleReactionsModify(
	ch storestate.ChangeOp,
	key *pb.KeyModify,
	oldReactions *model.ChatMessageReactions,
	msg *chatmodel.Message,
	result *anyenc.Value,
	a *anyenc.Arena,
) error {
	// Do not parse json, just trim "
	identity := strings.Trim(key.ModifyValue, `"`)
	if identity != ch.Change.Creator {
		return errors.Join(storestate.ErrValidation, fmt.Errorf("can't toggle someone else's reactions"))
	}
	// TODO Count validation

	reactionsCounterEnabled := ch.Change.Timestamp > d.reactionsCounterEpoch

	// Track unread reaction changes on current user's messages from other users
	if reactionsCounterEnabled && msg.Creator == d.currentIdentity && ch.Change.Creator != d.currentIdentity && len(key.KeyPath) > 1 {
		emoji := key.KeyPath[1]
		wasPresent := isIdentityInReactions(oldReactions, emoji, identity)
		isPresent := isIdentityInReactions(msg.GetReactions(), emoji, identity)

		switch {
		case !wasPresent && isPresent:
			d.onReactionAdded(ch, msg, emoji, identity, result, a)
		case wasPresent && !isPresent:
			d.onReactionRemoved(msg, emoji, identity, result, a)
		}
	}

	d.subscription.UpdateReactions(msg)
	return nil
}

func (d *ChatHandler) onReactionAdded(
	ch storestate.ChangeOp,
	msg *chatmodel.Message,
	emoji, identity string,
	result *anyenc.Value,
	a *anyenc.Arena,
) {
	msg.AddUnreadReaction(emoji, identity, chatmodel.ReactionChangeEntry{
		ChangeId: ch.Change.Id,
		OrderId:  ch.Change.Order,
	})
	msg.UnreadReaction = true
	msg.MarshalUnreadReactionIds(result, a)

	d.subscription.UpdateChatState(func(state *model.ChatState) *model.ChatState {
		if msg.OrderId > state.UnreadReactionOrderId {
			state.UnreadReactionOrderId = msg.OrderId
		}
		return state
	})
	d.subscription.UpdateReactionReadStatus(msg.Id, true)
}

func (d *ChatHandler) onReactionRemoved(
	msg *chatmodel.Message,
	emoji, identity string,
	result *anyenc.Value,
	a *anyenc.Arena,
) {
	empty := msg.RemoveUnreadReaction(emoji, identity)
	msg.MarshalUnreadReactionIds(result, a)
	if empty {
		msg.UnreadReaction = false
		d.subscription.UpdateReactionReadStatus(msg.Id, false)
		d.subscription.ForceReloadReactionState()
	}
}

func isIdentityInReactions(reactions *model.ChatMessageReactions, emoji string, identity string) bool {
	if reactions == nil {
		return false
	}
	identityList, ok := reactions.GetReactions()[emoji]
	if !ok {
		return false
	}
	for _, id := range identityList.GetIds() {
		if id == identity {
			return true
		}
	}
	return false
}
