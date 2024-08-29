package chatobject

import (
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	creatorKey   = "creator"
	createdAtKey = "createdAt"
	reactionsKey = "reactions"
	contentKey   = "content"
	orderKey     = "_o"
)

type messageModel struct {
	val *fastjson.Value
}

func newMessage(val *fastjson.Value) *messageModel {
	return &messageModel{val: val}
}

func (m *messageModel) getCreator() string {
	return string(m.val.GetStringBytes(creatorKey))
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
	root.Set(contentKey, content)
	root.Set(reactionsKey, reactions)
	return root
}

func unmarshalMessage(root *fastjson.Value) *model.ChatMessage {
	inMarks := root.GetArray(contentKey, "message", "marks")
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
		Text:  string(root.GetStringBytes(contentKey, "message", "text")),
		Style: model.BlockContentTextStyle(root.GetInt("content", "message", "style")),
		Marks: marks,
	}

	var attachments []*model.ChatMessageAttachment
	inAttachments := root.GetObject(contentKey, "attachments")
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
	inReactions := root.GetObject(reactionsKey)
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
		Creator:          string(root.GetStringBytes(creatorKey)),
		CreatedAt:        root.GetInt64(createdAtKey),
		OrderId:          string(root.GetStringBytes("_o", "id")),
		ReplyToMessageId: string(root.GetStringBytes("replyToMessageId")),
		Message:          content,
		Attachments:      attachments,
		Reactions:        reactions,
	}
}
