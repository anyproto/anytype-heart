package chatpush

const ChatsTopicName = "chats"

type Type int

const ChatMessage Type = 1

type Payload struct {
	SpaceId           string             `json:"spaceId,omitempty"`
	SenderId          string             `json:"senderId"`
	Type              Type               `json:"type"`
	NewMessagePayload *NewMessagePayload `json:"newMessage,omitempty"`
}

type NewMessagePayload struct {
	ChatId         string        `json:"chatId"`
	MsgId          string        `json:"msgId"`
	SpaceName      string        `json:"spaceName"`
	SenderName     string        `json:"senderName"`
	Text           string        `json:"text"`
	HasAttachments bool          `json:"hasAttachments"`
	Attachments    []*Attachment `json:"attachments"`
}

type Attachment struct {
	// See model.ChatMessageAttachmentAttachmentType
	Type int `json:"type"`
}
