package chats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject/mock_chatobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Repro for: chat search scores are float BM25 values but get truncated to
// int64 in MessageModel (Score: int64(r.Score)), and the SCORE comparator then
// computes int(a.Score - b.Score). Scores that differ by less than 1.0 compare
// as equal, so sorting by score is broken for typical BM25 values.
func TestService_SearchScoreSorting(t *testing.T) {
	chatId := "chat1"
	spaceId := "space1"

	t.Run("results are sorted by descending BM25 score", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		// float scores closer than 1.0 to each other
		fx.ftSearch.EXPECT().SearchChat(spaceId, chatId, "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
			{Score: 2.1, ID: domain.NewObjectPathWithMessage(chatId, "msgLow").String()},
			{Score: 2.9, ID: domain.NewObjectPathWithMessage(chatId, "msgHigh").String()},
			{Score: 0.9, ID: domain.NewObjectPathWithMessage(chatId, "msgLowest").String()},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		// store returns the messages in arbitrary (non-score) order
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).Return([]*chatmodel.Message{
			{ChatMessage: &model.ChatMessage{Id: "msgLow"}},
			{ChatMessage: &model.ChatMessage{Id: "msgLowest"}},
			{ChatMessage: &model.ChatMessage{Id: "msgHigh"}},
		}, nil)
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "query",
			Sorts: []*model.SearchMessageSort{
				{Key: model.SearchMessageSort_SCORE, Type: model.SearchMessageSort_Desc},
			},
		})

		// then
		require.NoError(t, err)
		require.Len(t, results, 3)
		got := []string{results[0].MessageId, results[1].MessageId, results[2].MessageId}
		assert.Equal(t, []string{"msgHigh", "msgLow", "msgLowest"}, got)
	})

	t.Run("MessageModel keeps distinct sub-integer scores distinguishable", func(t *testing.T) {
		// given
		higher := database.FulltextResult{Score: 0.9}.MessageModel()
		lower := database.FulltextResult{Score: 0.3}.MessageModel()

		// then
		assert.Greater(t, higher.Score, lower.Score)
	})
}

// Repro for: offset is only applied when len(results) >= offset. Paginating
// past the last page returns the full first page again instead of an empty
// result, so clients can never detect the end of the result set.
func TestService_SearchOffsetBeyondEnd(t *testing.T) {
	chatId := "chat1"
	spaceId := "space1"

	// given
	fx := newFixture(t)
	ctx := context.Background()

	fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
		Records: []*domain.Details{},
	}, nil).Maybe()

	fx.ftSearch.EXPECT().SearchChat(spaceId, chatId, "query", mock.Anything).Return([]*ftsearch.DocumentMatch{
		{Score: 2.0, ID: domain.NewObjectPathWithMessage(chatId, "msg1").String()},
		{Score: 1.0, ID: domain.NewObjectPathWithMessage(chatId, "msg2").String()},
	}, nil)

	mockChatObj := mock_chatobject.NewMockStoreObject(t)
	mockChatObj.EXPECT().Lock().Return()
	mockChatObj.EXPECT().Unlock().Return()
	mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).Return([]*chatmodel.Message{
		{ChatMessage: &model.ChatMessage{Id: "msg1"}},
		{ChatMessage: &model.ChatMessage{Id: "msg2"}},
	}, nil)
	fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

	fx.start(t)

	// when: requesting a page past the end of the result set
	results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
		SpaceId:  spaceId,
		ChatId:   chatId,
		FullText: "query",
		Offset:   5,
	})

	// then
	require.NoError(t, err)
	assert.Empty(t, results, "offset beyond the result set must yield an empty page, not the first page again")
}
