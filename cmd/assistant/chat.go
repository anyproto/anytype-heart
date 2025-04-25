package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/cmd/assistant/api"
	"github.com/anyproto/anytype-heart/cmd/assistant/mcp"
	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/jsonutil"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

type Chatter struct {
	limit        int
	myIdentity   string
	chatObjectId string
	systemPrompt string

	forceUpdate chan struct{}

	autoApproveToolUsage bool

	lock     sync.Mutex
	messages []*model.ChatMessage
	// message id => tool call
	pendingToolCalls map[string]*toolsCallRequest
	approvedMessages map[string]bool

	maxRequests int
	chatService chats.Service
	client      *openai.Client
	store       keyvaluestore.Store[string]

	toolCallResponses map[string]*mcp.ToolCallResult
	toolsLock         sync.Mutex
	toolRequests      []openai.Tool

	// tool name => mcp client
	toolClients       map[string]ToolCaller
	toolRequestsIndex map[string]int
	localApi          *api.APIClient
	spaceId           string
	sub               subscriber
}

func (c *Chatter) SetTool(tool openai.Tool, caller ToolCaller) {
	c.toolsLock.Lock()
	defer c.toolsLock.Unlock()
	fmt.Println("  registered tool:", tool.Function.Name)
	if index, ok := c.toolRequestsIndex[tool.Function.Name]; ok {
		c.toolRequests[index] = tool
		c.toolClients[tool.Function.Name] = caller
		return
	}
	c.toolRequests = append(c.toolRequests, tool)
	c.toolClients[tool.Function.Name] = caller
	c.toolRequestsIndex[tool.Function.Name] = len(c.toolRequests) - 1
}

type ToolCaller interface {
	CallTool(name string, params any) (*mcp.ToolCallResult, error)
}

func apiBaseURL(listenAddr string) string {
	return fmt.Sprintf("http://%s/v1", listenAddr)
}
func (c *Chatter) InitializeAnytypeApi(config *assistantConfig) error {
	c.localApi = api.NewAPIClient(apiBaseURL(config.apiListenAddr), config.apiKey, config.SpaceId)
	fmt.Println("registered local api client")
	fmt.Printf("  local api client: %s: %s\n", config.apiListenAddr, config.apiKey)
	for _, tool := range api.GetOpenAITools() {
		fmt.Println("  registered tool:", tool.Function.Name)
		c.toolRequests = append(c.toolRequests, tool)
		c.toolClients[tool.Function.Name] = c.localApi
	}

	return nil
}

func (c *Chatter) InitializeMcpClients(config *assistantConfig) error {
	for serverName, cfg := range config.McpServers {
		client, err := mcp.New(cfg)
		if err != nil {
			return fmt.Errorf("new mcp client: %w", err)
		}
		if cfg.Disabled {
			continue
		}

		mcpTools, err := client.ListTools()
		if err != nil {
			return fmt.Errorf("list tools: %w", err)
		}

		fmt.Println("registered server:", serverName)
		tools := make([]openai.Tool, 0, len(mcpTools))
		for _, mcpTool := range mcpTools {
			c.toolClients[mcpTool.Name] = client
			fmt.Println("  registered tool:", mcpTool.Name)
			tools = append(tools, openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        mcpTool.Name,
					Description: mcpTool.Description,
					Parameters:  mcpTool.InputSchema,
				},
			})
		}
		c.toolRequests = append(c.toolRequests, tools...)
	}

	return nil
}

func (c *Chatter) callTool(name string, args map[string]any) (*mcp.ToolCallResult, error) {
	c.toolsLock.Lock()
	cli, ok := c.toolClients[name]
	if !ok {
		return nil, fmt.Errorf("tool %s not found", name)
	}
	c.toolsLock.Unlock()
	fmt.Printf("tool %s request: %s\n", name, jsonutil.Stringify(args))
	result, err := cli.CallTool(name, args)
	if err != nil {
		return nil, fmt.Errorf("call tool %s: %w", name, err)
	}
	fmt.Printf("tool %s response: %s\n", name, jsonutil.Stringify(result))

	return result, nil
}

