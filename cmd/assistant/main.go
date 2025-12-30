package main

import (
	"context"
	"fmt"
	"os"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
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
	chatId := "bafyreihc5vhheckztsq35kq3dny4eo7v2lk5o36pgfniuv6eapwny3zkge"
	lastMessagesResp, err := chatService.SubscribeLastMessages(ctx, chatId, 20, "test")
	if err != nil {
		return fmt.Errorf("subscribe to chat: %w", err)
	}
	for _, msg := range lastMessagesResp.Messages {
		fmt.Printf("-- got msgs: %s\n", msg.ChatMessage.Message.Text)
	}
	for {
		msg, err := app.eventQueue.WaitOne(ctx)
		if err != nil {
			return fmt.Errorf("wait event: %w", err)
		}
		chatAddEv := msg.GetChatAdd()
		if chatAddEv != nil {
			fmt.Printf("-- chat msg: %s\n", chatAddEv.Message)
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
