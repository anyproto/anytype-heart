package spaceprogressbar

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
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger(serviceName)

const serviceName = "spaceLoadingProgress"

type spaceLoadingProgress struct {
	processService        process.Service
	spaceService          space.Service
	subService            subscription.Service
	spaceViewSubscription *objectsubscription.ObjectSubscription[struct{}]
	ctx                   context.Context
	cancel                context.CancelFunc
	spaceViewIds          map[string]struct{}
}

func NewSpaceLoadingProgress() app.ComponentRunnable {
	return &spaceLoadingProgress{}
}

func (s *spaceLoadingProgress) Init(a *app.App) (err error) {
	s.processService = app.MustComponent[process.Service](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.subService = app.MustComponent[subscription.Service](a)
	s.spaceViewIds = make(map[string]struct{})
	return
}

func (s *spaceLoadingProgress) Name() (name string) {
	return serviceName
}

func (s *spaceLoadingProgress) Run(ctx context.Context) (err error) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	searchReq := subscription.SubscribeRequest{
		SpaceId: s.spaceService.TechSpaceId(),
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceLocalStatus.String()},
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
		},
		Internal: true,
	}
	resp, err := s.subService.Search(searchReq)
	if err != nil {
		return fmt.Errorf("failed to run search: %w", err)
	}
	techSpace, err := s.spaceService.GetTechSpace(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to get tech space: %w", err)
	}
	ts, ok := techSpace.(techspace.TechSpace)
	if !ok {
		return errors.New("failed to cast tech space")
	}
	spaceViewIds := ts.GetViewIds()
	progress, err := s.makeProgressBar(len(spaceViewIds))
	if err != nil {
		return fmt.Errorf("failed to create progress bar: %w", err)
	}
	s.fillIdsMap(spaceViewIds)
	go s.readEvents(resp.Output, progress)
	return nil
}

func (s *spaceLoadingProgress) makeProgressBar(spaceViewCount int) (process.Progress, error) {
	progress := process.NewProgress(&pb.ModelProcessMessageOfSpaceLoading{})
	err := s.processService.Add(progress)
	if err != nil {
		return nil, err
	}
	progress.SetProgressMessage("start space loading")
	progress.SetTotal(int64(spaceViewCount))
	return progress, nil
}

func (s *spaceLoadingProgress) fillIdsMap(spaceViewIds []string) {
	for _, id := range spaceViewIds {
		s.spaceViewIds[id] = struct{}{}
	}
}

func (s *spaceLoadingProgress) readEvents(batcher *mb.MB[*pb.EventMessage], progress process.Progress) {
	defer progress.Finish(nil)
	if len(s.spaceViewIds) == 0 {
		return
	}
	matcher := subscription.EventMatcher{
		OnSet:   s.handleDetailsSetEvent(progress),
		OnAmend: s.handleDetailsAmendEvent(progress),
	}
	for {
		records, err := batcher.Wait(s.ctx)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			continue
		}
		for _, rec := range records {
			matcher.Match(rec)
		}
		if len(s.spaceViewIds) == 0 {
			return
		}
	}
}

func (s *spaceLoadingProgress) handleDetailsSetEvent(progress process.Progress) func(detailsSet *pb.EventObjectDetailsSet) {
	return func(detailsSet *pb.EventObjectDetailsSet) {
		status := pbtypes.GetInt64(detailsSet.Details, bundle.RelationKeySpaceLocalStatus.String())
		id := pbtypes.GetString(detailsSet.Details, bundle.RelationKeyId.String())
		if _, ok := s.spaceViewIds[id]; ok && status == int64(model.SpaceStatus_Ok) {
			progress.SetProgressMessage("space was loaded")
			progress.AddDone(1)
			delete(s.spaceViewIds, id)
		}
	}
}

func (s *spaceLoadingProgress) handleDetailsAmendEvent(progress process.Progress) func(detailsAmend *pb.EventObjectDetailsAmend) {
	return func(detailsAmend *pb.EventObjectDetailsAmend) {
		for _, detail := range detailsAmend.Details {
			if _, ok := s.spaceViewIds[detailsAmend.Id]; !ok {
				continue
			}
			if detail.Key != bundle.RelationKeySpaceLocalStatus.String() {
				return
			}
			if detail.Value.GetNumberValue() == float64(model.SpaceStatus_Ok) {
				progress.SetProgressMessage("space was loaded")
				progress.AddDone(1)
				delete(s.spaceViewIds, detailsAmend.Id)
			}
		}
	}
}

func (s *spaceLoadingProgress) Close(ctx context.Context) (err error) {
	if s.cancel != nil {
		s.cancel()
	}
	return
}
