package main

import (
	"context"
	"fmt"
	"os"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/anytype-heart/cmd/assistant/runtime"
	"github.com/anyproto/anytype-heart/core"
	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apiservice "github.com/anyproto/anytype-heart/core/api/service"
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

	for {
		msg, err := app.eventQueue.WaitOne(ctx)
		if err != nil {
			return fmt.Errorf("wait event: %w", err)
		}
		chatAddEv := msg.GetChatAdd()
		if chatAddEv != nil && chatAddEv.Message.Creator != app.config.AccountId {

			chatId, currentSpaceId, err := chatService.FindChatByMessageId(ctx, chatAddEv.Id)
			if err != nil {
				fmt.Printf("findChatByMessageId err: %s\n", err.Error())
				continue
			}

			// Create middleware wrapper for API service
			mw := core.NewWithApplicationService(app.appService)
			crossSpaceSub := getService[apicore.CrossSpaceSubscriptionService](app)
			svc := apiservice.NewService(mw, app.account.Info.GatewayUrl, app.account.Info.TechSpaceId, crossSpaceSub)

			// Initialize caches to populate properties, types, and tags
			if err := svc.InitializeAllCaches(); err != nil {
				fmt.Printf("InitializeAllCaches err: %s\n", err.Error())
				continue
			}

			reply, trace, err := runtime.HandleChatMsg(ctx, runtime.HandleChatMsgParams{
				ChatAddEv:      chatAddEv,
				OpenAIKey:      app.config.OpenAIKey,
				ClaudeKey:      app.config.ClaudeKey,
				ApiService:     svc,
				CurrentSpaceId: currentSpaceId,
				MainProgram:    "assistant@v2", // TODO: make configurable
				ApiBaseUrl:     app.config.GetApiBaseUrl(),
			})
			if trace != nil {
				fmt.Printf("-- trace:\n%s\n", runtime.TraceToJSON(trace))
			}
			if err != nil {
				fmt.Printf("handleChatMsg err: %s\n", err.Error())
				continue
			}

			todoCtx := session.NewContext()
			fmt.Printf("-- replying to chat:  %s\n", chatId)
			_, err = chatService.AddMessage(ctx, todoCtx, chatId, &chatmodel.Message{
				ChatMessage: &model.ChatMessage{
					Message: &model.ChatMessageMessageContent{
						Text: reply,
					},
				},
			})
			if err != nil {
				fmt.Printf("response in chat: %s", err.Error())
				continue
			}

		}
	}

}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
