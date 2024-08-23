package chatobject

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const collectionName = "chats"
const dataKey = "data"
const creatorKey = "creator"

type StoreObject interface {
	smartblock.SmartBlock

	AddMessage(ctx context.Context, message *model.ChatMessage) (string, error)
	GetMessages(ctx context.Context) ([]*model.ChatMessage, error)
	EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error
	SubscribeLastMessages(ctx context.Context, limit int) ([]*model.ChatMessage, int, error)
	Unsubscribe() error
}

type StoreDbProvider interface {
	GetStoreDb() anystore.DB
}

type AccountService interface {
	AccountID() string
}

type storeObject struct {
	smartblock.SmartBlock

	accountService AccountService
	dbProvider     StoreDbProvider
	storeSource    source.Store
	store          *storestate.StoreState
	eventSender    event.Sender
	subscription   *subscription

	arenaPool *fastjson.ArenaPool
}

func New(sb smartblock.SmartBlock, accountService AccountService, dbProvider StoreDbProvider, eventSender event.Sender) StoreObject {
	return &storeObject{
		SmartBlock:     sb,
		accountService: accountService,
		dbProvider:     dbProvider,
		arenaPool:      &fastjson.ArenaPool{},
		eventSender:    eventSender,
	}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	s.subscription = newSubscription(s.Id(), s.eventSender)

	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.dbProvider.GetStoreDb(), ChatHandler{
		chatId:       s.Id(),
		subscription: s.subscription,
	})
	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	s.store = stateStore

	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}
	s.storeSource = storeSource
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, s.onUpdate)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}

	return nil
}

func (s *storeObject) onUpdate() {
	s.subscription.flush()
}

func (s *storeObject) GetMessages(ctx context.Context) ([]*model.ChatMessage, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	query := coll.Find(nil).Sort("_o.id")
	return s.queryMessages(ctx, query)
}

func (s *storeObject) queryMessages(ctx context.Context, query anystore.Query) ([]*model.ChatMessage, error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	iter, err := query.Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find iter: %w", err)
	}
	var res []*model.ChatMessage
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(iter.Close(), err)
		}

		message := unmarshalMessage(doc.Value())
		res = append(res, message)
	}
	return res, errors.Join(iter.Close(), err)
}

func (s *storeObject) AddMessage(ctx context.Context, message *model.ChatMessage) (string, error) {
	// TODO Use one arena for whole object
	arena := &fastjson.Arena{}
	obj := marshalMessageTo(arena, message)

	builder := storestate.Builder{}
	err := builder.Create(collectionName, storestate.IdFromChange, obj)
	if err != nil {
		return "", fmt.Errorf("create chat: %w", err)
	}

	messageId, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("add change: %w", err)
	}
	return messageId, nil
}

// TODO Temp non-atomic method
func (s *storeObject) EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error {
	arena := &fastjson.Arena{}
	obj := marshalMessageTo(arena, newMessage)

	builder := storestate.Builder{}
	err := builder.Modify(collectionName, messageId, []string{"content"}, pb.ModifyOp_Set, obj.Get("content"))
	if err != nil {
		return fmt.Errorf("modify content: %w", err)
	}
	err = builder.Modify(collectionName, messageId, []string{"reactions"}, pb.ModifyOp_Set, obj.Get("reactions"))
	if err != nil {
		return fmt.Errorf("modify reactions: %w", err)
	}
	_, err = s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("add change: %w", err)
	}
	return nil
}

/*
{
  "id": "<changeCid>", // Unique message identifier
  "creator": "<authorId>",   // Identifier for the message author
  "replyToMessageId": "<messageId>",
  "dateCreated": "<ts>",  // Date and time the message was created
  "dateEdited": "<ts>",  // Date and time the message was last updated; >> for beta
  "wasEdited": false,       // Flag indicating if the message was edited; Sets automatically when content was changed; >> for beta
  "content": { // everything inside can be only edited by the creator
    "message": { // [set]; set all fields at once
      "text": "message text", // The text content of the message part
      "kind": "<partStyle>", // The style/type of the message part (e.g., Paragraph, Quote, Code)
      "marks": [
        {
          "from": 0, // Starting position of the mark in the text
          "to": 100, // Ending position of the mark in the text
          "type": "<markType>" // Type of the mark (e.g., Bold, Italic, Link)
        }
      ]
    },
    "attachments": { // [set], [unset];
      "<attachmentId>": { // use object_id as attachment_id in the first iteration
        "target": "<objectId1>",  // Identifier for the attachment object. todo: we have target in the key, should we remove it from here?
        "type": "<attachmentType>" // Type of attachment (e.g., file, image, link)
      }
  },
  "reactions": { // [addToSet], [pull] to specify the emoji
    "<emoji1>": ["<user_id_1>", "<user_id_2>"], // Users who reacted with this emoji
    "<emoji2>": ["<user_id_3>"] // Users who reacted with this emoji
  }
}

*/

