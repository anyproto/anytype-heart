package chatobject

import (
	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	creatorKey     = "creator"
	createdAtKey   = "createdAt"
	modifiedAtKey  = "modifiedAt"
	reactionsKey   = "reactions"
	contentKey     = "content"
	readKey        = "read"
	mentionReadKey = "mentionRead"
	addedKey       = "a"
	orderKey       = "_o"
)

type messageWrapper struct {
	val   *anyenc.Value
	arena *anyenc.Arena
}

func newMessageWrapper(arena *anyenc.Arena, val *anyenc.Value) *messageWrapper {
	return &messageWrapper{arena: arena, val: val}
}

func (m *messageWrapper) getCreator() string {
	return string(m.val.GetStringBytes(creatorKey))
}

func (m *messageWrapper) setCreator(v string) {
	m.val.Set(creatorKey, m.arena.NewString(v))
}

func (m *messageWrapper) setRead(v bool) {
	if v {
		m.val.Set(readKey, m.arena.NewTrue())
	} else {
		m.val.Set(readKey, m.arena.NewFalse())
	}
}

func (m *messageWrapper) setMentionRead(v bool) {
	if v {
		m.val.Set(mentionReadKey, m.arena.NewTrue())
	} else {
		m.val.Set(mentionReadKey, m.arena.NewFalse())
	}
}

func (m *messageWrapper) setCreatedAt(v int64) {
	m.val.Set(createdAtKey, m.arena.NewNumberInt(int(v)))
}

func (m *messageWrapper) setAddedAt(v int64) {
	m.val.Set(addedKey, m.arena.NewNumberInt(int(v)))
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

func marshalModel(arena *anyenc.Arena, msg *model.ChatMessage) *anyenc.Value {
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
	root.Set(creatorKey, arena.NewString(msg.Creator))
	root.Set(createdAtKey, arena.NewNumberInt(int(msg.CreatedAt)))
	root.Set(modifiedAtKey, arena.NewNumberInt(int(msg.ModifiedAt)))
	root.Set("replyToMessageId", arena.NewString(msg.ReplyToMessageId))
	root.Set(contentKey, content)
	var read *anyenc.Value
	if msg.Read {
		read = arena.NewTrue()
	} else {
		read = arena.NewFalse()
	}
	root.Set(readKey, read)

	root.Set(reactionsKey, reactions)
	return root
}

func (m *messageWrapper) toModel() *model.ChatMessage {
	return &model.ChatMessage{
		Id:               string(m.val.GetStringBytes("id")),
		Creator:          string(m.val.GetStringBytes(creatorKey)),
		CreatedAt:        int64(m.val.GetInt(createdAtKey)),
		ModifiedAt:       int64(m.val.GetInt(modifiedAtKey)),
		AddedAt:          int64(m.val.GetInt(addedKey)),
		OrderId:          string(m.val.GetStringBytes("_o", "id")),
		ReplyToMessageId: string(m.val.GetStringBytes("replyToMessageId")),
		Message:          m.contentToModel(),
		Read:             m.val.GetBool(readKey),
		Attachments:      m.attachmentsToModel(),
		Reactions:        m.reactionsToModel(),
	}
}

func (m *messageWrapper) contentToModel() *model.ChatMessageMessageContent {
	inMarks := m.val.GetArray(contentKey, "message", "marks")
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
		Text:  string(m.val.GetStringBytes(contentKey, "message", "text")),
		Style: model.BlockContentTextStyle(m.val.GetInt("content", "message", "style")),
		Marks: marks,
	}
}

func (m *messageWrapper) attachmentsToModel() []*model.ChatMessageAttachment {
	inAttachments := m.val.GetObject(contentKey, "attachments")
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

func (m *messageWrapper) reactionsToModel() *model.ChatMessageReactions {
	inReactions := m.val.GetObject(reactionsKey)
	reactions := &model.ChatMessageReactions{
		Reactions: map[string]*model.ChatMessageReactionsIdentityList{},
	}
	if inReactions != nil {
		inReactions.Visit(func(emoji []byte, inReaction *anyenc.Value) {
			inReactionArr := inReaction.GetArray()
			identities := make([]string, 0, len(inReactionArr))
			for _, identity := range inReactionArr {
				identities = append(identities, string(identity.GetStringBytes()))
			}
			if len(identities) > 0 {
				reactions.Reactions[string(emoji)] = &model.ChatMessageReactionsIdentityList{
					Ids: identities,
				}
			}
		})
	}
	return reactions
}
