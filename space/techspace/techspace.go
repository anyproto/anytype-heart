package techspace

import (
	"context"
	"fmt"
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

func New() TechSpace {
	return &techSpace{}
}

type TechSpace interface {
	CreateSpaceView(ctx context.Context, spaceID string) (spaceView *editor.SpaceView, err error)
	DeriveSpaceViewID(ctx context.Context, spaceID string) (string, error)
	SetStatuses(ctx context.Context, spaceID string, local spaceinfo.LocalStatus, remote spaceinfo.RemoteStatus) (err error)
	SetInfo(ctx context.Context, info spaceinfo.SpaceInfo) (err error)

	GetInfo(spaceID string) spaceinfo.SpaceInfo

	app.ComponentRunnable
}

type techSpace struct {
	techCore         *spacecore.AnySpace
	spaceCoreService spacecore.SpaceCoreService
	objectCache      objectcache.Cache

	info map[string]spaceinfo.SpaceInfo
	mu   sync.Mutex

	ctx       context.Context
	ctxCancel context.CancelFunc
}

func (s *techSpace) Init(a *app.App) (err error) {
	s.info = make(map[string]spaceinfo.SpaceInfo)
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
	go func() {
		if e := s.wakeUpViews(); e != nil {
			log.Warn("wake up views error", zap.Error(e))
		}
	}()
	return
}

func (s *techSpace) wakeUpViews() (err error) {
	for _, id := range s.techCore.StoredIds() {
		_ = s.doSpaceView(s.ctx, "", id, func(spaceView *editor.SpaceView) error {
			return nil
		})
	}
	s.techCore.TreeSyncer().StartSync()
	return
}

func (s *techSpace) CreateSpaceView(ctx context.Context, spaceID string) (spaceView *editor.SpaceView, err error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceObject, "")
	if err != nil {
		return
	}
	obj, err := s.objectCache.DeriveTreeObject(ctx, s.techCore.Id(), objectcache.TreeDerivationParams{
		Key: uniqueKey,
		InitFunc: func(id string) *editorsb.InitContext {
			return &editorsb.InitContext{Ctx: ctx, SpaceID: s.techCore.Id(), State: state.NewDoc(id, nil).(*state.State)}
		},
		TargetSpaceID: spaceID,
	})
	if err != nil {
		return
	}
	spaceView, ok := obj.(*editor.SpaceView)
	if !ok {
		return nil, fmt.Errorf("smartblock not a spaceView")
	}
	return spaceView, nil
}

func (s *techSpace) DeriveSpaceViewID(ctx context.Context, spaceID string) (string, error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceObject, "")
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

func (s *techSpace) getOrCreate(ctx context.Context, spaceID, viewID string) (*editor.SpaceView, error) {
	obj, err := s.objectCache.GetObject(ctx, domain.FullID{
		ObjectID: viewID,
		SpaceID:  s.techCore.Id(),
	})
	if err != nil { // TODO: check specific error
		return s.CreateSpaceView(ctx, spaceID)
	}
	spaceView, ok := obj.(*editor.SpaceView)
	if !ok {
		return nil, fmt.Errorf("smartblock not a spaceView")
	}
	return spaceView, nil
}

func (s *techSpace) doSpaceView(ctx context.Context, spaceID, viewID string, apply func(spaceView *editor.SpaceView) error) (err error) {
	spaceView, err := s.getOrCreate(ctx, spaceID, viewID)
	if err != nil {
		return err
	}
	spaceView.Lock()
	defer spaceView.Unlock()
	return apply(spaceView)
}

func (s *techSpace) SetStatuses(ctx context.Context, spaceID string, local spaceinfo.LocalStatus, remote spaceinfo.RemoteStatus) (err error) {
	s.mu.Lock()
	info, ok := s.info[spaceID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("space info not found")
	}
	info.LocalStatus = local
	info.RemoteStatus = remote
	s.info[spaceID] = info
	s.mu.Unlock()
	return s.doSpaceView(ctx, info.SpaceID, info.ViewID, func(spaceView *editor.SpaceView) error {
		return spaceView.SetSpaceInfo(info)
	})
}

func (s *techSpace) SetInfo(ctx context.Context, info spaceinfo.SpaceInfo) (err error) {
	s.mu.Lock()
	// do nothing if it's identical
	if s.info[info.SpaceID] == info {
		s.mu.Unlock()
		return nil
	}
	s.info[info.SpaceID] = info
	s.mu.Unlock()
	return s.doSpaceView(ctx, info.SpaceID, info.ViewID, func(spaceView *editor.SpaceView) error {
		return spaceView.SetSpaceInfo(info)
	})
}

func (s *techSpace) GetInfo(spaceID string) spaceinfo.SpaceInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info[spaceID]
}

func (s *techSpace) Close(ctx context.Context) (err error) {
	s.ctxCancel()
	return
}
