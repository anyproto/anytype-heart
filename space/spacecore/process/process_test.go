package process

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	mb2 "github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSpaceLoadingProgress_Run(t *testing.T) {
	t.Run("new account", func(t *testing.T) {
		// given
		f := newFixture(t, true)

		// when
		err := f.Run(context.Background())

		// then
		assert.NoError(t, err)
		assert.Nil(t, f.progress)
	})
}

func TestSpaceLoadingProgress_runSpaceLoadingProgress(t *testing.T) {
	t.Run("no space view", func(t *testing.T) {
		// given
		f := newFixture(t, false)
		techSpace := "techSpaceId"
		f.mockSpace.EXPECT().TechSpaceId().Return(techSpace)
		outputQueue := mb2.New[*pb.EventMessage](0)
		f.mockService.EXPECT().Search(subscription.SubscribeRequest{
			SpaceId: techSpace,
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceLocalStatus.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
			},
			Internal: true,
		}).Return(&subscription.SubscribeResponse{Output: outputQueue}, nil)
		f.ctx, f.cancel = context.WithCancel(context.Background())

		// when
		go f.runSpaceLoadingProgress()

		// then
		assert.NotNil(t, f.progress)
		assert.Equal(t, int64(0), f.progress.Info().Progress.Total)
		err := f.Close(nil)
		assert.Nil(t, err)
	})
	t.Run("missing space view", func(t *testing.T) {
		// given
		f := newFixture(t, false)
		techSpace := "techSpaceId"
		f.mockSpace.EXPECT().TechSpaceId().Return(techSpace)
		outputQueue := mb2.New[*pb.EventMessage](0)
		f.mockService.EXPECT().Search(subscription.SubscribeRequest{
			SpaceId: techSpace,
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceLocalStatus.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
			},
			Internal: true,
		}).Return(&subscription.SubscribeResponse{Output: outputQueue}, nil)
		f.ctx, f.cancel = context.WithCancel(context.Background())

		// when
		go f.runSpaceLoadingProgress()
		err := outputQueue.Add(context.Background(),
			&pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectDetailsSet{
					ObjectDetailsSet: &pb.EventObjectDetailsSet{
						Id: "id",
						Details: &types.Struct{Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():               pbtypes.String("id"),
							bundle.RelationKeySpaceLocalStatus.String(): pbtypes.Int64(int64(spaceinfo.LocalStatusMissing)),
						}},
					},
				},
			},
			&pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectDetailsSet{
					ObjectDetailsSet: &pb.EventObjectDetailsSet{
						Id: "id1",
						Details: &types.Struct{Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():               pbtypes.String("id1"),
							bundle.RelationKeySpaceLocalStatus.String(): pbtypes.Int64(int64(spaceinfo.LocalStatusOk)),
						}},
					},
				},
			})
		assert.Nil(t, err)

		// then
		waitForEmptyQueue(outputQueue)
		assert.Nil(t, f.Close(nil))
		assert.Equal(t, int64(1), f.progress.Info().Progress.Total)
		assert.Equal(t, int64(1), f.progress.Info().Progress.Done)
	})
	t.Run("space view is loading", func(t *testing.T) {
		// given
		f := newFixture(t, false)
		techSpace := "techSpaceId"
		f.mockSpace.EXPECT().TechSpaceId().Return(techSpace)
		outputQueue := mb2.New[*pb.EventMessage](0)
		f.mockService.EXPECT().Search(subscription.SubscribeRequest{
			SpaceId: techSpace,
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceLocalStatus.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
			},
			Internal: true,
		}).Return(&subscription.SubscribeResponse{Output: outputQueue}, nil)
		f.ctx, f.cancel = context.WithCancel(context.Background())

		// when
		go f.runSpaceLoadingProgress()
		err := outputQueue.Add(context.Background(),
			&pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectDetailsAmend{
					ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
						Id: "id",
						Details: []*pb.EventObjectDetailsAmendKeyValue{
							{
								Key:   bundle.RelationKeySpaceLocalStatus.String(),
								Value: pbtypes.Int64(int64(spaceinfo.LocalStatusLoading)),
							},
							{
								Key:   bundle.RelationKeyId.String(),
								Value: pbtypes.String("id"),
							},
						},
					},
				},
			})
		assert.Nil(t, err)

		// then
		waitForEmptyQueue(outputQueue)
		assert.Nil(t, f.Close(nil))
		assert.Equal(t, int64(1), f.progress.Info().Progress.Total)
		assert.Equal(t, int64(0), f.progress.Info().Progress.Done)
	})
	t.Run("cancel process", func(t *testing.T) {
		// given
		f := newFixture(t, false)
		techSpace := "techSpaceId"
		f.mockSpace.EXPECT().TechSpaceId().Return(techSpace)
		outputQueue := mb2.New[*pb.EventMessage](0)
		f.mockService.EXPECT().Search(subscription.SubscribeRequest{
			SpaceId: techSpace,
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceLocalStatus.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
			},
			Internal: true,
		}).Return(&subscription.SubscribeResponse{Output: outputQueue}, nil)
		f.ctx, f.cancel = context.WithCancel(context.Background())

		// when
		go f.runSpaceLoadingProgress()
		for {
			if f.progress == nil {
				continue
			}
			err := f.progress.Cancel()
			assert.NoError(t, err)
			<-f.progress.Canceled()
			break
		}

		// then
		assert.Equal(t, pb.ModelProcess_Canceled, f.progress.Info().State)
	})
}

func waitForEmptyQueue(outputQueue *mb2.MB[*pb.EventMessage]) {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	var stop bool
	for {
		if stop {
			break
		}
		select {
		case <-ticker.C:
			queueLen := outputQueue.Len()
			if queueLen == 0 {
				stop = true
				break
			}
		}
	}
}

func newFixture(t *testing.T, newAccount bool) *fixture {
	slp := &spaceLoadingProgress{}

	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Maybe()
	eventSender.EXPECT().BroadcastExceptSessions(mock.Anything, mock.Anything).Maybe()
	service := process.New()
	mockService := mock_subscription.NewMockService(t)
	mockSpace := mock_space.NewMockService(t)
	c := &config.Config{NewAccount: newAccount}

	a := &app.App{}

	a.Register(service).
		Register(testutil.PrepareMock(context.Background(), a, eventSender)).
		Register(testutil.PrepareMock(context.Background(), a, mockService)).
		Register(testutil.PrepareMock(context.Background(), a, mockSpace)).
		Register(c)

	err := service.Init(a)
	assert.Nil(t, err)
	err = slp.Init(a)
	assert.Nil(t, err)
	return &fixture{
		spaceLoadingProgress: slp,
		mockService:          mockService,
		processService:       service,
		cfg:                  c,
		mockSpace:            mockSpace,
	}
}

type fixture struct {
	*spaceLoadingProgress
	mockService    *mock_subscription.MockService
	processService process.Service
	cfg            *config.Config
	mockSpace      *mock_space.MockService
}
