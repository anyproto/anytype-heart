package progressbar

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger(serviceName)

const serviceName = "spaceLoadingProgress"

type SpaceLoadingProgress interface {
}

type spaceLoadingProgress struct {
	processService        process.Service
	spaceService          space.Service
	subService            subscription.Service
	spaceViewSubscription *objectsubscription.ObjectSubscription[struct{}]
	ctx                   context.Context
	cancel                context.CancelFunc
}

func (s *spaceLoadingProgress) Init(a *app.App) (err error) {
	s.processService = app.MustComponent[process.Service](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.subService = app.MustComponent[subscription.Service](a)
	return
}

func (s *spaceLoadingProgress) Name() (name string) {
	return serviceName
}

func (s *spaceLoadingProgress) Run(ctx context.Context) (err error) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	objectReq := subscription.SubscribeRequest{
		SpaceId: s.spaceService.TechSpaceId(),
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyTargetSpaceId.String()},
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpaceAccountStatus.String(),
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       pbtypes.IntList(int(model.Account_Deleted)),
			},
			{
				RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(model.SpaceStatus_Ok), int(model.SpaceStatus_Unknown)),
			},
		},
		Internal: true,
	}
	s.spaceViewSubscription = objectsubscription.NewIdSubscription(s.subService, objectReq)
	err = s.spaceViewSubscription.Run()
	if err != nil {
		return fmt.Errorf("failed to run subscription: %w", err)
	}
	progress := process.NewProgress(&pb.ModelProcessMessageOfSpaceLoading{})
	err = s.processService.Add(progress)
	if err != nil {
		return fmt.Errorf("failed to create progress bar: %w", err)
	}
	progress.SetProgressMessage("start space view loading")
	progress.SetTotal(int64(s.spaceViewSubscription.Len()))
	s.readEvents(s.spaceViewSubscription)
	return nil
}

func (s *spaceLoadingProgress) readEvents(queue *mb.MB[*pb.EventMessage]) {
	matcher := subscription.EventMatcher{
		OnSet: func(detailsSet *pb.EventObjectDetailsSet) {

		},
		OnUnset: nil,
	}
	for {
		msgs, err := queue.Wait(s.ctx)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			log.Error(err)
			continue
		}
		for _, msg := range msgs {
			matcher.Match(msg)
		}
	}
}

func (s *spaceLoadingProgress) Close(ctx context.Context) (err error) {
	if s.cancel != nil {
		s.cancel()
	}
	return
}
