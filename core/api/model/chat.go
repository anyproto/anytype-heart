package apimodel

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ChatMessage struct {
	Id               string                    `json:"id" example:"msg-abc123"`
	OrderId          string                    `json:"order_id" example:"00a1b2c3d4e5f6"`
	Creator          string                    `json:"creator" example:"AAjEbEzQx9FNvf5LQFEJEGRojZt3L1MRmBFzP2Q"`
	CreatedAt        int64                     `json:"created_at" example:"1717405200"`
	ModifiedAt       int64                     `json:"modified_at" example:"1717405200"`
	ReplyToMessageId string                    `json:"reply_to_message_id,omitempty" example:"msg-def456"`
	Content          ChatMessageContent        `json:"content"`
	Attachments      []ChatAttachment          `json:"attachments"`
	Reactions        map[string][]string       `json:"reactions"`
	Pinned           bool                      `json:"pinned"`
}

type ChatMessageContent struct {
	Text  string     `json:"text" example:"Hello, world!"`
	Style string     `json:"style,omitempty" example:"paragraph"`
	Marks []TextMark `json:"marks,omitempty"`
}

type TextMark struct {
	From  int32  `json:"from" example:"0"`
	To    int32  `json:"to" example:"5"`
	Type  string `json:"type" example:"bold"`
	Param string `json:"param,omitempty" example:""`
}

type ChatAttachment struct {
	Target string `json:"target" example:"bafyreie6n5l5nkbjal37su54cha4coy"`
	Type   string `json:"type" example:"image"`
}

type AddChatMessageRequest struct {
	Text             string           `json:"text" binding:"required" example:"Hello, world!"`
	Style            string           `json:"style,omitempty" example:"paragraph"`
	Marks            []TextMark       `json:"marks,omitempty"`
	Attachments      []ChatAttachment `json:"attachments,omitempty"`
	ReplyToMessageId string           `json:"reply_to_message_id,omitempty" example:"msg-def456"`
}

type EditChatMessageRequest struct {
	Text        string           `json:"text" binding:"required" example:"Updated message text"`
	Style       string           `json:"style,omitempty" example:"paragraph"`
	Marks       []TextMark       `json:"marks,omitempty"`
	Attachments []ChatAttachment `json:"attachments,omitempty"`
}

type ToggleReactionRequest struct {
	Emoji string `json:"emoji" binding:"required" example:"👍"`
}

type AddChatMessageResponse struct {
	MessageId string `json:"message_id" example:"msg-abc123"`
}

type ChatMessageResponse struct {
	Message ChatMessage `json:"message"`
}

type ChatMessagesResponse struct {
	Messages []ChatMessage `json:"messages"`
}

type ChatEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type ChatEventMessageAdded struct {
	Message ChatMessage `json:"message"`
}

type ChatEventMessageUpdated struct {
	Message ChatMessage `json:"message"`
}

type ChatEventMessageDeleted struct {
	Id string `json:"id"`
}

type ChatEventReactionsUpdated struct {
	Id        string              `json:"id"`
	Reactions map[string][]string `json:"reactions"`
}

// Conversion functions

func ChatMessageFromProto(msg *model.ChatMessage) ChatMessage {
	if msg == nil {
		return ChatMessage{
			Attachments: []ChatAttachment{},
			Reactions:   map[string][]string{},
		}
	}

	cm := ChatMessage{
		Id:               msg.Id,
		OrderId:          msg.OrderId,
		Creator:          msg.Creator,
		CreatedAt:        msg.CreatedAt,
		ModifiedAt:       msg.ModifiedAt,
		ReplyToMessageId: msg.ReplyToMessageId,
		Content:          chatMessageContentFromProto(msg.Message),
		Attachments:      chatAttachmentsFromProto(msg.Attachments),
		Reactions:        chatReactionsFromProto(msg.Reactions),
		Pinned:           msg.Pinned,
	}

	if cm.Attachments == nil {
		cm.Attachments = []ChatAttachment{}
	}
	if cm.Reactions == nil {
		cm.Reactions = map[string][]string{}
	}

	return cm
}

func chatMessageContentFromProto(content *model.ChatMessageMessageContent) ChatMessageContent {
	if content == nil {
		return ChatMessageContent{}
	}

	marks := make([]TextMark, 0, len(content.Marks))
	for _, m := range content.Marks {
		tm := TextMark{
			Type:  markTypeToString(m.Type),
			Param: m.Param,
		}
		if m.Range != nil {
			tm.From = m.Range.From
			tm.To = m.Range.To
		}
		marks = append(marks, tm)
	}

	return ChatMessageContent{
		Text:  content.Text,
		Style: textStyleToString(content.Style),
		Marks: marks,
	}
}

func chatAttachmentsFromProto(attachments []*model.ChatMessageAttachment) []ChatAttachment {
	result := make([]ChatAttachment, 0, len(attachments))
	for _, a := range attachments {
		result = append(result, ChatAttachment{
			Target: a.Target,
			Type:   attachmentTypeToString(a.Type),
		})
	}
	return result
}

func chatReactionsFromProto(reactions *model.ChatMessageReactions) map[string][]string {
	if reactions == nil || reactions.Reactions == nil {
		return nil
	}
	result := make(map[string][]string, len(reactions.Reactions))
	for emoji, identityList := range reactions.Reactions {
		ids := make([]string, 0, len(identityList.Ids))
		ids = append(ids, identityList.Ids...)
		result[emoji] = ids
	}
	return result
}

