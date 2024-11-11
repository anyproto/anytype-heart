package process

import (
	"context"

	"github.com/anyproto/any-sync/app"
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

const serviceName = "client.space.spacecore.spaceLoadingProgress"

type spaceLoadingProgress struct {
	processService     process.Service
	spaceService       space.Service
	subService         subscription.Service
	activeViewIds      map[string]struct{}
	spaceViewIdsLoaded map[string]struct{}
	newAccount         bool
	progress           process.Progress
	sub                *objectsubscription.ObjectSubscription[struct{}]
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
	s.runSpaceLoadingProgress()
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
	s.sub = objectsubscription.New[struct{}](s.subService, objectsubscription.SubscriptionParams[struct{}]{
		Request: searchReq,
		Update: func(spaceId, key string, detail *types.Value, _ struct{}) struct{} {
			s.handleDetailsAmendEvent(spaceId, key, detail)
			return struct{}{}
		},
		Extract: func(spaceDetails *types.Struct) (string, struct{}) {
			s.handleDetailsSetEvent(spaceDetails)
			return "", struct{}{}
		},
		Unset: func(strings []string, s struct{}) struct{} {
			return struct{}{}
		},
	})
	var err error
	s.progress, err = s.makeProgressBar()
	if err != nil {
		log.Errorf("failed to create progress bar: %s", err)
		return
	}
	go func() {
		for {
			select {
			case <-s.progress.Canceled():
				s.progress.Finish(nil)
				s.sub.Close()
			}
		}
	}()
	err = s.sub.Run()
	if err != nil {
		log.Errorf("failed to run search: %s", err)
		return
	}
}

func (s *spaceLoadingProgress) makeProgressBar() (process.Progress, error) {
	progress := process.NewProgress(&pb.ModelProcessMessageOfSpaceLoading{})
	err := s.processService.Add(progress)
	if err != nil {
		return nil, err
	}
	progress.SetProgressMessage("start space loading")
	progress.SetTotal(0)
	return progress, nil
}

func (s *spaceLoadingProgress) handleDetailsSetEvent(details *types.Struct) {
	status := pbtypes.GetInt64(details, bundle.RelationKeySpaceLocalStatus.String())
	spaceId := pbtypes.GetString(details, bundle.RelationKeyId.String())
	s.updateProgressBar(spaceId, status)
}

func (s *spaceLoadingProgress) handleDetailsAmendEvent(spaceId, key string, details *types.Value) {
	if key != bundle.RelationKeySpaceLocalStatus.String() {
		return
	}
	s.updateProgressBar(spaceId, int64(details.GetNumberValue()))
}

func (s *spaceLoadingProgress) updateProgressBar(spaceId string, status int64) {
	// in case space is missed, decrease number of active view
	if status == int64(spaceinfo.LocalStatusMissing) {
		delete(s.activeViewIds, spaceId)
		s.progress.SetTotal(int64(len(s.activeViewIds)))
		return
	}
	// if new space view is appeared, increase number of total active view
	if _, ok := s.activeViewIds[spaceId]; !ok {
		s.activeViewIds[spaceId] = struct{}{}
		s.progress.SetTotal(int64(len(s.activeViewIds)))
	}
	// mark space view as loaded
	if status == int64(spaceinfo.LocalStatusOk) {
		s.spaceViewIdsLoaded[spaceId] = struct{}{}
		s.progress.SetProgressMessage("space was loaded")
		s.progress.SetDone(int64(len(s.spaceViewIdsLoaded)))
	}

}

func (s *spaceLoadingProgress) Close(ctx context.Context) (err error) {
	s.progress.Finish(nil)
	s.sub.Close()
	return
}
