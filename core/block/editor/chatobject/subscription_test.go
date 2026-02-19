package chatobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSubscription(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	for i := 0; i < 10; i++ {
		inputMessage := givenComplexMessage()
		inputMessage.Message.Text = fmt.Sprintf("text %d", i+1)
		messageId, err := fx.AddMessage(ctx, nil, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)
	}

	resp, err := fx.chatSubscriptionService.SubscribeLastMessages(ctx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId: fx.Id(), SubId: "subId", Limit: 5,
	})
	require.NoError(t, err)
	wantTexts := []string{"text 6", "text 7", "text 8", "text 9", "text 10"}
	for i, msg := range resp.Messages {
		assert.Equal(t, wantTexts[i], msg.Message.Text)
	}

	lastOrderId := resp.Messages[len(resp.Messages)-1].OrderId
	var lastStateId string
	t.Run("add message", func(t *testing.T) {
		fx.events = nil

		messageId, err := fx.AddMessage(ctx, nil, givenComplexMessage())
		require.NoError(t, err)
		require.Len(t, fx.events, 2)

		message, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)

		lastStateId = message.StateId

		wantEvents := []*pb.EventMessage{
			{
				SpaceId: testSpaceId,
				Value: &pb.EventMessageValueOfChatAdd{
					ChatAdd: &pb.EventChatAdd{
						Id:           message.Id,
						OrderId:      message.OrderId,
						AfterOrderId: lastOrderId,
						Message:      message.ChatMessage,
						SubIds:       []string{"subId"},
						Dependencies: nil,
					},
				},
			},
			{
				SpaceId: testSpaceId,
				Value: &pb.EventMessageValueOfChatStateUpdate{
					ChatStateUpdate: &pb.EventChatUpdateState{
						State: &model.ChatState{
							Messages:    &model.ChatStateUnreadState{},
							Mentions:    &model.ChatStateUnreadState{},
							LastStateId: message.StateId,
							Order:       11,
						},
						SubIds: []string{"subId"},
					},
				},
			},
		}
		assert.Equal(t, wantEvents, fx.events)
	})

	t.Run("edit message", func(t *testing.T) {
		fx.events = nil

		edited := givenComplexMessage()
		edited.Message.Text = "edited text"

		// Use index 1 because the message with index 0 is deleted from the subscription state after adding another message due to limit of 5
		err = fx.EditMessage(ctx, resp.Messages[1].Id, edited)
		require.NoError(t, err)
		require.Len(t, fx.events, 1)

		message, err := fx.GetMessageById(ctx, resp.Messages[1].Id)
		require.NoError(t, err)

		wantEvents := []*pb.EventMessage{
			{
				SpaceId: testSpaceId,
				Value: &pb.EventMessageValueOfChatUpdate{
					ChatUpdate: &pb.EventChatUpdate{
						Id:      resp.Messages[1].Id,
						Message: message.ChatMessage,
						SubIds:  []string{"subId"},
					},
				},
			},
		}
		assert.Equal(t, wantEvents, fx.events)
	})

	t.Run("toggle message reaction", func(t *testing.T) {
		fx.events = nil

		// Use index 1 because the message with index 0 is deleted from the subscription state after adding another message due to limit of 5
		added, err := fx.ToggleMessageReaction(ctx, resp.Messages[1].Id, "👍")
		require.NoError(t, err)
		require.Len(t, fx.events, 1)
		assert.True(t, added)

		wantEvents := []*pb.EventMessage{
			{
				SpaceId: testSpaceId,
				Value: &pb.EventMessageValueOfChatUpdateReactions{
					ChatUpdateReactions: &pb.EventChatUpdateReactions{
						Id: resp.Messages[1].Id,
						Reactions: &model.ChatMessageReactions{
							Reactions: map[string]*model.ChatMessageReactionsIdentityList{
								"👍": {
									Ids: []string{testCreator},
								},
								"🥰": {
									Ids: []string{"identity1", "identity2"},
								},
								"🤔": {
									Ids: []string{"identity3"},
								},
							},
						},
						SubIds: []string{"subId"},
					},
				},
			},
		}
		assert.Equal(t, wantEvents, fx.events)
	})

	t.Run("delete message", func(t *testing.T) {
		fx.events = nil

		err = fx.DeleteMessage(ctx, resp.Messages[0].Id)
		require.NoError(t, err)
		require.Len(t, fx.events, 2)

		wantEvents := []*pb.EventMessage{
			{
				SpaceId: testSpaceId,
				Value: &pb.EventMessageValueOfChatDelete{
					ChatDelete: &pb.EventChatDelete{
						Id:     resp.Messages[0].Id,
						SubIds: []string{"subId"},
					},
				},
			},
			{
				SpaceId: testSpaceId,
				Value: &pb.EventMessageValueOfChatStateUpdate{
					ChatStateUpdate: &pb.EventChatUpdateState{
						State: &model.ChatState{
							Messages:    &model.ChatStateUnreadState{},
							Mentions:    &model.ChatStateUnreadState{},
							LastStateId: lastStateId,
							Order:       12,
						},
						SubIds: []string{"subId"},
					},
				},
			},
		}
		assert.Equal(t, wantEvents, fx.events)
	})
}

