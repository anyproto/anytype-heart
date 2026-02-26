package chatmodel

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textUtil "github.com/anyproto/anytype-heart/util/text"
)

type CounterType int

const (
	CounterTypeMessage = CounterType(iota)
	CounterTypeMention
)

const (
	DiffManagerMessages = "messages"
	DiffManagerMentions = "mentions"

	MaxMessageLength = 4000
)

func (t CounterType) DiffManagerName() string {
	switch t {
	case CounterTypeMessage:
		return DiffManagerMessages
	case CounterTypeMention:
		return DiffManagerMentions
	default:
		return "unknown"
	}
}

const (
	CreatorKey              = "creator"
	CreatedAtKey            = "createdAt"
	ModifiedAtKey           = "modifiedAt"
	ReactionsKey            = "reactions"
	ContentKey              = "content"
	ReadKey                 = "read"
	MentionReadKey          = "mentionRead"
	HasMentionKey           = "hasMention"
	StateIdKey              = "stateId"
	OrderKey                = "_o"
	SyncedKey               = "synced"
	PinnedKey               = "pinned"
	ReactionReadChangeIdKey  = "rReadChId"
	ReactionReadChangeIdsKey = "rReadChIds"
	ReactionReadOrderIdKey   = "rReadOrdId"
)

// ReactionChangeEntry tracks the change that added an unread reaction.
type ReactionChangeEntry struct {
	ChangeId string
	OrderId  string
}

type Message struct {
	*model.ChatMessage

	// UnreadReactionIds tracks individual unread reactions: emoji → identity → entry.
	// This is a local-only field (not in protobuf) used to detect when reactions are
	// removed and to update the unread state accordingly.
	UnreadReactionIds map[string]map[string]ReactionChangeEntry
}

func (m *Message) Clone() *Message {
	cloned := &Message{
		ChatMessage: proto.Clone(m.ChatMessage).(*model.ChatMessage),
	}
	if m.UnreadReactionIds != nil {
		cloned.UnreadReactionIds = make(map[string]map[string]ReactionChangeEntry, len(m.UnreadReactionIds))
		for emoji, identities := range m.UnreadReactionIds {
			identitiesCopy := make(map[string]ReactionChangeEntry, len(identities))
			for id, entry := range identities {
				identitiesCopy[id] = entry
			}
			cloned.UnreadReactionIds[emoji] = identitiesCopy
		}
	}
	return cloned
}

// AddUnreadReaction records an unread reaction from identity on emoji.
func (m *Message) AddUnreadReaction(emoji, identity string, entry ReactionChangeEntry) {
	if m.UnreadReactionIds == nil {
		m.UnreadReactionIds = make(map[string]map[string]ReactionChangeEntry)
	}
	if m.UnreadReactionIds[emoji] == nil {
		m.UnreadReactionIds[emoji] = make(map[string]ReactionChangeEntry)
	}
	m.UnreadReactionIds[emoji][identity] = entry
}

// RemoveUnreadReaction removes an unread reaction entry. Returns true if the map is now empty.
func (m *Message) RemoveUnreadReaction(emoji, identity string) (empty bool) {
	if m.UnreadReactionIds == nil {
		return true
	}
	identities := m.UnreadReactionIds[emoji]
	if identities == nil {
		return len(m.UnreadReactionIds) == 0
	}
	delete(identities, identity)
	if len(identities) == 0 {
		delete(m.UnreadReactionIds, emoji)
	}
	return len(m.UnreadReactionIds) == 0
}

