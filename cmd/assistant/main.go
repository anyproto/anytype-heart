package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"anyproto/anytype-agent-runtime/anyruntime"
	agentrt "anyproto/anytype-agent-runtime/runtime"
	"anyproto/anytype-agent-runtime/runtime/hostfn"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

var log = logging.Logger("assistant").Desugar()

type DbProvider interface {
	GetCommonDb() anystore.DB
}

const mainProgram = "private:init_agent@v1"

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("pass datadir and name as argument")
	}
	dataDir := os.Args[1]

	ctx := context.Background()
	app, err := createAccountAndStartApp(ctx, dataDir, os.Args[2], pb.RpcObjectImportUseCaseRequest_CHAT_SPACE)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}

	err = app.config.Validate()
	if err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	dbProvider := getService[DbProvider](app)
	db := dbProvider.GetCommonDb()
	handledMessages, err := keyvaluestore.New(db, "assistant/handledMessages", func(t string) ([]byte, error) {
		return []byte(t), nil
	}, func(bytes []byte) (string, error) {
		return string(bytes), nil
	})
	if err != nil {
		return fmt.Errorf("init handled messages store: %w", err)
	}
	_ = handledMessages

	chatService := getService[chats.Service](app)
	// hacky way to subscribe to all chats
	_, err = chatService.SubscribeToMessagePreviews(ctx, "ai_assistant_chat")
	if err != nil {
		return fmt.Errorf("SubscribeToMessagePreviews: %w", err)
	}

	apiBaseUrl := app.config.GetApiBaseUrl()
	privateSpaceId := os.Getenv("ANYTYPE_PRIVATE_SPACE_ID")
	claudeKey := os.Getenv("CLAUDE_API_KEY")

	fmt.Printf("API base URL: %s\n", apiBaseUrl)
	fmt.Printf("Private space ID: %s\n", privateSpaceId)

	for {
		msg, err := app.eventQueue.WaitOne(ctx)
		if err != nil {
			return fmt.Errorf("wait event: %w", err)
		}
		chatAddEv := msg.GetChatAdd()
		if chatAddEv == nil || chatAddEv.Message.Creator == app.config.AccountId {
			continue
		}

		chatId, currentSpaceId, err := chatService.FindChatByMessageId(ctx, chatAddEv.Id)
		if err != nil {
			fmt.Printf("findChatByMessageId err: %s\n", err.Error())
			continue
		}

		fmt.Printf("-- message in chat %s (space %s)\n", chatId, currentSpaceId)

		// Create a new runtime for each message
		rt, err := createMessageRuntime(ctx, chatService, chatId, currentSpaceId, apiBaseUrl, privateSpaceId, claudeKey)
		if err != nil {
			fmt.Printf("create runtime err: %s\n", err.Error())
			continue
		}

		quotedText, _ := json.Marshal(chatAddEv.Message.Message.Text)
		quotedIdentity, _ := json.Marshal(chatAddEv.Message.Creator)
		quotedSpaceId, _ := json.Marshal(currentSpaceId)
		quotedApiBaseUrl, _ := json.Marshal(apiBaseUrl)

		wrapperSource := fmt.Sprintf(`import { main as entryMain } from %q;
export function main() {
  return entryMain({ text: %s, identity: %s, spaceId: %s, apiBaseUrl: %s, verbose: false });
}`, mainProgram, string(quotedText), string(quotedIdentity), string(quotedSpaceId), string(quotedApiBaseUrl))

		res, err := rt.EvalToString("__wrapper__", wrapperSource, nil)
		if res != nil {
			anyruntime.PrintTrace(res.Trace)
			if res.Error != "" {
				fmt.Printf("-- runtime error: %s\n", res.Error)
			}
		}
		if err != nil {
			fmt.Printf("runtime eval err: %s\n", err.Error())
			continue
		}

		// If the program returned a string (old-style), send it as a reply too
		if res != nil && res.Result != nil {
			if reply, ok := res.Result.(string); ok && reply != "" {
				todoCtx := session.NewContext()
				fmt.Printf("-- replying to chat: %s\n", chatId)
				_, err = chatService.AddMessage(ctx, todoCtx, chatId, &chatmodel.Message{
					ChatMessage: &model.ChatMessage{
						Message: &model.ChatMessageMessageContent{
							Text: reply,
						},
					},
				})
				if err != nil {
					fmt.Printf("response in chat err: %s\n", err.Error())
					continue
				}
			}
		}
	}
}

// createMessageRuntime creates a new anytype-agent-runtime for a single message,
// following the same pattern as anyruntime.CreateAnytypeJSRuntime but with:
// - chatReply wired to send messages to chat via chatService
// - per-message space ID
// - API URL pointing to our local API server
func createMessageRuntime(ctx context.Context, chatService chats.Service, chatId string, spaceId string, apiBaseUrl string, privateSpaceId string, claudeKey string) (agentrt.Runtime, error) {
	rt, err := agentrt.NewSobekRuntime()
	if err != nil {
		return nil, fmt.Errorf("create sobek runtime: %w", err)
	}

	rt.SetEffectResolver("fetch", hostfn.Fetch)
	rt.SetEffectResolver("sleep", hostfn.Sleep)

	// Wire chatReply to actually send messages to the chat
	rt.SetEffectResolver("chatReply", newChatReplyEffect(ctx, chatService, chatId))

	rt.EnableConsole()
	rt.EnableJSEval()

	rt.SetGlobal("env", map[string]any{
		"ANYTYPE_API_URL":          apiBaseUrl,
		"ANYTYPE_API_KEY":          "", // auth disabled via ANYTYPE_API_DISABLE_AUTH=1
		"ANYTYPE_SPACE_ID":         spaceId,
		"ANYTYPE_PRIVATE_SPACE_ID": privateSpaceId,
		"CLAUDE_API_KEY":           claudeKey,
	})

	loader := anyruntime.NewAnytypeLoader(anyruntime.AnytypeLoaderConfig{
		BaseURL:        apiBaseUrl,
		SpaceID:        spaceId,
		PrivateSpaceID: privateSpaceId,
		APIKey:         "", // auth disabled
	})
	rt.SetModuleResolver(loader)

	return rt, nil
}

// newChatReplyEffect returns a chatReply effect handler that sends messages
// to the given chat via chatService.AddMessage.
func newChatReplyEffect(ctx context.Context, chatService chats.Service, chatId string) func(tr *agentrt.TraceRecord, args ...any) any {
	return func(tr *agentrt.TraceRecord, args ...any) any {
		if len(args) == 0 {
			return nil
		}
		msg := fmt.Sprintf("%v", args[0])
		tr.SetInput(msg)

		fmt.Printf("-- chatReply to %s: %s\n", chatId, msg)

		todoCtx := session.NewContext()
		_, err := chatService.AddMessage(ctx, todoCtx, chatId, &chatmodel.Message{
			ChatMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: msg,
				},
			},
		})
		if err != nil {
			fmt.Printf("chatReply err: %s\n", err.Error())
			return map[string]any{"error": err.Error()}
		}
		return nil
	}
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
