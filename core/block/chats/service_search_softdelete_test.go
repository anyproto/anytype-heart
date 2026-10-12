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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestService_SearchSkipsDeletedComments(t *testing.T) {
	spaceId := "space1"
	chatId := "discussion1"
	tombstone := func(id, creator string, createdAt int64) *chatmodel.Message {
		msg := &chatmodel.Message{ChatMessage: &model.ChatMessage{Id: id, Creator: creator, CreatedAt: createdAt}}
		msg.MakeTombstone()
		return msg
	}
	live := func(id, creator string, createdAt int64) *chatmodel.Message {
		return &chatmodel.Message{ChatMessage: &model.ChatMessage{
			Id: id, Creator: creator, CreatedAt: createdAt,
			Message: &model.ChatMessageMessageContent{Text: "text"},
		}}
	}
	newFx := func(t *testing.T) *fixture {
		fx := newFixture(t)
		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()
		addChatObject(t, fx, chatId, spaceId, model.ObjectType_discussion)
		return fx
	}

	t.Run("a stale fulltext hit on a deleted comment is dropped", func(t *testing.T) {
		// given
		fx := newFx(t)
		fx.ftSearch.EXPECT().SearchChat(spaceId, chatId, "query", mock.Anything, mock.Anything).Return([]*ftsearch.DocumentMatch{
			{Score: 2.0, ID: domain.NewObjectPathWithMessage(chatId, "msgDeleted").String()},
			{Score: 1.0, ID: domain.NewObjectPathWithMessage(chatId, "msgLive").String()},
		}, nil)
		fx.chatRepoService.repo = &fakeChatRepository{messagesByIds: []*chatmodel.Message{
			tombstone("msgDeleted", "A5alice", 200),
			live("msgLive", "A5alice", 100),
		}}
		fx.start(t)

		// when
		results, err := fx.Search(context.Background(), &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "query",
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "msgLive", results[0].MessageId)
	})

	// Browse with creators reads GetLastMessagesByCreators, which is live-only in the repository
	// itself (chatrepository softdelete_test); only the creatorless read is chosen here.
	t.Run("browse without creators reads live messages only", func(t *testing.T) {
		// given: the fake's GetLastMessages keeps tombstones like the real one; only the
		// live-only read skips them
		fx := newFx(t)
		fx.chatRepoService.repos = map[string]chatrepository.Repository{
			chatId: &fakeChatRepository{lastMessages: []*chatmodel.Message{
				tombstone("msgNewest", "A5alice", 300),
				tombstone("msgNewer", "A5alice", 200),
				live("msgOld", "A5alice", 100),
			}},
		}
		fx.start(t)

		// when
		results, err := fx.Search(context.Background(), &pb.RpcChatSearchRequest{
			SpaceId: spaceId,
			Limit:   1,
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "msgOld", results[0].MessageId)
	})
}
