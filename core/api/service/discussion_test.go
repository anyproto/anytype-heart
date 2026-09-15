package service

import (
	"context"
	"testing"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/stretchr/testify/assert"
)

func TestObjectService_AddDiscussion(t *testing.T) {
	t.Run("returns the new discussion id", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()
		fx.mwMock.EXPECT().ObjectAddDiscussion(ctx, &pb.RpcObjectDiscussionAddRequest{ObjectId: "obj1"}).Return(&pb.RpcObjectDiscussionAddResponse{
			DiscussionId: "discussion1",
			Error:        &pb.RpcObjectDiscussionAddResponseError{Code: pb.RpcObjectDiscussionAddResponseError_NULL},
		})

		discussionId, err := fx.service.AddDiscussion(ctx, "obj1")

		assert.NoError(t, err)
		assert.Equal(t, "discussion1", discussionId)
	})

	t.Run("middleware failure maps to error", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()
		fx.mwMock.EXPECT().ObjectAddDiscussion(ctx, &pb.RpcObjectDiscussionAddRequest{ObjectId: "obj1"}).Return(&pb.RpcObjectDiscussionAddResponse{
			Error: &pb.RpcObjectDiscussionAddResponseError{
				Code:        pb.RpcObjectDiscussionAddResponseError_UNKNOWN_ERROR,
				Description: "boom",
			},
		})

		_, err := fx.service.AddDiscussion(ctx, "obj1")

		assert.ErrorIs(t, err, ErrFailedAddDiscussion)
	})
}
