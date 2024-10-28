package process

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
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
	activeViewIds         map[string]struct{}
	spaceViewIdsLoaded    map[string]struct{}
	newAccount            bool
}

func NewSpaceLoadingProgress() app.ComponentRunnable {
	return &spaceLoadingProgress{}
}

func (s *spaceLoadingProgress) Init(a *app.App) (err error) {
	s.processService = app.MustComponent[process.Service](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.subService = app.MustComponent[subscription.Service](a)
	s.activeViewIds = make(map[string]struct{})
	s.spaceViewIdsLoaded = make(map[string]struct{})
	config := app.MustComponent[*config.Config](a)
	s.newAccount = config.IsNewAccount()
	return
}

func (s *spaceLoadingProgress) Name() (name string) {
	return serviceName
}

func (s *spaceLoadingProgress) Run(ctx context.Context) (err error) {
	if s.newAccount {
		return nil
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.runSpaceLoadingProgress()
	return nil
}

func (s *spaceLoadingProgress) runSpaceLoadingProgress() {
	searchReq := subscription.SubscribeRequest{
		SpaceId: s.spaceService.TechSpaceId(),
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceLocalStatus.String()},
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
		},
		Internal: true,
	}
	resp, err := s.subService.Search(searchReq)
	if err != nil {
		log.Errorf("failed to run search: %s", err)
		return
	}
	progress, err := s.makeProgressBar(len(resp.Records))
	s.fillIdsMap(resp.Records)
	go s.readEvents(resp.Output, progress)
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

func (s *spaceLoadingProgress) fillIdsMap(spaceViews []*types.Struct) {
	for _, spaceView := range spaceViews {
		id := pbtypes.GetString(spaceView, bundle.RelationKeyId.String())
		s.activeViewIds[id] = struct{}{}
	}
}

func (s *spaceLoadingProgress) readEvents(batcher *mb.MB[*pb.EventMessage], progress process.Progress) {
	defer progress.Finish(nil)
	matcher := subscription.EventMatcher{
		OnSet:   s.handleDetailsSetEvent(progress),
		OnAmend: s.handleDetailsAmendEvent(progress),
	}
	for {
		select {
		case <-progress.Canceled():
			return
		default:
		}
		records, err := batcher.Wait(s.ctx)
		if errors.Is(err, context.Canceled) {
			if err != nil {
				log.Errorf("failed to cancel process, %s", err)
			}
			return
		}
		if err != nil {
			continue
		}
		for _, rec := range records {
			matcher.Match(rec)
		}
	}
}

func (s *spaceLoadingProgress) handleDetailsSetEvent(progress process.Progress) func(detailsSet *pb.EventObjectDetailsSet) {
	return func(detailsSet *pb.EventObjectDetailsSet) {
		status := pbtypes.GetInt64(detailsSet.Details, bundle.RelationKeySpaceLocalStatus.String())
		id := pbtypes.GetString(detailsSet.Details, bundle.RelationKeyId.String())
		s.updateProgressBar(id, progress, status)
	}
}

func (s *spaceLoadingProgress) handleDetailsAmendEvent(progress process.Progress) func(detailsAmend *pb.EventObjectDetailsAmend) {
	return func(detailsAmend *pb.EventObjectDetailsAmend) {
		for _, detail := range detailsAmend.Details {
			if detail.Key != bundle.RelationKeySpaceLocalStatus.String() {
				continue
			}
			s.updateProgressBar(detailsAmend.Id, progress, int64(detail.Value.GetNumberValue()))
		}
	}
}

func (s *spaceLoadingProgress) updateProgressBar(id string, progress process.Progress, status int64) {
	// in case space is missed, decrease number of active view
	if status == int64(spaceinfo.LocalStatusMissing) {
		delete(s.activeViewIds, id)
		progress.SetTotal(int64(len(s.activeViewIds)))
		return
	}
	// if new space view is appeared, increase number of total active view
	if _, ok := s.activeViewIds[id]; !ok {
		s.activeViewIds[id] = struct{}{}
		progress.SetTotal(int64(len(s.activeViewIds)))
	}
	// if space view is not loaded, remove it from map and decrease number of processed view
	if status != int64(model.SpaceStatus_Ok) {
		delete(s.spaceViewIdsLoaded, id)
		progress.SetDone(int64(len(s.spaceViewIdsLoaded)))
	}
	// mark space view as loaded
	if status == int64(model.SpaceStatus_Ok) {
		s.spaceViewIdsLoaded[id] = struct{}{}
		progress.SetProgressMessage("space was loaded")
		progress.SetDone(int64(len(s.spaceViewIdsLoaded)))
	}

}

func (s *spaceLoadingProgress) Close(ctx context.Context) (err error) {
	if s.cancel != nil {
		s.cancel()
	}
	return
}