func (c *Chatter) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)

	for {
		select {
		case <-c.forceUpdate:
			err := c.handleMessages(ctx)
			if err != nil {
				log.Error("handle messages", zap.Error(err))
			}
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

	for messageId := range c.pendingToolCalls {
		if _, ok := c.approvedMessages[messageId]; ok {
			c.lock.Unlock()

			pending, ok := c.pendingToolCalls[messageId]
			if !ok {
				return fmt.Errorf("no such pending tool call")
			}

			err := c.callTools(ctx, pending)
			if err != nil {
				return fmt.Errorf("call pending tools: %w", err)
			}

			c.lock.Lock()
			delete(c.pendingToolCalls, messageId)
			c.lock.Unlock()
			return nil
		}
	}

	needToSend, err := c.needToSend(ctx)
	if err != nil {
		c.lock.Unlock()
		return fmt.Errorf("need to send: %w", err)
	}
	if !needToSend {
		c.lock.Unlock()
		return nil
	}

	currentSpacePrompt := "When user mention 'here', it means in anytype tool in the current space. Current space called '%s'. You have no access to other spaces of the user"
	currentSpacePrompt += fmt.Sprintf("Current datetime is %s", time.Now().Format(time.RFC3339))
	toMarkAsRead := make([]string, 0, len(c.messages))
	messages := make([]openai.ChatCompletionMessage, 0, len(c.messages)+1)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: c.systemPrompt + ". " + currentSpacePrompt,
	})

	for _, msg := range c.messages {
		toMarkAsRead = append(toMarkAsRead, msg.Id)
		if msg.Message.Text == "" {
			continue
		}

		if msg.Creator == c.myIdentity {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: msg.Message.Text,
			})
		} else {
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

type toolCallParams struct {
	id         string
	name       string
	argsString string
	args       map[string]any
}

type toolsCallRequest struct {
	messageId string
	calls     []toolCallParams

	context []openai.ChatCompletionMessage
}

func (c *Chatter) addPendingToolCalls(messageId string, messages []openai.ChatCompletionMessage, toolCalls []openai.ToolCall) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	// TODO Save context
	// TODO Report that user need to approve request

	req, err := createToolsCallRequest(toolCalls, messages)
	if err != nil {
		return err
	}

	req.messageId = messageId

	c.pendingToolCalls[messageId] = req
	return nil
}

func createToolsCallRequest(toolCalls []openai.ToolCall, messages []openai.ChatCompletionMessage) (*toolsCallRequest, error) {
	requests := make([]toolCallParams, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		var args map[string]any
		err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
		if err != nil {
			return nil, fmt.Errorf("unmarshal tool arguments: %w", err)
		}
		requests = append(requests, toolCallParams{
			id:         toolCall.ID,
			name:       toolCall.Function.Name,
			argsString: toolCall.Function.Arguments,
			args:       args,
		})
	}

	req := &toolsCallRequest{

		calls:   requests,
		context: messages,
	}
	return req, nil
}

// call approved tool calls
func (c *Chatter) callTools(ctx context.Context, req *toolsCallRequest) error {
	toolCalls := make([]openai.ToolCall, 0, len(req.calls))
	callResultMessages := make([]openai.ChatCompletionMessage, 0, len(req.calls))

	for _, call := range req.calls {
		callRes, err := c.callTool(call.name, call.args)
		if err != nil {
			return fmt.Errorf("call tool: %w", err)
		}
		c.toolCallResponses[call.id] = callRes

		var callContent strings.Builder
		for _, c := range callRes.Content {
			if c.Type == "text" {
				callContent.WriteString(c.Text)
				callContent.WriteString("\n")
			}
		}

		callResultMessages = append(callResultMessages, openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: call.id,
			Name:       call.name,
			Content:    callContent.String(),
		})

		toolCalls = append(toolCalls, openai.ToolCall{
			Type: openai.ToolTypeFunction,
			ID:   call.id,
			Function: openai.FunctionCall{
				Name:      call.name,
				Arguments: call.argsString,
			},
		})

	}

	messages := req.context
	messages = append(messages, openai.ChatCompletionMessage{
		Role:      openai.ChatMessageRoleAssistant,
		ToolCalls: toolCalls,
	})
	messages = append(messages, callResultMessages...)

	return c.sendRequest(ctx, messages)
}

