package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

type Chatter struct {
	limit        int
	myIdentity   string
	chatObjectId string
	systemPrompt string

	lock     sync.Mutex
	messages []*model.ChatMessage

	maxRequests int
	chatService chats.Service
	client      *openai.Client
	store       keyvaluestore.Store[string]
}

func (c *Chatter) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ticker.C:
			err := c.handleMessages(ctx)
			if err != nil {
				log.Error("handle messages", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

// needToSend have to be used under lock
func (c *Chatter) needToSend(ctx context.Context) (bool, error) {
	if len(c.messages) == 0 {
		return false, nil
	}

	lastMessage := c.messages[len(c.messages)-1]
	if lastMessage.Creator == c.myIdentity {
		return false, nil
	}

	for _, msg := range c.messages {
		if msg.Creator != c.myIdentity {
			ok, err := c.store.Has(msg.Id)
			if err != nil {
				return false, fmt.Errorf("check if message is handled: %w", err)
			}
			// At least one is unhandled
			if !ok {
				return true, nil
			}
		}
	}

	return false, nil
}

func (c *Chatter) handleMessages(ctx context.Context) error {
	c.lock.Lock()

	needToSend, err := c.needToSend(ctx)
	if err != nil {
		c.lock.Unlock()
		return fmt.Errorf("need to send: %w", err)
	}
	if !needToSend {
		c.lock.Unlock()
		return nil
	}

	toMarkAsRead := make([]string, 0, len(c.messages))
	messages := make([]openai.ChatCompletionMessage, 0, len(c.messages)+1)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: c.systemPrompt,
	})

	for _, msg := range c.messages {
		toMarkAsRead = append(toMarkAsRead, msg.Id)
		if msg.Creator == c.myIdentity {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: msg.Message.Text,
			})
		} else {
			if msg.Message.Text == "" {
				continue
			}
			// TODO Prepend with "${user} said: "
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: msg.Message.Text,
			})
		}
	}
	c.lock.Unlock()

	err = c.sendRequest(ctx, messages)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	for _, msgId := range toMarkAsRead {
		err = c.store.Set(msgId, "handled")
		if err != nil {
			return fmt.Errorf("store handled status: %w", err)
		}
	}
	return nil
}

func (c *Chatter) sendRequest(ctx context.Context, messages []openai.ChatCompletionMessage) error {
	if c.maxRequests <= 0 {
		return nil
	}
	c.maxRequests--

	compResp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    openai.GPT4oMini,
		Messages: messages,
	})
	if err != nil {
		return fmt.Errorf("create chat completion: %w", err)
	}

	completion := compResp.Choices[0].Message.Content

	_, err = c.chatService.AddMessage(ctx, nil, c.chatObjectId, &chatobject.Message{
		ChatMessage: &model.ChatMessage{
			Message: &model.ChatMessageMessageContent{
				Text: completion,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("response in chat: %w", err)
	}
	return nil
}

func (c *Chatter) InitWith(messages []*model.ChatMessage) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.setMessages(messages)
}

func (c *Chatter) setMessages(messages []*model.ChatMessage) {
	c.messages = messages
	if len(c.messages) > c.limit {
		c.messages = c.messages[len(c.messages)-c.limit:]
	}
}

func (c *Chatter) Add(message *model.ChatMessage) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.setMessages(append(c.messages, message))
}
