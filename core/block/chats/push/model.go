package push

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

const ChatsTopicName = "chats"

type Type int

const ChatMessage Type = 1

type Payload struct {
	SpaceId           string             `json:"spaceId,omitempty"`
	SenderId          string             `json:"senderId"`
	Type              Type               `json:"type"`
	NewMessagePayload *NewMessagePayload `json:"newMessage,omitempty"`
}

func MakePushPayload(spaceId, accountId, chatId string, message *model.ChatMessage) *Payload {
	return &Payload{
		SpaceId:           spaceId,
		SenderId:          accountId,
		Type:              ChatMessage,
		NewMessagePayload: makeNewMessagePayload(chatId, message),
	}
}

type NewMessagePayload struct {
	ChatId string `json:"chatId"`
	MsgId  string `json:"msgId"`
	Text   string `json:"text"`
}

func makeNewMessagePayload(chatId string, message *model.ChatMessage) *NewMessagePayload {
	return &NewMessagePayload{
		ChatId: chatId,
		MsgId:  message.Id,
		Text:   message.Message.Text,
	}
}