func marshalMessageTo(arena *fastjson.Arena, msg *model.ChatMessage) *fastjson.Value {
	message := arena.NewObject()
	message.Set("text", arena.NewString(msg.Message.Text))
	message.Set("style", arena.NewNumberInt(int(msg.Message.Style)))
	marks := arena.NewArray()
	for i, inMark := range msg.Message.Marks {
		mark := arena.NewObject()
		mark.Set("from", arena.NewNumberInt(int(inMark.Range.From)))
		mark.Set("to", arena.NewNumberInt(int(inMark.Range.To)))
		mark.Set("type", arena.NewNumberInt(int(inMark.Type)))
		if inMark.Param != "" {
			mark.Set("param", arena.NewString(inMark.Param))
		}
		marks.SetArrayItem(i, mark)
	}
	message.Set("marks", marks)

	attachments := arena.NewObject()
	for i, inAttachment := range msg.Attachments {
		attachment := arena.NewObject()
		attachment.Set("type", arena.NewNumberInt(int(inAttachment.Type)))
		attachments.Set(inAttachment.Target, attachment)
		attachments.SetArrayItem(i, attachment)
	}

	content := arena.NewObject()
	content.Set("message", message)
	content.Set("attachments", attachments)

	reactions := arena.NewObject()
	for emoji, inReaction := range msg.GetReactions().GetReactions() {
		identities := arena.NewArray()
		for j, identity := range inReaction.Ids {
			identities.SetArrayItem(j, arena.NewString(identity))
		}
		reactions.Set(emoji, identities)
	}

	root := arena.NewObject()
	root.Set("replyToMessageId", arena.NewString(msg.ReplyToMessageId))
	root.Set("content", content)
	root.Set("reactions", reactions)
	return root
}

func unmarshalMessage(root *fastjson.Value) *model.ChatMessage {
	inMarks := root.GetArray("content", "message", "marks")
	marks := make([]*model.BlockContentTextMark, 0, len(inMarks))
	for _, inMark := range inMarks {
		mark := &model.BlockContentTextMark{
			Range: &model.Range{
				From: int32(inMark.GetInt("from")),
				To:   int32(inMark.GetInt("to")),
			},
			Type:  model.BlockContentTextMarkType(inMark.GetInt("type")),
			Param: string(inMark.GetStringBytes("param")),
		}
		marks = append(marks, mark)
	}
	content := &model.ChatMessageMessageContent{
		Text:  string(root.GetStringBytes("content", "message", "text")),
		Style: model.BlockContentTextStyle(root.GetInt("content", "message", "style")),
		Marks: marks,
	}

	var attachments []*model.ChatMessageAttachment
	inAttachments := root.GetObject("content", "attachments")
	if inAttachments != nil {
		attachments = make([]*model.ChatMessageAttachment, 0, inAttachments.Len())
		inAttachments.Visit(func(targetObjectId []byte, inAttachment *fastjson.Value) {
			attachments = append(attachments, &model.ChatMessageAttachment{
				Target: string(targetObjectId),
				Type:   model.ChatMessageAttachmentAttachmentType(inAttachment.GetInt("type")),
			})
		})
	}

	reactions := &model.ChatMessageReactions{
		Reactions: map[string]*model.ChatMessageReactionsIdentityList{},
	}
	inReactions := root.GetObject("reactions")
	if inReactions != nil {
		inReactions.Visit(func(emoji []byte, inReaction *fastjson.Value) {
			inReactionArr := inReaction.GetArray()
			identities := make([]string, 0, len(inReactionArr))
			for _, identity := range inReactionArr {
				identities = append(identities, string(identity.GetStringBytes()))
			}
			reactions.Reactions[string(emoji)] = &model.ChatMessageReactionsIdentityList{
				Ids: identities,
			}
		})
	}

	return &model.ChatMessage{
		Id:               string(root.GetStringBytes("id")),
		Creator:          string(root.GetStringBytes("creator")),
		CreatedAt:        root.GetInt64("createdAt"),
		OrderId:          string(root.GetStringBytes("_o", "id")),
		ReplyToMessageId: string(root.GetStringBytes("replyToMessageId")),
		Message:          content,
		Attachments:      attachments,
		Reactions:        reactions,
	}
}

func (s *storeObject) SubscribeLastMessages(ctx context.Context, limit int) ([]*model.ChatMessage, int, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, 0, fmt.Errorf("get collection: %w", err)
	}
	query := coll.Find(nil).Sort("-_o.id").Limit(uint(limit))
	messages, err := s.queryMessages(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("query messages: %w", err)
	}
	// reverse
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].OrderId < messages[j].OrderId
	})

	s.subscription.init(messages)

	return messages, 0, nil
}

func (s *storeObject) Unsubscribe() error {
	s.subscription.close()
	return nil
}

func (s *storeObject) Close() error {
	s.subscription.close()
	return s.SmartBlock.Close()
}
