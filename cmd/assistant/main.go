package main

import (
	"context"
	"fmt"
	"os"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
	"github.com/buke/quickjs-go"
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

			reply, err := handleChatMsg(chatAddEv)
			if err != nil {
				fmt.Printf("handleChatMsg err: %s\n", err.Error())
				continue
			}
			chatId, err := chatService.FindChatByMessageId(ctx, chatAddEv.Id)
			if err != nil {
				fmt.Printf("findChatByMessageId err: %s\n", err.Error())
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

type jsMessage struct {
	Identity string `js:"identity"`
	Text     string `js:"text"`
}

func handleChatMsg(chatAddEv *pb.EventChatAdd) (reply string, err error) {

	// Create a new runtime
	rt := quickjs.NewRuntime()
	defer rt.Close()

	// Create a new context
	ctx := rt.NewContext()
	defer ctx.Close()

	ret := ctx.Eval(`
function main(args) {
  const reply = "you've said: " + args.message.text
  return { result: reply }
}
`)
	defer ret.Free()
	if ret.IsException() {
		err = ctx.Exception()
		return
	}

	jsMessage := jsMessage{
		Text:     chatAddEv.Message.Message.Text,
		Identity: chatAddEv.Message.Creator,
	}
	jsMessageVal, err := ctx.Marshal(jsMessage)
	if err != nil {
		return
	}
	defer jsMessageVal.Free()

	result := ctx.Globals().Call("main", jsMessageVal)
	defer result.Free()

	reply = ret.ToString()
	return
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
