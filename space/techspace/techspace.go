package techspace

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

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

type TechSpace interface {
	Run(techCoreSpace *spacecore.AnySpace, objectCache objectcache.Cache) (err error)

	TechSpaceId() string
	SpaceViewCreate(ctx context.Context, spaceId string) (err error)
	SpaceViewExists(ctx context.Context, spaceId string) (exists bool, err error)
	SetInfo(ctx context.Context, info spaceinfo.SpaceInfo) (err error)
	SpaceViewSetData(ctx context.Context, spaceId string, details *types.Struct) (err error)
}

type SpaceView interface {
	sync.Locker
	SetSpaceData(details *types.Struct) error
	SetSpaceInfo(info spaceinfo.SpaceInfo) (err error)
}

func New() TechSpace {
	s := &techSpace{
		viewIds: make(map[string]string),
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	return s
}

type techSpace struct {
	techCore    *spacecore.AnySpace
	objectCache objectcache.Cache

	mu sync.Mutex

	ctx        context.Context
	ctxCancel  context.CancelFunc
	idsWakedUp chan struct{}
	viewIds    map[string]string
}

func (s *techSpace) Run(techCoreSpace *spacecore.AnySpace, objectCache objectcache.Cache) (err error) {
	s.techCore = techCoreSpace
	s.objectCache = objectCache
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
		if _, err := s.objectCache.GetObject(s.ctx, id); err != nil {
			log.Warn("wakeUp views: get object error", zap.String("objectId", id), zap.Error(err))
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
	return s.doSpaceView(ctx, info.SpaceID, func(spaceView SpaceView) error {
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
	_, err = s.objectCache.GetObject(ctx, viewId)
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
	_, getErr := s.objectCache.GetObject(ctx, viewId)
	return getErr == nil, nil
}

func (s *techSpace) SpaceViewSetData(ctx context.Context, spaceId string, details *types.Struct) (err error) {
	return s.doSpaceView(ctx, spaceId, func(spaceView SpaceView) error {
		return spaceView.SetSpaceData(details)
	})
}

func (s *techSpace) spaceViewCreate(ctx context.Context, spaceID string) (err error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceView, "")
	if err != nil {
		return
	}
	_, err = s.objectCache.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
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
	payload, err := s.objectCache.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{
		Key:           uniqueKey,
		TargetSpaceID: spaceID,
	})
	if err != nil {
		return "", err
	}
	return payload.RootRawChange.Id, nil
}

func (s *techSpace) doSpaceView(ctx context.Context, spaceID string, apply func(spaceView SpaceView) error) (err error) {
	viewId, err := s.getViewId(ctx, spaceID)
	if err != nil {
		return
	}
	obj, err := s.objectCache.GetObject(ctx, viewId)
	if err != nil {
		return ErrSpaceViewNotExists
	}
	spaceView, ok := obj.(SpaceView)
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
