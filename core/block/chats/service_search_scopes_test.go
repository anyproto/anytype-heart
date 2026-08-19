package chats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Multi-chat search scopes (GO-7449): empty chatId = all chats in spaceId,
// empty spaceId too = all chats in all spaces. See docs/fts/SpecChatSearchScopes.md.

func chatRecord(chatId, spaceId string) *domain.Details {
	d := domain.NewDetails()
	d.SetString(bundle.RelationKeyId, chatId)
	d.SetString(bundle.RelationKeySpaceId, spaceId)
	return d
}

func fakeRepoWithMessages(msgs ...*model.ChatMessage) chatrepository.Repository {
	messages := make([]*chatmodel.Message, 0, len(msgs))
	for _, m := range msgs {
		messages = append(messages, &chatmodel.Message{ChatMessage: m})
	}
	return &fakeChatRepository{messagesByIds: messages}
}

func TestService_SearchAllChatsInSpace(t *testing.T) {
	spaceId := "space1"

	t.Run("hydrates known chats, drops object docs and unknown chats", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				chatRecord("chat1", spaceId),
				chatRecord("chat2", spaceId),
			},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().SearchChat(spaceId, "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
			{Score: 3.0, ID: domain.NewObjectPathWithMessage("chat1", "msg1").String()},
			{Score: 5.0, ID: domain.NewObjectPathWithMessage("chat2", "msg2").String()},
			// an "m"-token false positive: a block doc, not a message doc
			{Score: 4.0, ID: domain.NewObjectPathWithBlock("obj1", "m").String()},
			// a chat the registry doesn't know (deleted / not loaded): must be
			// skipped without hydration — the repos map would fail on it
			{Score: 6.0, ID: domain.NewObjectPathWithMessage("chatGhost", "msgG").String()},
		}, nil)

		fx.chatRepoService.repos = map[string]chatrepository.Repository{
			"chat1": fakeRepoWithMessages(&model.ChatMessage{Id: "msg1"}),
			"chat2": fakeRepoWithMessages(&model.ChatMessage{Id: "msg2"}),
		}

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			FullText: "query",
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 2)
		// default sort is score desc, interleaved across chats
		assert.Equal(t, "msg2", results[0].MessageId)
		assert.Equal(t, "chat2", results[0].ChatId)
		assert.Equal(t, "msg1", results[1].MessageId)
		assert.Equal(t, "chat1", results[1].ChatId)
		for _, result := range results {
			assert.Equal(t, spaceId, result.SpaceId)
		}
	})

	t.Run("space scope excludes registry chats of other spaces", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				chatRecord("chat1", spaceId),
				chatRecord("chatOther", "space2"),
			},
		}, nil).Maybe()

		// even if FT returned another space's message (it should not thanks to
		// the space clause), the registry snapshot drops it
		fx.ftSearch.EXPECT().SearchChat(spaceId, "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
			{Score: 2.0, ID: domain.NewObjectPathWithMessage("chat1", "msg1").String()},
			{Score: 9.0, ID: domain.NewObjectPathWithMessage("chatOther", "msgOther").String()},
		}, nil)

		fx.chatRepoService.repos = map[string]chatrepository.Repository{
			"chat1": fakeRepoWithMessages(&model.ChatMessage{Id: "msg1"}),
		}

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			FullText: "query",
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "msg1", results[0].MessageId)
	})

	t.Run("created_at sort across chats", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				chatRecord("chat1", spaceId),
				chatRecord("chat2", spaceId),
			},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().SearchChat(spaceId, "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
			{Score: 5.0, ID: domain.NewObjectPathWithMessage("chat1", "msgOld").String()},
			{Score: 1.0, ID: domain.NewObjectPathWithMessage("chat2", "msgNew").String()},
		}, nil)

		fx.chatRepoService.repos = map[string]chatrepository.Repository{
			"chat1": fakeRepoWithMessages(&model.ChatMessage{Id: "msgOld", CreatedAt: 100}),
			"chat2": fakeRepoWithMessages(&model.ChatMessage{Id: "msgNew", CreatedAt: 200}),
		}

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			FullText: "query",
			Sorts: []*model.SearchMessageSort{
				{Key: model.SearchMessageSort_CREATED_AT, Type: model.SearchMessageSort_Desc},
			},
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "msgNew", results[0].MessageId)
		assert.Equal(t, "msgOld", results[1].MessageId)
	})

	t.Run("equal scores order deterministically by chatId then messageId", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				chatRecord("chat1", spaceId),
				chatRecord("chat2", spaceId),
			},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().SearchChat(spaceId, "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
			{Score: 1.0, ID: domain.NewObjectPathWithMessage("chat2", "msgC").String()},
			{Score: 1.0, ID: domain.NewObjectPathWithMessage("chat1", "msgB").String()},
			{Score: 1.0, ID: domain.NewObjectPathWithMessage("chat1", "msgA").String()},
		}, nil)

		fx.chatRepoService.repos = map[string]chatrepository.Repository{
			"chat1": fakeRepoWithMessages(&model.ChatMessage{Id: "msgB"}, &model.ChatMessage{Id: "msgA"}),
			"chat2": fakeRepoWithMessages(&model.ChatMessage{Id: "msgC"}),
		}

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			FullText: "query",
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 3)
		got := []string{results[0].MessageId, results[1].MessageId, results[2].MessageId}
		assert.Equal(t, []string{"msgA", "msgB", "msgC"}, got)
	})
}

func TestService_SearchAllSpaces(t *testing.T) {
	// given
	fx := newFixture(t)
	ctx := context.Background()

	fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
		Records: []*domain.Details{
			chatRecord("chat1", "space1"),
			chatRecord("chat2", "space2"),
		},
	}, nil).Maybe()

	fx.ftSearch.EXPECT().SearchChat("", "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
		{Score: 2.0, ID: domain.NewObjectPathWithMessage("chat1", "msg1").String()},
		{Score: 1.0, ID: domain.NewObjectPathWithMessage("chat2", "msg2").String()},
	}, nil)

	fx.chatRepoService.repos = map[string]chatrepository.Repository{
		"chat1": fakeRepoWithMessages(&model.ChatMessage{Id: "msg1"}),
		"chat2": fakeRepoWithMessages(&model.ChatMessage{Id: "msg2"}),
	}

	fx.start(t)

	// when
	results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
		FullText: "query",
	})

	// then
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "space1", results[0].SpaceId)
	assert.Equal(t, "chat1", results[0].ChatId)
	assert.Equal(t, "space2", results[1].SpaceId)
	assert.Equal(t, "chat2", results[1].ChatId)
}
