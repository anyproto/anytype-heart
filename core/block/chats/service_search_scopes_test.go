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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Multi-chat search scopes (GO-7449): empty chatId = all chats in spaceId,
// empty spaceId too = all chats in all spaces. See docs/fts/SpecChatSearchScopes.md.

// addChatObject makes the object index know the chat as live — the gate
// multi-chat scopes hydrate through (isLiveChat)
func addChatObject(t *testing.T, fx *fixture, chatId, spaceId string, layout model.ObjectTypeLayout) {
	fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String(chatId),
			bundle.RelationKeySpaceId:        domain.String(spaceId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(layout)),
		},
	})
}

func fakeRepoWithMessages(msgs ...*model.ChatMessage) chatrepository.Repository {
	messages := make([]*chatmodel.Message, 0, len(msgs))
	for _, m := range msgs {
		messages = append(messages, &chatmodel.Message{ChatMessage: m})
	}
	return &fakeChatRepository{messagesByIds: messages, lastMessages: messages}
}

func TestService_SearchAllChatsInSpace(t *testing.T) {
	spaceId := "space1"

	t.Run("hydrates known chats, drops object docs and unknown chats", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()
		addChatObject(t, fx, "chat1", spaceId, model.ObjectType_chatDerived)
		addChatObject(t, fx, "chat2", spaceId, model.ObjectType_chatDerived)

		fx.ftSearch.EXPECT().SearchChat(spaceId, "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
			{Score: 3.0, ID: domain.NewObjectPathWithMessage("chat1", "msg1").String()},
			{Score: 5.0, ID: domain.NewObjectPathWithMessage("chat2", "msg2").String()},
			// an "m"-token false positive: a block doc, not a message doc
			{Score: 4.0, ID: domain.NewObjectPathWithBlock("obj1", "m").String()},
			// a chat the object index doesn't know (deleted / stale FT doc):
			// must be skipped without hydration — the repos map would fail on it
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
			Records: []*domain.Details{},
		}, nil).Maybe()
		addChatObject(t, fx, "chat1", spaceId, model.ObjectType_chatDerived)
		addChatObject(t, fx, "chatOther", "space2", model.ObjectType_chatDerived)

		// even if FT returned another space's message (it should not thanks to
		// the space clause), the liveness gate checks the requested space's
		// index and drops it
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
			Records: []*domain.Details{},
		}, nil).Maybe()
		addChatObject(t, fx, "chat1", spaceId, model.ObjectType_chatDerived)
		addChatObject(t, fx, "chat2", spaceId, model.ObjectType_chatDerived)

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
			Records: []*domain.Details{},
		}, nil).Maybe()
		addChatObject(t, fx, "chat1", spaceId, model.ObjectType_chatDerived)
		addChatObject(t, fx, "chat2", spaceId, model.ObjectType_chatDerived)

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

func TestService_SearchSpaceScopeIncludesObjectChats(t *testing.T) {
	// GO-7449 review F1: message docs are FT-indexed for both chatDerived
	// (space chats) and discussion (object chats); the liveness gate must
	// admit both layouts
	spaceId := "space1"

	// given
	fx := newFixture(t)
	ctx := context.Background()

	fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
		Records: []*domain.Details{},
	}, nil).Maybe()
	addChatObject(t, fx, "spaceChat", spaceId, model.ObjectType_chatDerived)
	addChatObject(t, fx, "objectChat", spaceId, model.ObjectType_discussion)
	// a plain object caught by a stale/foreign FT doc must stay excluded
	addChatObject(t, fx, "notAChat", spaceId, model.ObjectType_basic)

	fx.ftSearch.EXPECT().SearchChat(spaceId, "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
		{Score: 2.0, ID: domain.NewObjectPathWithMessage("spaceChat", "msg1").String()},
		{Score: 1.0, ID: domain.NewObjectPathWithMessage("objectChat", "msg2").String()},
		{Score: 3.0, ID: domain.NewObjectPathWithMessage("notAChat", "msg3").String()},
	}, nil)

	fx.chatRepoService.repos = map[string]chatrepository.Repository{
		"spaceChat":  fakeRepoWithMessages(&model.ChatMessage{Id: "msg1"}),
		"objectChat": fakeRepoWithMessages(&model.ChatMessage{Id: "msg2"}),
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
	assert.Equal(t, "spaceChat", results[0].ChatId)
	assert.Equal(t, "objectChat", results[1].ChatId)
}