func MessageContentToProto(req AddChatMessageRequest) *model.ChatMessage {
	marks := make([]*model.BlockContentTextMark, 0, len(req.Marks))
	for _, m := range req.Marks {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{From: m.From, To: m.To},
			Type:  stringToMarkType(m.Type),
			Param: m.Param,
		})
	}

	attachments := make([]*model.ChatMessageAttachment, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		attachments = append(attachments, &model.ChatMessageAttachment{
			Target: a.Target,
			Type:   stringToAttachmentType(a.Type),
		})
	}

	return &model.ChatMessage{
		ReplyToMessageId: req.ReplyToMessageId,
		Message: &model.ChatMessageMessageContent{
			Text:  req.Text,
			Style: stringToTextStyle(req.Style),
			Marks: marks,
		},
		Attachments: attachments,
	}
}

func EditContentToProto(req EditChatMessageRequest) *model.ChatMessage {
	marks := make([]*model.BlockContentTextMark, 0, len(req.Marks))
	for _, m := range req.Marks {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{From: m.From, To: m.To},
			Type:  stringToMarkType(m.Type),
			Param: m.Param,
		})
	}

	attachments := make([]*model.ChatMessageAttachment, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		attachments = append(attachments, &model.ChatMessageAttachment{
			Target: a.Target,
			Type:   stringToAttachmentType(a.Type),
		})
	}

	return &model.ChatMessage{
		Message: &model.ChatMessageMessageContent{
			Text:  req.Text,
			Style: stringToTextStyle(req.Style),
			Marks: marks,
		},
		Attachments: attachments,
	}
}

func ConvertEventMessage(msg *pb.EventMessage) *ChatEvent {
	if ev := msg.GetChatAdd(); ev != nil {
		return &ChatEvent{
			Type: "message_added",
			Payload: ChatEventMessageAdded{
				Message: ChatMessageFromProto(ev.Message),
			},
		}
	}
	if ev := msg.GetChatUpdate(); ev != nil {
		return &ChatEvent{
			Type: "message_updated",
			Payload: ChatEventMessageUpdated{
				Message: ChatMessageFromProto(ev.Message),
			},
		}
	}
	if ev := msg.GetChatDelete(); ev != nil {
		return &ChatEvent{
			Type: "message_deleted",
			Payload: ChatEventMessageDeleted{
				Id: ev.Id,
			},
		}
	}
	if ev := msg.GetChatUpdateReactions(); ev != nil {
		return &ChatEvent{
			Type: "reactions_updated",
			Payload: ChatEventReactionsUpdated{
				Id:        ev.Id,
				Reactions: chatReactionsFromProto(ev.Reactions),
			},
		}
	}
	return nil
}

var markTypeMap = map[model.BlockContentTextMarkType]string{
	model.BlockContentTextMark_Strikethrough:   "strikethrough",
	model.BlockContentTextMark_Keyboard:        "keyboard",
	model.BlockContentTextMark_Italic:          "italic",
	model.BlockContentTextMark_Bold:            "bold",
	model.BlockContentTextMark_Underscored:     "underscored",
	model.BlockContentTextMark_Link:            "link",
	model.BlockContentTextMark_TextColor:       "text_color",
	model.BlockContentTextMark_BackgroundColor: "background_color",
	model.BlockContentTextMark_Mention:         "mention",
	model.BlockContentTextMark_Emoji:           "emoji",
	model.BlockContentTextMark_Object:          "object",
}

var reverseMarkTypeMap = reverseMap(markTypeMap)

func markTypeToString(t model.BlockContentTextMarkType) string {
	if s, ok := markTypeMap[t]; ok {
		return s
	}
	return "strikethrough"
}

func stringToMarkType(s string) model.BlockContentTextMarkType {
	if t, ok := reverseMarkTypeMap[s]; ok {
		return t
	}
	return model.BlockContentTextMark_Strikethrough
}

var textStyleMap = map[model.BlockContentTextStyle]string{
	model.BlockContentText_Paragraph: "paragraph",
	model.BlockContentText_Header1:   "header1",
	model.BlockContentText_Header2:   "header2",
	model.BlockContentText_Header3:   "header3",
	model.BlockContentText_Header4:   "header4",
	model.BlockContentText_Quote:     "quote",
	model.BlockContentText_Code:      "code",
	model.BlockContentText_Checkbox:  "checkbox",
	model.BlockContentText_Marked:    "bulleted",
	model.BlockContentText_Numbered:  "numbered",
	model.BlockContentText_Toggle:    "toggle",
	model.BlockContentText_Callout:   "callout",
}

var reverseTextStyleMap = reverseMap(textStyleMap)

func textStyleToString(s model.BlockContentTextStyle) string {
	if str, ok := textStyleMap[s]; ok {
		return str
	}
	return "paragraph"
}

func stringToTextStyle(s string) model.BlockContentTextStyle {
	if t, ok := reverseTextStyleMap[s]; ok {
		return t
	}
	return model.BlockContentText_Paragraph
}

var attachmentTypeMap = map[model.ChatMessageAttachmentAttachmentType]string{
	model.ChatMessageAttachment_FILE:  "file",
	model.ChatMessageAttachment_IMAGE: "image",
	model.ChatMessageAttachment_LINK:  "link",
}

var reverseAttachmentTypeMap = reverseMap(attachmentTypeMap)

func attachmentTypeToString(t model.ChatMessageAttachmentAttachmentType) string {
	if s, ok := attachmentTypeMap[t]; ok {
		return s
	}
	return "file"
}

func stringToAttachmentType(s string) model.ChatMessageAttachmentAttachmentType {
	if t, ok := reverseAttachmentTypeMap[s]; ok {
		return t
	}
	return model.ChatMessageAttachment_FILE
}

func reverseMap[K, V comparable](m map[K]V) map[V]K {
	r := make(map[V]K, len(m))
	for k, v := range m {
		r[v] = k
	}
	return r
}
