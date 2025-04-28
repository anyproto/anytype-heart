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

func MakePushPayload(spaceId, accountId, chatId string, messageId string, messageText string) *Payload {
	return &Payload{
		SpaceId:  spaceId,
		SenderId: accountId,
		Type:     ChatMessage,
		NewMessagePayload: &NewMessagePayload{
			ChatId: chatId,
			MsgId:  messageId,
			Text:   messageText,
		},
	}
}

type NewMessagePayload struct {
	ChatId string `json:"chatId"`
	MsgId  string `json:"msgId"`
	Text   string `json:"text"`
}