func TestService_SearchSurvivesBrokenChat(t *testing.T) {
	// GO-7449 review F2: one chat's hydration failure (space mid-deletion,
	// broken store) must not abort the whole multi-chat search
	spaceId := "space1"

	// given
	fx := newFixture(t)
	ctx := context.Background()

	fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
		Records: []*domain.Details{},
	}, nil).Maybe()
	addChatObject(t, fx, "chatBroken", spaceId, model.ObjectType_chatDerived)
	addChatObject(t, fx, "chatOk", spaceId, model.ObjectType_chatDerived)

	fx.ftSearch.EXPECT().SearchChat(spaceId, "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
		{Score: 2.0, ID: domain.NewObjectPathWithMessage("chatBroken", "msgB").String()},
		{Score: 1.0, ID: domain.NewObjectPathWithMessage("chatOk", "msgOk").String()},
	}, nil)

	// chatBroken has no repository (Repository returns an error for it)
	fx.chatRepoService.repos = map[string]chatrepository.Repository{
		"chatOk": fakeRepoWithMessages(&model.ChatMessage{Id: "msgOk"}),
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
	assert.Equal(t, "msgOk", results[0].MessageId)
}

func TestService_BrowseLastMessages(t *testing.T) {
	// empty fullText = browse: latest messages in scope from the stores,
	// default CREATED_AT desc, no FT query
	spaceId := "space1"

	t.Run("space scope merges chats newest-first", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()
		addChatObject(t, fx, "chat1", spaceId, model.ObjectType_chatDerived)
		addChatObject(t, fx, "objectChat", spaceId, model.ObjectType_discussion)
		// a non-chat object must not be browsed
		addChatObject(t, fx, "notAChat", spaceId, model.ObjectType_basic)

		fx.chatRepoService.repos = map[string]chatrepository.Repository{
			"chat1": fakeRepoWithMessages(
				&model.ChatMessage{Id: "msg3", CreatedAt: 300},
				&model.ChatMessage{Id: "msg1", CreatedAt: 100},
			),
			"objectChat": fakeRepoWithMessages(
				&model.ChatMessage{Id: "msg2", CreatedAt: 200},
			),
		}

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId: spaceId,
			Sorts: []*model.SearchMessageSort{
				{Key: model.SearchMessageSort_CREATED_AT, Type: model.SearchMessageSort_Desc},
			},
			Limit: 2,
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "msg3", results[0].MessageId)
		assert.Equal(t, "chat1", results[0].ChatId)
		assert.Equal(t, "msg2", results[1].MessageId)
		assert.Equal(t, "objectChat", results[1].ChatId)
		for _, r := range results {
			assert.Equal(t, spaceId, r.SpaceId)
		}
	})

	t.Run("vault scope browses across spaces with attribution", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()
		addChatObject(t, fx, "chat1", "space1", model.ObjectType_chatDerived)
		addChatObject(t, fx, "chat2", "space2", model.ObjectType_chatDerived)

		fx.chatRepoService.repos = map[string]chatrepository.Repository{
			"chat1": fakeRepoWithMessages(&model.ChatMessage{Id: "msg1", CreatedAt: 100}),
			"chat2": fakeRepoWithMessages(&model.ChatMessage{Id: "msg2", CreatedAt: 200}),
		}

		fx.start(t)

		// when: no sorts — browse defaults to CREATED_AT desc
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{})

		// then
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "msg2", results[0].MessageId)
		assert.Equal(t, "space2", results[0].SpaceId)
		assert.Equal(t, "msg1", results[1].MessageId)
		assert.Equal(t, "space1", results[1].SpaceId)
	})

	t.Run("offset pagination", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()
		addChatObject(t, fx, "chat1", spaceId, model.ObjectType_chatDerived)

		fx.chatRepoService.repos = map[string]chatrepository.Repository{
			"chat1": fakeRepoWithMessages(
				&model.ChatMessage{Id: "msg3", CreatedAt: 300},
				&model.ChatMessage{Id: "msg2", CreatedAt: 200},
				&model.ChatMessage{Id: "msg1", CreatedAt: 100},
			),
		}

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId: spaceId,
			Offset:  1,
			Limit:   1,
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "msg2", results[0].MessageId)
	})
}

func TestService_SearchAllSpaces(t *testing.T) {
	// given
	fx := newFixture(t)
	ctx := context.Background()

	fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
		Records: []*domain.Details{},
	}, nil).Maybe()
	addChatObject(t, fx, "chat1", "space1", model.ObjectType_chatDerived)
	addChatObject(t, fx, "chat2", "space2", model.ObjectType_chatDerived)

	// in the all-spaces scope the hit's stored space field drives attribution
	fx.ftSearch.EXPECT().SearchChat("", "", "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
		{Score: 2.0, ID: domain.NewObjectPathWithMessage("chat1", "msg1").String(), SpaceId: "space1"},
		{Score: 1.0, ID: domain.NewObjectPathWithMessage("chat2", "msg2").String(), SpaceId: "space2"},
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
