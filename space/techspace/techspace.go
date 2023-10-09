package techspace

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor"
	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.space.techspace"

var log = logger.NewNamed(CName)

var (
	ErrSpaceViewExists    = errors.New("spaceView exists")
	ErrSpaceViewNotExists = errors.New("spaceView not exists")
	ErrNotASpaceView      = errors.New("smartblock not a spaceView")
)

func New() TechSpace {
	return &techSpace{}
}

type TechSpace interface {
	TechSpaceId() string
	SpaceViewCreate(ctx context.Context, spaceId string) (err error)
	SpaceViewExists(ctx context.Context, spaceId string) (exists bool, err error)
	SetInfo(ctx context.Context, info spaceinfo.SpaceInfo) (err error)

	app.ComponentRunnable
}

type techSpace struct {
	techCore         *spacecore.AnySpace
	spaceCoreService spacecore.SpaceCoreService
	objectCache      objectcache.Cache

	mu sync.Mutex

	ctx        context.Context
	ctxCancel  context.CancelFunc
	idsWakedUp chan struct{}
	viewIds    map[string]string
}

func (s *techSpace) Init(a *app.App) (err error) {
	s.viewIds = make(map[string]string)
	s.objectCache = app.MustComponent[objectcache.Cache](a)
	s.spaceCoreService = app.MustComponent[spacecore.SpaceCoreService](a)
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	return
}

func (s *techSpace) Name() (name string) {
	return CName
}

func (s *techSpace) Run(ctx context.Context) (err error) {
	if s.techCore, err = s.spaceCoreService.Derive(ctx, spacecore.TechSpaceType); err != nil {
		return
	}
	s.idsWakedUp = make(chan struct{})
	go func() {
		defer close(s.idsWakedUp)
		s.wakeUpViews()
	}()
	return
}

func (s *techSpace) wakeUpViews() {
	for _, id := range s.techCore.StoredIds() {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		s.mu.Lock()
		if _, e := s.objectCache.GetObject(s.ctx, domain.FullID{
			ObjectID: id,
			SpaceID:  s.techCore.Id(),
		}); e != nil {
			log.Warn("wakeUp views: get object error", zap.Error(e))
		}
		s.mu.Unlock()
	}
	s.techCore.TreeSyncer().StartSync()
	return
}

func (s *techSpace) TechSpaceId() string {
	return s.techCore.Id()
}

func (s *techSpace) SetInfo(ctx context.Context, info spaceinfo.SpaceInfo) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doSpaceView(ctx, info.SpaceID, func(spaceView *editor.SpaceView) error {
		return spaceView.SetSpaceInfo(info)
	})
}

func (s *techSpace) SpaceViewCreate(ctx context.Context, spaceId string) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	viewId, err := s.getViewId(ctx, spaceId)
	if err != nil {
		return err
	}
	_, err = s.objectCache.GetObject(ctx, domain.FullID{
		ObjectID: viewId,
		SpaceID:  s.techCore.Id(),
	})
	if err != nil { // TODO: check specific error
		return s.spaceViewCreate(ctx, spaceId)
	}
	return ErrSpaceViewExists
}

func (s *techSpace) SpaceViewExists(ctx context.Context, spaceId string) (exists bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	viewId, err := s.getViewId(ctx, spaceId)
	if err != nil {
		return
	}
	_, getErr := s.objectCache.GetObject(ctx, domain.FullID{
		ObjectID: viewId,
		SpaceID:  s.techCore.Id(),
	})
	return getErr == nil, nil
}

func (s *techSpace) spaceViewCreate(ctx context.Context, spaceID string) (err error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceView, "")
	if err != nil {
		return
	}
	_, err = s.objectCache.DeriveTreeObject(ctx, s.techCore.Id(), objectcache.TreeDerivationParams{
		Key: uniqueKey,
		InitFunc: func(id string) *editorsb.InitContext {
			return &editorsb.InitContext{Ctx: ctx, SpaceID: s.techCore.Id(), State: state.NewDoc(id, nil).(*state.State)}
		},
		TargetSpaceID: spaceID,
	})
	if err != nil {
		return
	}
	return
}

func (s *techSpace) deriveSpaceViewID(ctx context.Context, spaceID string) (string, error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceView, "")
	if err != nil {
		return "", err
	}
	payload, err := s.objectCache.DeriveTreePayload(ctx, s.techCore.Id(), payloadcreator.PayloadDerivationParams{
		Key:           uniqueKey,
		TargetSpaceID: spaceID,
	})
	if err != nil {
		return "", err
	}
	return payload.RootRawChange.Id, nil
}

func (s *techSpace) doSpaceView(ctx context.Context, spaceID string, apply func(spaceView *editor.SpaceView) error) (err error) {
	viewId, err := s.getViewId(ctx, spaceID)
	if err != nil {
		return
	}
	obj, err := s.objectCache.GetObject(ctx, domain.FullID{
		ObjectID: viewId,
		SpaceID:  s.techCore.Id(),
	})
	if err != nil {
		return ErrSpaceViewNotExists
	}
	spaceView, ok := obj.(*editor.SpaceView)
	if !ok {
		return ErrNotASpaceView
	}

	spaceView.Lock()
	defer spaceView.Unlock()
	return apply(spaceView)
}

func (s *techSpace) getViewId(ctx context.Context, spaceId string) (viewId string, err error) {
	if viewId = s.viewIds[spaceId]; viewId != "" {
		return
	}
	if viewId, err = s.deriveSpaceViewID(ctx, spaceId); err != nil {
		return
	}
	s.viewIds[spaceId] = viewId
	return
}

func (s *techSpace) Close(ctx context.Context) (err error) {
	s.ctxCancel()
	if s.idsWakedUp != nil {
		<-s.idsWakedUp
	}
	return
}