func TestSubscriptionMessageCounters(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	fx.chatHandler.forceNotRead = true

	subscribeResp, err := fx.chatSubscriptionService.SubscribeLastMessages(ctx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId: fx.Id(), SubId: "subId", Limit: 10,
	})
	require.NoError(t, err)

	assert.Empty(t, subscribeResp.Messages)
	assert.Equal(t, &model.ChatState{
		Messages:    &model.ChatStateUnreadState{},
		Mentions:    &model.ChatStateUnreadState{},
		LastStateId: "",
	}, subscribeResp.ChatState)

	// Add first message
	firstMessageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("first"))
	require.NoError(t, err)
	firstMessage, err := fx.GetMessageById(ctx, firstMessageId)
	require.NoError(t, err)

	wantEvents := []*pb.EventMessage{
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatAdd{
				ChatAdd: &pb.EventChatAdd{
					Id:           firstMessage.Id,
					OrderId:      firstMessage.OrderId,
					AfterOrderId: "",
					Message:      firstMessage.ChatMessage,
					SubIds:       []string{"subId"},
					Dependencies: nil,
				},
			},
		},
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatStateUpdate{
				ChatStateUpdate: &pb.EventChatUpdateState{
					State: &model.ChatState{
						Messages: &model.ChatStateUnreadState{
							Counter:       1,
							OldestOrderId: firstMessage.OrderId,
						},
						Mentions:    &model.ChatStateUnreadState{},
						LastStateId: firstMessage.StateId,
						Order:       1,
					},
					SubIds: []string{"subId"},
				},
			},
		},
	}

	assert.Equal(t, wantEvents, fx.events)
	fx.events = nil

	secondMessageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("second"))
	require.NoError(t, err)

	secondMessage, err := fx.GetMessageById(ctx, secondMessageId)
	require.NoError(t, err)

	wantEvents = []*pb.EventMessage{
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatAdd{
				ChatAdd: &pb.EventChatAdd{
					Id:           secondMessage.Id,
					OrderId:      secondMessage.OrderId,
					AfterOrderId: firstMessage.OrderId,
					Message:      secondMessage.ChatMessage,
					SubIds:       []string{"subId"},
					Dependencies: nil,
				},
			},
		},
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatStateUpdate{
				ChatStateUpdate: &pb.EventChatUpdateState{
					State: &model.ChatState{
						Messages: &model.ChatStateUnreadState{
							Counter:       2,
							OldestOrderId: firstMessage.OrderId,
						},
						Mentions:    &model.ChatStateUnreadState{},
						LastStateId: secondMessage.StateId,
						Order:       2,
					},
					SubIds: []string{"subId"},
				},
			},
		},
	}
	assert.Equal(t, wantEvents, fx.events)

	// Read first message

	fx.events = nil

	_, err = fx.MarkReadMessages(ctx, ReadMessagesRequest{
		AfterOrderId:  "",
		BeforeOrderId: firstMessage.OrderId,
		LastStateId:   secondMessage.StateId,
		CounterType:   chatmodel.CounterTypeMessage,
	})
	require.NoError(t, err)

	wantEvents = []*pb.EventMessage{
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatUpdateMessageReadStatus{
				ChatUpdateMessageReadStatus: &pb.EventChatUpdateMessageReadStatus{
					SubIds: []string{"subId"},
					Ids:    []string{firstMessageId},
					IsRead: true,
				},
			},
		},
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatStateUpdate{
				ChatStateUpdate: &pb.EventChatUpdateState{
					State: &model.ChatState{
						Messages: &model.ChatStateUnreadState{
							Counter:       1,
							OldestOrderId: secondMessage.OrderId,
						},
						Mentions:    &model.ChatStateUnreadState{},
						LastStateId: secondMessage.StateId,
						Order:       4,
					},
					SubIds: []string{"subId"},
				},
			},
		},
	}

	assert.Equal(t, wantEvents, fx.events)
}

