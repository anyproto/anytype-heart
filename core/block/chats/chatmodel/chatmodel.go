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
	CreatorKey     = "creator"
	CreatedAtKey   = "createdAt"
	ModifiedAtKey  = "modifiedAt"
	ReactionsKey   = "reactions"
	ContentKey     = "content"
	ReadKey        = "read"
	MentionReadKey = "mentionRead"
	HasMentionKey  = "hasMention"
	StateIdKey     = "stateId"
	OrderKey       = "_o"
	SyncedKey      = "synced"
	PinnedKey      = "pinned"
)

type Message struct {
	*model.ChatMessage
}

func (m *Message) Clone() *Message {
	return &Message{
		ChatMessage: proto.Clone(m.ChatMessage).(*model.ChatMessage),
	}
}

type MessagesGetter interface {
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*Message, error)
}

// BlocksText returns the concatenated text of all text blocks, separated by newlines.
func (m *Message) BlocksText() string {
	var sb strings.Builder
	for _, block := range m.ChatMessage.Blocks {
		if tb := block.GetText(); tb != nil {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(tb.Text)
		}
	}
	return sb.String()
}

// LinkBlockTargetIds returns all targetObjectIds from link blocks.
func (m *Message) LinkBlockTargetIds() []string {
	var ids []string
	for _, block := range m.ChatMessage.Blocks {
		if lb := block.GetLink(); lb != nil && lb.TargetObjectId != "" {
			ids = append(ids, lb.TargetObjectId)
		}
	}
	return ids
}

func (m *Message) allMarks() []*model.BlockContentTextMark {
	var all []*model.BlockContentTextMark
	all = append(all, m.Message.Marks...)
	for _, block := range m.ChatMessage.Blocks {
		if textBlock := block.GetText(); textBlock != nil {
			all = append(all, textBlock.Marks...)
		}
	}
	return all
}