// MarshalUnreadReactionIds serializes the UnreadReactionIds to the given anyenc value,
// updating rReadChIds, rReadChId, and rReadOrdId fields.
func (m *Message) MarshalUnreadReactionIds(v *anyenc.Value, arena *anyenc.Arena) {
	if len(m.UnreadReactionIds) > 0 {
		chIds := arena.NewObject()
		for emoji, identities := range m.UnreadReactionIds {
			emojiObj := arena.NewObject()
			for identity, entry := range identities {
				entryObj := arena.NewObject()
				entryObj.Set("c", arena.NewString(entry.ChangeId))
				if entry.OrderId != "" {
					entryObj.Set("o", arena.NewString(entry.OrderId))
				}
				emojiObj.Set(identity, entryObj)
			}
			chIds.Set(emoji, emojiObj)
		}
		v.Set(ReactionReadChangeIdsKey, chIds)
		v.Set(ReactionReadChangeIdKey, arena.NewString(m.PickRemainingChangeId()))
		if orderId := m.pickOrderId(); orderId != "" {
			v.Set(ReactionReadOrderIdKey, arena.NewString(orderId))
		} else {
			v.Del(ReactionReadOrderIdKey)
		}
	} else {
		v.Del(ReactionReadChangeIdsKey)
		v.Del(ReactionReadChangeIdKey)
		v.Del(ReactionReadOrderIdKey)
	}
}

// pickOrderId returns the OrderId from the message itself (for indexing purposes).
func (m *Message) pickOrderId() string {
	return m.ChatMessage.GetOrderId()
}

// PickRemainingChangeId returns an arbitrary change ID from the unread reactions map.
func (m *Message) PickRemainingChangeId() string {
	for _, identities := range m.UnreadReactionIds {
		for _, entry := range identities {
			return entry.ChangeId
		}
	}
	return ""
}

type MessagesGetter interface {
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*Message, error)
}

func (m *Message) IsCurrentUserMentioned(ctx context.Context, myParticipantId string, myIdentity string, repo MessagesGetter) (bool, error) {
	for _, mark := range m.Message.Marks {
		if mark.Type == model.BlockContentTextMark_Mention && mark.Param == myParticipantId {
			return true, nil
		}
	}

	if m.ReplyToMessageId != "" {
		msgs, err := repo.GetMessagesByIds(ctx, []string{m.ReplyToMessageId})
		if err != nil {
			return false, fmt.Errorf("get messages by id: %w", err)
		}
		if len(msgs) == 1 {
			msg := msgs[0]
			if msg.Creator == myIdentity {
				return true, nil
			}
		}
	}

	return false, nil
}

func (m *Message) MentionIdentities(ctx context.Context, repo MessagesGetter) ([]string, error) {
	var mentions []string
	for _, mark := range m.Message.Marks {
		if mark.Type == model.BlockContentTextMark_Mention {
			if identity := extractIdentity(mark.Param); identity != "" {
				mentions = append(mentions, identity)
			}
		}
	}
	if m.ReplyToMessageId != "" {
		msgs, err := repo.GetMessagesByIds(ctx, []string{m.ReplyToMessageId})
		if err != nil {
			return nil, fmt.Errorf("get messages by id: %w", err)
		}
		if len(msgs) == 1 {
			msg := msgs[0]
			mentions = append(mentions, msg.Creator)
		}
	}
	return mentions, nil
}

func (m *Message) Validate() error {
	utf16text := textUtil.StrToUTF16(m.Message.Text)

	if len(utf16text) > MaxMessageLength {
		return fmt.Errorf("message text exceeds maximum length of %d characters", MaxMessageLength)
	}

	for _, mark := range m.Message.Marks {
		if mark.Range.From < 0 {
			return fmt.Errorf("invalid range.from")
		}
		if mark.Range.To < 0 {
			return fmt.Errorf("invalid range.to")
		}
		if mark.Range.From > mark.Range.To {
			return fmt.Errorf("range.from should be less than range.to")
		}
		if int(mark.Range.From) >= len(utf16text) {
			return fmt.Errorf("invalid range.from")
		}
		if int(mark.Range.To) > len(utf16text) {
			return fmt.Errorf("invalid range.to")
		}
	}

	for _, att := range m.Attachments {
		if att.Target == "" {
			return fmt.Errorf("attachment target is empty")
		}
		switch att.Type {
		case model.ChatMessageAttachment_FILE,
			model.ChatMessageAttachment_IMAGE,
			model.ChatMessageAttachment_LINK:
			continue
		default:
			return fmt.Errorf("unknown attachment type: %v", att.Type)
		}
	}

	return nil
}

