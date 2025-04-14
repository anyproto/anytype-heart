package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anyproto/anytype-heart/core/acl"
	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/cheggaaa/mb/v3"
	"github.com/sashabaranov/go-openai"
)

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("pass datadir as argument")
	}
	dataDir := os.Args[1]

	ctx := context.Background()
	app, err := createAccountAndStartApp(ctx, dataDir, pb.RpcObjectImportUseCaseRequest_EMPTY)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}

	err = app.config.Validate()
	if err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}


	aclSerivce := getService[acl.AclService](app)

	invite, err := viewInvite(ctx, aclSerivce, &pb.RpcSpaceInviteViewRequest{
		InviteCid:     app.config.InviteCid,
		InviteFileKey: app.config.InviteKey,
	})
	if err != nil {
		return fmt.Errorf("view invite: %w", err)
	}

	spaceViewSub := mb.New[*pb.EventMessage](0)
	subscriptionService := getService[subscription.Service](app)
	subResp, err := subscriptionService.Search(subscription.SubscribeRequest{
		SpaceId: app.account.Info.TechSpaceId,
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceLocalStatus.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyTargetSpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(invite.SpaceId),
			},
		},
		Internal:      true,
		InternalQueue: spaceViewSub,
	})
	if err != nil {
		return fmt.Errorf("subscribe to space view: %w", err)
	}

	// m := &jsonpb.Marshaler{}

	if len(subResp.Records) > 0 {
		// TODO revive
		// fmt.Println(subResp.Records[0])
		// for {
		// 	msg, err := spaceViewSub.WaitOne(ctx)
		// 	if err != nil {
		// 		return fmt.Errorf("wait one: %w", err)
		// 	}

		// 	m.Marshal(os.Stdout, msg)
		// }
	} else {
		err = joinSpace(ctx, aclSerivce, &pb.RpcSpaceJoinRequest{
			NetworkId:     "N83gJpVd9MuNRZAuJLZ7LiMntTThhPc6DtzWWVjb1M3PouVU",
			SpaceId:       invite.SpaceId,
			InviteCid:     "bafybeidc37n3oadwu267rjkl6vocpfggmpibbqdor2aqp4qaovemqekwry",
			InviteFileKey: "CEhksAtZnzAmNKeMcR9ETiYb8P8ngKBqPQKczio3a1QL",
		})
		if err != nil {
			return fmt.Errorf("join space: %w", err)
		}
	}

	time.Sleep(1 * time.Second)

	chatObjectId := "bafyreiefybfdqajqwa6hrkl4wa3pm2knr3s7rto3m67chybl5svyffhvui"

	chatService := getService[chats.Service](app)
	_, err = chatService.SubscribeLastMessages(ctx, chatObjectId, 0, "test")
	if err != nil {
		return fmt.Errorf("subscribe to chat: %w", err)
	}


	openAiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	for {
		msg, err := app.eventQueue.WaitOne(ctx)
		if err != nil {
			return fmt.Errorf("wait event: %w", err)
		}
		chatAddEv := msg.GetChatAdd()
		if chatAddEv != nil {
			if chatAddEv.Message.Creator != app.account.Id {
				fmt.Println("CAPTURE:", chatAddEv.Message.Message.Text)

				compResp, err := openAiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
					Model: openai.GPT4oMini,
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleUser,
							Content: chatAddEv.Message.Message.Text,
						},
					},
				})
				if err != nil {
					return fmt.Errorf("create chat completion: %w", err)
				}

				completion := compResp.Choices[0].Message.Content

				_, err  = chatService.AddMessage(ctx, nil, chatObjectId, &chatobject.Message{
					ChatMessage: &model.ChatMessage{
						Message: &model.ChatMessageMessageContent{
							Text: completion,
						},
					},
				})
				if err != nil {
					return fmt.Errorf("response in chat: %w", err)
				}
			}
		}
	}

	// TODO Check that space view is not exist -> join space
	// TODO If invite is sent, wait for space to load
	// TODO When it loaded -> write a message to chat :)

	return nil
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