func TestSubscriptionMentionCounters(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	fx.chatHandler.forceNotRead = true

	subscribeResp, err := fx.chatSubscriptionService.SubscribeLastMessages(ctx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId: fx.Id(),
		SubId:        "subId",
		Limit:        10,
	})
	require.NoError(t, err)

	assert.Empty(t, subscribeResp.Messages)
	assert.Equal(t, &model.ChatState{
		Messages:    &model.ChatStateUnreadState{},
		Mentions:    &model.ChatStateUnreadState{},
		LastStateId: "",
	}, subscribeResp.ChatState)

	// Add first message
	firstMessageId, err := fx.AddMessage(ctx, nil, givenMessageWithMention("first"))
	require.NoError(t, err)
	firstMessage, err := fx.GetMessageById(ctx, firstMessageId)
	require.NoError(t, err)

	wantEvents := []*pb.EventMessage{
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatAdd{
				ChatAdd: &pb.EventChatAdd{
					Id:           firstMessage.Id,
					OrderId:      firstMessage.OrderId,
					AfterOrderId: "",
					Message:      firstMessage.ChatMessage,
					SubIds:       []string{"subId"},
					Dependencies: nil,
				},
			},
		},
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatStateUpdate{
				ChatStateUpdate: &pb.EventChatUpdateState{
					State: &model.ChatState{
						Messages: &model.ChatStateUnreadState{
							Counter:       1,
							OldestOrderId: firstMessage.OrderId,
						},
						Mentions: &model.ChatStateUnreadState{
							Counter:       1,
							OldestOrderId: firstMessage.OrderId,
						},
						LastStateId: firstMessage.StateId,
						Order:       1,
					},
					SubIds: []string{"subId"},
				},
			},
		},
	}

	assert.Equal(t, wantEvents, fx.events)
	fx.events = nil

	secondMessageId, err := fx.AddMessage(ctx, nil, givenMessageWithMention("second"))
	require.NoError(t, err)

	secondMessage, err := fx.GetMessageById(ctx, secondMessageId)
	require.NoError(t, err)

	wantEvents = []*pb.EventMessage{
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatAdd{
				ChatAdd: &pb.EventChatAdd{
					Id:           secondMessage.Id,
					OrderId:      secondMessage.OrderId,
					AfterOrderId: firstMessage.OrderId,
					Message:      secondMessage.ChatMessage,
					SubIds:       []string{"subId"},
					Dependencies: nil,
				},
			},
		},
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatStateUpdate{
				ChatStateUpdate: &pb.EventChatUpdateState{
					State: &model.ChatState{
						Messages: &model.ChatStateUnreadState{
							Counter:       2,
							OldestOrderId: firstMessage.OrderId,
						},
						Mentions: &model.ChatStateUnreadState{
							Counter:       2,
							OldestOrderId: firstMessage.OrderId,
						},
						LastStateId: secondMessage.StateId,
						Order:       2,
					},
					SubIds: []string{"subId"},
				},
			},
		},
	}
	assert.Equal(t, wantEvents, fx.events)

	// Read first message

	fx.events = nil

	_, err = fx.MarkReadMessages(ctx, ReadMessagesRequest{
		AfterOrderId:  "",
		BeforeOrderId: firstMessage.OrderId,
		LastStateId:   secondMessage.StateId,
		CounterType:   chatmodel.CounterTypeMention,
	})
	require.NoError(t, err)

	wantEvents = []*pb.EventMessage{
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatUpdateMentionReadStatus{
				ChatUpdateMentionReadStatus: &pb.EventChatUpdateMentionReadStatus{
					SubIds: []string{"subId"},
					Ids:    []string{firstMessageId},
					IsRead: true,
				},
			},
		},
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatStateUpdate{
				ChatStateUpdate: &pb.EventChatUpdateState{
					State: &model.ChatState{
						Messages: &model.ChatStateUnreadState{
							Counter:       2,
							OldestOrderId: firstMessage.OrderId,
						},
						Mentions: &model.ChatStateUnreadState{
							Counter:       1,
							OldestOrderId: secondMessage.OrderId,
						},
						LastStateId: secondMessage.StateId,
						Order:       4,
					},
					SubIds: []string{"subId"},
				},
			},
		},
	}

	assert.Equal(t, wantEvents, fx.events)
}