func (m *Message) IsCurrentUserMentioned(ctx context.Context, myParticipantId string, myIdentity string, repo MessagesGetter) (bool, error) {
	for _, mark := range m.allMarks() {
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
	for _, mark := range m.allMarks() {
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

	if err := validateMarks(m.Message.Marks, utf16text); err != nil {
		return err
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

	for _, block := range m.ChatMessage.Blocks {
		switch {
		case block.GetText() != nil:
			tb := block.GetText()
			utf16 := textUtil.StrToUTF16(tb.Text)
			if len(utf16) > MaxMessageLength {
				return fmt.Errorf("block text exceeds maximum length of %d characters", MaxMessageLength)
			}
			if err := validateMarks(tb.Marks, utf16); err != nil {
				return err
			}
		case block.GetLink() != nil:
			lb := block.GetLink()
			if lb.TargetObjectId == "" {
				return fmt.Errorf("link block targetObjectId is empty")
			}
		default:
			return fmt.Errorf("block content is nil")
		}
	}

	return nil
}

func validateMarks(marks []*model.BlockContentTextMark, utf16text []uint16) error {
	for _, mark := range marks {
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
		marshalMark(arena, marks, i, inMark)
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
	content.Set("blocks", marshalBlocks(arena, m.ChatMessage.Blocks))

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
}

func marshalBlocks(arena *anyenc.Arena, inBlocks []*model.ChatMessageMessageBlock) *anyenc.Value {
	blocks := arena.NewArray()
	for i, inBlock := range inBlocks {
		block := arena.NewObject()
		if tb := inBlock.GetText(); tb != nil {
			textObj := arena.NewObject()
			textObj.Set("text", arena.NewString(tb.Text))
			textObj.Set("style", arena.NewNumberInt(int(tb.Style)))
			textMarks := arena.NewArray()
			for j, inMark := range tb.Marks {
				marshalMark(arena, textMarks, j, inMark)
			}
			textObj.Set("marks", textMarks)
			block.Set("text", textObj)
		} else if lb := inBlock.GetLink(); lb != nil {
			linkObj := arena.NewObject()
			linkObj.Set("targetObjectId", arena.NewString(lb.TargetObjectId))
			linkObj.Set("type", arena.NewNumberInt(int(lb.Type)))
			block.Set("link", linkObj)
		}
		blocks.SetArrayItem(i, block)
	}
	return blocks
}

func marshalMark(arena *anyenc.Arena, arr *anyenc.Value, idx int, inMark *model.BlockContentTextMark) {
	mark := arena.NewObject()
	mark.Set("from", arena.NewNumberInt(int(inMark.Range.From)))
	mark.Set("to", arena.NewNumberInt(int(inMark.Range.To)))
	mark.Set("type", arena.NewNumberInt(int(inMark.Type)))
	if inMark.Param != "" {
		mark.Set("param", arena.NewString(inMark.Param))
	}
	arr.SetArrayItem(idx, mark)
}

func arenaNewBool(a *anyenc.Arena, value bool) *anyenc.Value {
	if value {
		return a.NewTrue()
	} else {
		return a.NewFalse()
	}
}

func (m *messageUnmarshaller) toModel() (*Message, error) {
	return &Message{
		ChatMessage: &model.ChatMessage{
			Id:               string(m.val.GetStringBytes("id")),
			Creator:          string(m.val.GetStringBytes(CreatorKey)),
			CreatedAt:        int64(m.val.GetInt(CreatedAtKey)),
			ModifiedAt:       int64(m.val.GetInt(ModifiedAtKey)),
			StateId:          m.val.GetString(StateIdKey),
			OrderId:          string(m.val.GetStringBytes("_o", "id")),
			ReplyToMessageId: string(m.val.GetStringBytes("replyToMessageId")),
			Message:          m.contentToModel(),
			Read:             m.val.GetBool(ReadKey),
			MentionRead:      m.val.GetBool(MentionReadKey),
			Attachments:      m.attachmentsToModel(),
			Blocks:           m.blocksToModel(),
			Reactions:        m.reactionsToModel(),
			Synced:           m.val.GetBool(SyncedKey),
			HasMention:       m.val.GetBool(HasMentionKey),
			Pinned:           m.val.GetBool(PinnedKey),
		},
	}, nil
}

func (m *messageUnmarshaller) contentToModel() *model.ChatMessageMessageContent {
	inMarks := m.val.GetArray(ContentKey, "message", "marks")
	return &model.ChatMessageMessageContent{
		Text:  string(m.val.GetStringBytes(ContentKey, "message", "text")),
		Style: model.BlockContentTextStyle(m.val.GetInt("content", "message", "style")),
		Marks: unmarshalMarks(inMarks),
	}
}

func (m *messageUnmarshaller) blocksToModel() []*model.ChatMessageMessageBlock {
	inBlocks := m.val.GetArray(ContentKey, "blocks")
	if len(inBlocks) == 0 {
		return nil
	}
	blocks := make([]*model.ChatMessageMessageBlock, 0, len(inBlocks))
	for _, inBlock := range inBlocks {
		block := &model.ChatMessageMessageBlock{}
		if textVal := inBlock.Get("text"); textVal != nil {
			block.Content = &model.ChatMessageMessageBlockContentOfText{
				Text: &model.ChatMessageMessageBlockText{
					Text:  string(textVal.GetStringBytes("text")),
					Style: model.BlockContentTextStyle(textVal.GetInt("style")),
					Marks: unmarshalMarks(textVal.GetArray("marks")),
				},
			}
		} else if linkVal := inBlock.Get("link"); linkVal != nil {
			block.Content = &model.ChatMessageMessageBlockContentOfLink{
				Link: &model.ChatMessageMessageBlockLink{
					TargetObjectId: string(linkVal.GetStringBytes("targetObjectId")),
					Type:           model.ChatMessageMessageBlockLinkLinkType(linkVal.GetInt("type")),
				},
			}
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func unmarshalMarks(inMarks []*anyenc.Value) []*model.BlockContentTextMark {
	marks := make([]*model.BlockContentTextMark, 0, len(inMarks))
	for _, inMark := range inMarks {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: int32(inMark.GetInt("from")),
				To:   int32(inMark.GetInt("to")),
			},
			Type:  model.BlockContentTextMarkType(inMark.GetInt("type")),
			Param: string(inMark.GetStringBytes("param")),
		})
	}
	return marks
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
