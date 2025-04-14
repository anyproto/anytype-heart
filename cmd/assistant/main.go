package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anyproto/anytype-heart/core/acl"
	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
	"github.com/cheggaaa/mb/v3"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

var log = logging.Logger("assistant").Desugar()

const contextWindow = 20

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("pass datadir and name as argument")
	}
	dataDir := os.Args[1]

	ctx := context.Background()
	app, err := createAccountAndStartApp(ctx, dataDir, os.Args[2], pb.RpcObjectImportUseCaseRequest_EMPTY)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}

	err = app.config.Validate()
	if err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	localStoreDb := getService[anystoreprovider.Provider](app).GetCommonDb()
	handledMessages, err := keyvaluestore.New(localStoreDb, "handledMessages", keyvaluestore.StringMarshal, keyvaluestore.StringUnmarshal)
	if err != nil {
		return fmt.Errorf("init handled messages store: %w", err)
	}
	_ = handledMessages

	if app.config.SpaceId == "" {
		aclSerivce := getService[acl.AclService](app)
		invite, err := viewInvite(ctx, aclSerivce, &pb.RpcSpaceInviteViewRequest{
			InviteCid:     app.config.InviteCid,
			InviteFileKey: app.config.InviteKey,
		})
		if err != nil {
			return fmt.Errorf("view invite: %w", err)
		}
		app.config.SpaceId = invite.SpaceId

		err = writeConfig(filepath.Join(dataDir, "config.json"), app.config)
		if err != nil {
			return fmt.Errorf("update config: %w", err)
		}
	}

	err = app.waitSpaceToLoad(ctx)
	if err != nil {
		return fmt.Errorf("wait space to load: %w", err)
	}

	chatObjectId, err := app.waitForChatToLoad(ctx)
	if err != nil {
		return fmt.Errorf("wait for chat to load: %w", err)
	}

	chatService := getService[chats.Service](app)

	openAiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	chatter := &Chatter{
		limit:        contextWindow,
		myIdentity:   app.account.Id,
		chatObjectId: chatObjectId,
		systemPrompt: app.config.SystemPrompt,
		chatService:  chatService,
		client:       openAiClient,
		store:        handledMessages,
		maxRequests:  100,
	}

	go func() {
		chatter.Run(context.Background())
	}()

	lastMessagesResp, err := chatService.SubscribeLastMessages(ctx, chatObjectId, contextWindow, "test")
	if err != nil {
		return fmt.Errorf("subscribe to chat: %w", err)
	}

	messages := make([]*model.ChatMessage, 0, len(lastMessagesResp.Messages))
	for _, msg := range lastMessagesResp.Messages {
		messages = append(messages, msg.ChatMessage)
	}

	chatter.InitWith(messages)

	for {
		msg, err := app.eventQueue.WaitOne(ctx)
		if err != nil {
			return fmt.Errorf("wait event: %w", err)
		}
		chatAddEv := msg.GetChatAdd()
		if chatAddEv != nil {
			chatter.Add(chatAddEv.Message)
		}
	}

	return nil
}

func (app *testApplication) waitSpaceToLoad(ctx context.Context) error {
	doneCh := make(chan struct{})

	spaceViewSub := mb.New[*pb.EventMessage](0)
	subscriptionService := getService[subscription.Service](app)
	subResp, err := subscriptionService.Search(subscription.SubscribeRequest{
		SpaceId: app.account.Info.TechSpaceId,
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceLocalStatus.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyTargetSpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(app.config.SpaceId),
			},
		},
		Internal:      true,
		InternalQueue: spaceViewSub,
	})
	if err != nil {
		return fmt.Errorf("subscribe to space view: %w", err)
	}

	waitCtx, waitCtxCancel := context.WithCancel(context.Background())
	defer waitCtxCancel()
	go func() {
		defer func() {
			close(doneCh)
		}()

		for {
			msg, err := spaceViewSub.WaitOne(waitCtx)
			if err != nil {
				log.Error("wait space", zap.Error(err))
				return
			}

			var details *domain.Details
			if ev := msg.GetObjectDetailsSet(); ev != nil {
				details = domain.NewDetailsFromProto(ev.Details)
			}
			if ev := msg.GetObjectDetailsAmend(); ev != nil {
				details = domain.NewDetails()
				for _, kv := range ev.Details {
					details.SetProtoValue(domain.RelationKey(kv.Key), kv.Value)
				}
			}

			if details != nil {
				if details.GetInt64(bundle.RelationKeySpaceLocalStatus) == int64(model.SpaceStatus_Ok) {
					return
				}
			}
		}
	}()

	if len(subResp.Records) == 0 {
		aclSerivce := getService[acl.AclService](app)
		err = joinSpace(ctx, aclSerivce, &pb.RpcSpaceJoinRequest{
			NetworkId:     "N83gJpVd9MuNRZAuJLZ7LiMntTThhPc6DtzWWVjb1M3PouVU",
			SpaceId:       app.config.SpaceId,
			InviteCid:     app.config.InviteCid,
			InviteFileKey: app.config.InviteKey,
		})
		if err != nil {
			return fmt.Errorf("join space: %w", err)
		}
	} else {
		details := subResp.Records[0]
		if details.GetInt64(bundle.RelationKeySpaceLocalStatus) == int64(model.SpaceStatus_Ok) {
			waitCtxCancel()
		}
	}

	<-doneCh

	log.Warn("space is loaded")
	return subscriptionService.Unsubscribe(subResp.SubId)
}

func (app *testApplication) waitForChatToLoad(ctx context.Context) (string, error) {
	defer func() {
		log.Warn("chat is loaded")
	}()
	chatsSub := mb.New[*pb.EventMessage](0)
	subscriptionService := getService[subscription.Service](app)
	subResp, err := subscriptionService.Search(subscription.SubscribeRequest{
		SpaceId: app.config.SpaceId,
		Keys:    []string{bundle.RelationKeyId.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_chatDerived),
			},
		},
		Internal:      true,
		InternalQueue: chatsSub,
	})
	if err != nil {
		return "", fmt.Errorf("subscribe to chats: %w", err)
	}
	defer func() {
		err = subscriptionService.Unsubscribe(subResp.SubId)
		if err != nil {
			log.Error("unsubscribe from chats", zap.Error(err))
		}
	}()

	if len(subResp.Records) > 0 {
		return subResp.Records[0].GetString(bundle.RelationKeyId), nil
	} else {
		for {
			msg, err := chatsSub.WaitOne(ctx)
			if err != nil {
				return "", fmt.Errorf("wait: %w", err)
			}
			log.Warn("wait for chat: handling", zap.Any("event", msg))
			if ev := msg.GetSubscriptionAdd(); ev != nil {
				return ev.Id, nil
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