func (c *Chatter) sendRequest(ctx context.Context, messages []openai.ChatCompletionMessage) error {
	if c.maxRequests <= 0 {
		return nil
	}
	c.maxRequests--

	var messageText string
	compResp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    "gpt-4.1",
		Messages: messages,
		Tools:    c.toolRequests,
	})
	if err != nil {
		return fmt.Errorf("create chat completion: %w", err)
	}

	result := compResp.Choices[0]
	messageText = result.Message.Content
	var objectIds []string
	objectIds, messageText = ExtractObjectIDs(messageText)

	// TODO Send message for tool call
	if result.FinishReason == openai.FinishReasonToolCalls {
		// TODO Send message and wait for approve
		if messageText != "" {
			msg := &chatobject.Message{
				ChatMessage: &model.ChatMessage{
					Message: &model.ChatMessageMessageContent{
						Text: messageText,
					},
				},
			}

			if msg.Message.Text != "[no answer]" {
				_, err = c.chatService.AddMessage(ctx, nil, c.chatObjectId, msg)
				if err != nil {
					return fmt.Errorf("response in chat: %w", err)
				}
			}
		}

		if c.autoApproveToolUsage {
			toolsCallReq, err := createToolsCallRequest(result.Message.ToolCalls, messages)
			if err != nil {
				return fmt.Errorf("create tools call: %w", err)
			}

			return c.callTools(ctx, toolsCallReq)
		} else {
			toolNames := make([]string, 0, len(result.Message.ToolCalls))
			for _, call := range result.Message.ToolCalls {
				toolNames = append(toolNames, call.Function.Name)
			}

			approvalMessageId, err := c.chatService.AddMessage(ctx, nil, c.chatObjectId, &chatobject.Message{
				ChatMessage: &model.ChatMessage{
					Message: &model.ChatMessageMessageContent{
						Text: fmt.Sprintf("I need to call tools to finish your request: %s", strings.Join(toolNames, " ")),
					},
				},
			})
			if err != nil {
				return fmt.Errorf("response in chat: %w", err)
			}

			err = c.addPendingToolCalls(approvalMessageId, messages, result.Message.ToolCalls)
			if err != nil {
				return fmt.Errorf("add pending tool calls: %w", err)
			}
		}

	} else if result.FinishReason == openai.FinishReasonStop {
		msg := &chatobject.Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: messageText,
				},
			},
		}

		if len(objectIds) > 0 {
			// max 5 objects
			if len(objectIds) > 5 {
				objectIds = objectIds[:5]
			}
			msg.Attachments = make([]*model.ChatMessageAttachment, 0, len(objectIds))
			for _, objectId := range objectIds {
				msg.Attachments = append(msg.Attachments, &model.ChatMessageAttachment{
					Target: objectId,
					Type:   model.ChatMessageAttachment_LINK,
				})
			}
		}

		if msg.Message.Text != "[no answer]" {
			_, err = c.chatService.AddMessage(ctx, nil, c.chatObjectId, msg)
			if err != nil {
				return fmt.Errorf("response in chat: %w", err)
			}
		}
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

func (c *Chatter) AddReaction(messageId string, reactions map[string]bool) {
	c.lock.Lock()
	// _, ok := reactions["✅"]
	// if ok {
	// 	c.approvedMessages[messageId] = true
	// }

	c.approvedMessages[messageId] = true
	c.lock.Unlock()

	c.forceUpdate <- struct{}{}
}

// headerRe matches <<<id1,id2,id3>>>restOfText
var headerRe = regexp.MustCompile(`^<<<([^>]*)>>>(.*)`)

// ExtractObjectIDs pulls any leading <<<…>>> block out of text.
// If present, it returns the IDs slice and the remainder of the text.
// If absent or malformed, it returns nil + the original text.
func ExtractObjectIDs(text string) ([]string, string) {
	m := headerRe.FindStringSubmatch(text)
	if m == nil {
		// no header → nothing to extract
		return nil, text
	}
	// m[1] is the comma‑separated IDs, m[2] is the rest of the message
	idsRaw := m[1]
	rest := m[2]

	// split, but guard against empty
	if idsRaw == "" {
		return nil, strings.TrimSpace(rest)
	}
	ids := strings.Split(idsRaw, ",")
	// optionally trim whitespace around the rest
	return ids, strings.TrimLeft(rest, " \t\r\n")
}