func extractIdentity(participantId string) string {
	idx := strings.LastIndex(participantId, "_")
	return participantId[idx+1:]
}

func UnmarshalMessage(val *anyenc.Value) (*Message, error) {
	return newMessageWrapper(val).toModel()
}

type messageUnmarshaller struct {
	val *anyenc.Value
}

func newMessageWrapper(val *anyenc.Value) *messageUnmarshaller {
	return &messageUnmarshaller{val: val}
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
  },
  "pinned": false,
}

*/

func (m *Message) MarshalAnyenc(marshalTo *anyenc.Value, arena *anyenc.Arena) {
	message := arena.NewObject()
	message.Set("text", arena.NewString(m.Message.Text))
	message.Set("style", arena.NewNumberInt(int(m.Message.Style)))
	marks := arena.NewArray()
	for i, inMark := range m.Message.Marks {
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
	for _, inAttachment := range m.Attachments {
		if inAttachment.Target == "" {
			// we should catch this earlier on Validate()
			continue
		}
		attachment := arena.NewObject()
		attachment.Set("type", arena.NewNumberInt(int(inAttachment.Type)))
		attachments.Set(inAttachment.Target, attachment)
	}

	content := arena.NewObject()
	content.Set("message", message)
	content.Set("attachments", attachments)

	reactions := arena.NewObject()
	for emoji, inReaction := range m.GetReactions().GetReactions() {
		identities := arena.NewArray()
		for j, identity := range inReaction.Ids {
			identities.SetArrayItem(j, arena.NewString(identity))
		}
		reactions.Set(emoji, identities)
	}

	marshalTo.Set("id", arena.NewString(m.Id))
	marshalTo.Set(CreatorKey, arena.NewString(m.Creator))
	marshalTo.Set(CreatedAtKey, arena.NewNumberInt(int(m.CreatedAt)))
	marshalTo.Set(ModifiedAtKey, arena.NewNumberInt(int(m.ModifiedAt)))
	marshalTo.Set("replyToMessageId", arena.NewString(m.ReplyToMessageId))
	marshalTo.Set(ContentKey, content)
	marshalTo.Set(ReadKey, arenaNewBool(arena, m.Read))
	marshalTo.Set(MentionReadKey, arenaNewBool(arena, m.MentionRead))
	marshalTo.Set(HasMentionKey, arenaNewBool(arena, m.HasMention))
	marshalTo.Set(StateIdKey, arena.NewString(m.StateId))
	marshalTo.Set(ReactionsKey, reactions)
	marshalTo.Set(SyncedKey, arenaNewBool(arena, m.Synced))
	if m.Pinned {
		// we save Pinned value only in case of =true for good sparse index search
		marshalTo.Set(PinnedKey, arenaNewBool(arena, m.Pinned))
	} else {
		marshalTo.Del(PinnedKey)
	}

	m.MarshalUnreadReactionIds(marshalTo, arena)
}

func arenaNewBool(a *anyenc.Arena, value bool) *anyenc.Value {
	if value {
		return a.NewTrue()
	} else {
		return a.NewFalse()
	}
}

func (m *messageUnmarshaller) toModel() (*Message, error) {
	unreadReactionIds, lastUnreadReactionOrderId := m.unreadReactionIdsToModel()
	return &Message{
		ChatMessage: &model.ChatMessage{
			Id:                        string(m.val.GetStringBytes("id")),
			Creator:                   string(m.val.GetStringBytes(CreatorKey)),
			CreatedAt:                 int64(m.val.GetInt(CreatedAtKey)),
			ModifiedAt:                int64(m.val.GetInt(ModifiedAtKey)),
			StateId:                   m.val.GetString(StateIdKey),
			OrderId:                   string(m.val.GetStringBytes("_o", "id")),
			ReplyToMessageId:          string(m.val.GetStringBytes("replyToMessageId")),
			Message:                   m.contentToModel(),
			Read:                      m.val.GetBool(ReadKey),
			MentionRead:               m.val.GetBool(MentionReadKey),
			Attachments:               m.attachmentsToModel(),
			Reactions:                 m.reactionsToModel(),
			Synced:                    m.val.GetBool(SyncedKey),
			HasMention:                m.val.GetBool(HasMentionKey),
			Pinned:                    m.val.GetBool(PinnedKey),
			UnreadReaction:            len(unreadReactionIds) > 0 || m.val.GetString(ReactionReadChangeIdKey) != "",
			LastUnreadReactionOrderId: lastUnreadReactionOrderId,
		},
		UnreadReactionIds: unreadReactionIds,
	}, nil
}

func (m *messageUnmarshaller) unreadReactionIdsToModel() (map[string]map[string]ReactionChangeEntry, string) {
	lastUnreadReactionOrderId := m.val.GetString(ReactionReadOrderIdKey)

	chIdsObj := m.val.GetObject(ReactionReadChangeIdsKey)
	if chIdsObj == nil {
		return nil, lastUnreadReactionOrderId
	}
	result := make(map[string]map[string]ReactionChangeEntry)
	chIdsObj.Visit(func(emoji []byte, emojiVal *anyenc.Value) {
		emojiObj := emojiVal.GetObject()
		if emojiObj == nil {
			return
		}
		identities := make(map[string]ReactionChangeEntry)
		emojiObj.Visit(func(identity []byte, entryVal *anyenc.Value) {
			identities[string(identity)] = ReactionChangeEntry{
				ChangeId: entryVal.GetString("c"),
				OrderId:  entryVal.GetString("o"),
			}
		})
		if len(identities) > 0 {
			result[string(emoji)] = identities
		}
	})
	if len(result) == 0 {
		return nil, lastUnreadReactionOrderId
	}
	return result, lastUnreadReactionOrderId
}

func (m *messageUnmarshaller) contentToModel() *model.ChatMessageMessageContent {
	inMarks := m.val.GetArray(ContentKey, "message", "marks")
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
	return &model.ChatMessageMessageContent{
		Text:  string(m.val.GetStringBytes(ContentKey, "message", "text")),
		Style: model.BlockContentTextStyle(m.val.GetInt("content", "message", "style")),
		Marks: marks,
	}
}

func (m *messageUnmarshaller) attachmentsToModel() []*model.ChatMessageAttachment {
	inAttachments := m.val.GetObject(ContentKey, "attachments")
	var attachments []*model.ChatMessageAttachment
	if inAttachments != nil {
		attachments = make([]*model.ChatMessageAttachment, 0, inAttachments.Len())
		inAttachments.Visit(func(targetObjectId []byte, inAttachment *anyenc.Value) {
			attachments = append(attachments, &model.ChatMessageAttachment{
				Target: string(targetObjectId),
				Type:   model.ChatMessageAttachmentAttachmentType(inAttachment.GetInt("type")),
			})
		})
	}
	return attachments
}

func (m *messageUnmarshaller) reactionsToModel() *model.ChatMessageReactions {
	inReactions := m.val.GetObject(ReactionsKey)
	reactions := &model.ChatMessageReactions{}
	if inReactions != nil {
		inReactions.Visit(func(emoji []byte, inReaction *anyenc.Value) {
			inReactionArr := inReaction.GetArray()
			identities := make([]string, 0, len(inReactionArr))
			for _, identity := range inReactionArr {
				identities = append(identities, string(identity.GetStringBytes()))
			}
			if len(identities) > 0 {
				if reactions.Reactions == nil {
					reactions.Reactions = make(map[string]*model.ChatMessageReactionsIdentityList)
				}
				reactions.Reactions[string(emoji)] = &model.ChatMessageReactionsIdentityList{
					Ids: identities,
				}
			}
		})
	}
	return reactions
}