func TestSubscriptionWithDeps(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	_, err := fx.chatSubscriptionService.SubscribeLastMessages(ctx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId:     fx.Id(),
		SubId:            "subId",
		Limit:            10,
		WithDependencies: true,
	})
	require.NoError(t, err)

	myParticipantId := domain.NewParticipantId(testSpaceId, testCreator)

	identityDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:   domain.String(myParticipantId),
		bundle.RelationKeyName: domain.String("John Doe"),
	})
	err = fx.spaceIndex.UpdateObjectDetails(ctx, myParticipantId, identityDetails)
	require.NoError(t, err)

	attachmentDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:   domain.String("fileObjectId1"),
		bundle.RelationKeyName: domain.String("file 1"),
	})
	err = fx.spaceIndex.UpdateObjectDetails(ctx, "fileObjectId1", attachmentDetails)
	require.NoError(t, err)

	inputMessage := givenSimpleMessage("hello!")
	inputMessage.Attachments = []*model.ChatMessageAttachment{
		{
			Target: attachmentDetails.GetString(bundle.RelationKeyId),
			Type:   model.ChatMessageAttachment_FILE,
		},
		{
			Target: "unknown object id",
			Type:   model.ChatMessageAttachment_FILE,
		},
	}

	messageId, err := fx.AddMessage(ctx, nil, inputMessage)
	require.NoError(t, err)

	message, err := fx.GetMessageById(ctx, messageId)
	require.NoError(t, err)

	wantEvents := []*pb.EventMessage{
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatAdd{
				ChatAdd: &pb.EventChatAdd{
					Id:           message.Id,
					OrderId:      message.OrderId,
					AfterOrderId: "",
					Message:      message.ChatMessage,
					SubIds:       []string{"subId"},
					Dependencies: []*types.Struct{
						identityDetails.ToProto(),
						attachmentDetails.ToProto(),
					},
				},
			},
		},
		{
			SpaceId: testSpaceId,
			Value: &pb.EventMessageValueOfChatStateUpdate{
				ChatStateUpdate: &pb.EventChatUpdateState{
					State: &model.ChatState{
						Messages:    &model.ChatStateUnreadState{},
						Mentions:    &model.ChatStateUnreadState{},
						LastStateId: message.StateId,
						Order:       1,
					},
					SubIds: []string{"subId"},
				},
			},
		},
	}
	assert.Equal(t, wantEvents, fx.events)
}
