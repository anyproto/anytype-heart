package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/cmd/assistant/mcp"
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

	mcpClients []*mcp.Client
}

func (c *Chatter) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)

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

func (c *Chatter) listTools() ([]openai.Tool, error) {
	cli := c.mcpClients[0]

	mcpTools, err := cli.ListTools()
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	tools := make([]openai.Tool, 0, len(mcpTools))
	for _, mcpTool := range mcpTools {
		tools = append(tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        mcpTool.Name,
				Description: mcpTool.Description,
				Parameters:  mcpTool.InputSchema,
			},
		})
	}
	return tools, nil
}

func (c *Chatter) sendRequest(ctx context.Context, messages []openai.ChatCompletionMessage) error {
	if c.maxRequests <= 0 {
		return nil
	}
	c.maxRequests--

	tools, err := c.listTools()
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	var messageText string
	for {
		compResp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:    openai.GPT4Turbo,
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			return fmt.Errorf("create chat completion: %w", err)
		}

		result := compResp.Choices[0]
		messageText = result.Message.Content

		// TODO Send message for tool call
		if result.FinishReason == openai.FinishReasonToolCalls {
			// TODO Send message and wait for approve

			toolCalls := make([]openai.ToolCall, 0, len(tools))
			callResultMessages := make([]openai.ChatCompletionMessage, 0, len(tools))
			for _, tool := range result.Message.ToolCalls {
				var args map[string]any
				err = json.Unmarshal([]byte(tool.Function.Arguments), &args)
				if err != nil {
					return fmt.Errorf("unmarshal tool arguments: %w", err)
				}

				callRes, err := c.mcpClients[0].CallTool(tool.Function.Name, args)
				if err != nil {
					return fmt.Errorf("call tool: %w", err)
				}

				var callContent strings.Builder
				for _, c := range callRes.Content {
					if c.Type == "text" {
						callContent.WriteString(c.Text)
						callContent.WriteString("\n")
					}
				}

				callResultMessages = append(callResultMessages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					ToolCallID: tool.ID,
					Name:       tool.Function.Name,
					Content:    callContent.String(),
				})

				toolCalls = append(toolCalls, openai.ToolCall{
					Type:     openai.ToolTypeFunction,
					ID:       tool.ID,
					Function: tool.Function,
				})

			}
			fmt.Println("tool calls finished")

			messages = append(messages, openai.ChatCompletionMessage{
				Role:      openai.ChatMessageRoleAssistant,
				ToolCalls: toolCalls,
			})
			messages = append(messages, callResultMessages...)
		} else {
			break
		}

	}

	_, err = c.chatService.AddMessage(ctx, nil, c.chatObjectId, &chatobject.Message{
		ChatMessage: &model.ChatMessage{
			Message: &model.ChatMessageMessageContent{
				Text: messageText,
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
