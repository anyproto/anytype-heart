package techspace

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.space.techspace"

var log = logger.NewNamed(CName)

const spaceViewCheckTimeout = time.Second * 15

var (
	ErrSpaceViewExists    = errors.New("spaceView exists")
	ErrSpaceViewNotExists = errors.New("spaceView not exists")
	ErrNotASpaceView      = errors.New("smartblock not a spaceView")
)

type TechSpace interface {
	app.Component
	Run(techCoreSpace commonspace.Space, objectCache objectcache.Cache) (err error)
	Close(ctx context.Context) (err error)

	TechSpaceId() string
	SpaceViewCreate(ctx context.Context, spaceId string, force bool, info spaceinfo.SpacePersistentInfo) (err error)
	GetSpaceView(ctx context.Context, spaceId string) (SpaceView, error)
	SpaceViewExists(ctx context.Context, spaceId string) (exists bool, err error)
	SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error)
	SetAccessType(ctx context.Context, spaceId string, acc spaceinfo.AccessType) (err error)
	SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) (err error)
	SpaceViewSetData(ctx context.Context, spaceId string, details *types.Struct) (err error)
	SpaceViewId(id string) (string, error)
}

type SpaceView interface {
	sync.Locker
	GetPersistentInfo() spaceinfo.SpacePersistentInfo
	GetLocalInfo() spaceinfo.SpaceLocalInfo
	SetSpaceData(details *types.Struct) error
	SetSpaceLocalInfo(info spaceinfo.SpaceLocalInfo) error
	SetAccessType(acc spaceinfo.AccessType) error
	SetSpacePersistentInfo(info spaceinfo.SpacePersistentInfo) error
}

func New() TechSpace {
	s := &techSpace{
		viewIds: make(map[string]string),
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	return s
}

type techSpace struct {
	techCore    commonspace.Space
	objectCache objectcache.Cache

	mu sync.Mutex

	ctx        context.Context
	ctxCancel  context.CancelFunc
	idsWakedUp chan struct{}
	viewIds    map[string]string
}

func (s *techSpace) Init(a *app.App) (err error) {
	return nil
}

func (s *techSpace) Name() (name string) {
	return CName
}

func (s *techSpace) Run(techCoreSpace commonspace.Space, objectCache objectcache.Cache) (err error) {
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

		if _, err := s.objectCache.GetObject(s.ctx, id); err != nil {
			log.Warn("wakeUp views: get object error", zap.String("objectId", id), zap.Error(err))
		}
	}
	s.techCore.TreeSyncer().StartSync()
	return
}

func (s *techSpace) TechSpaceId() string {
	return s.techCore.Id()
}

func (s *techSpace) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error) {
	return s.doSpaceView(ctx, info.SpaceId, func(spaceView SpaceView) error {
		return spaceView.SetSpaceLocalInfo(info)
	})
}

func (s *techSpace) SetAccessType(ctx context.Context, spaceId string, acc spaceinfo.AccessType) (err error) {
	return s.doSpaceView(ctx, spaceId, func(spaceView SpaceView) error {
		return spaceView.SetAccessType(acc)
	})
}

func (s *techSpace) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) (err error) {
	return s.doSpaceView(ctx, info.SpaceID, func(spaceView SpaceView) error {
		return spaceView.SetSpacePersistentInfo(info)
	})
}

func (s *techSpace) SpaceViewCreate(ctx context.Context, spaceId string, force bool, info spaceinfo.SpacePersistentInfo) (err error) {
	if force {
		return s.spaceViewCreate(ctx, spaceId, info)
	}
	viewId, err := s.getViewIdLocked(ctx, spaceId)
	if err != nil {
		return err
	}
	_, err = s.objectCache.GetObject(ctx, viewId)
	if err != nil { // TODO: check specific error
		return s.spaceViewCreate(ctx, spaceId, info)
	}
	return ErrSpaceViewExists
}

func (s *techSpace) SpaceViewExists(ctx context.Context, spaceId string) (exists bool, err error) {
	viewId, err := s.getViewIdLocked(ctx, spaceId)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, spaceViewCheckTimeout)
	defer cancel()
	ctx = peer.CtxWithPeerId(ctx, peer.CtxResponsiblePeers)
	_, getErr := s.objectCache.GetObject(ctx, viewId)
	return getErr == nil, nil
}

func (s *techSpace) GetSpaceView(ctx context.Context, spaceId string) (SpaceView, error) {
	viewId, err := s.getViewIdLocked(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	obj, err := s.objectCache.GetObject(ctx, viewId)
	if err != nil {
		return nil, err
	}
	spaceView, ok := obj.(SpaceView)
	if !ok {
		return nil, ErrNotASpaceView
	}
	return spaceView, nil
}

func (s *techSpace) SpaceViewSetData(ctx context.Context, spaceId string, details *types.Struct) (err error) {
	return s.doSpaceView(ctx, spaceId, func(spaceView SpaceView) error {
		return spaceView.SetSpaceData(details)
	})
}

func (s *techSpace) SpaceViewId(spaceId string) (string, error) {
	return s.getViewIdLocked(context.TODO(), spaceId)
}

func (s *techSpace) spaceViewCreate(ctx context.Context, spaceID string, info spaceinfo.SpacePersistentInfo) (err error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceView, spaceID)
	if err != nil {
		return
	}
	initFunc := func(id string) *editorsb.InitContext {
		st := state.NewDoc(id, nil).(*state.State)
		info.UpdateDetails(st)
		return &editorsb.InitContext{Ctx: ctx, SpaceID: s.techCore.Id(), State: st}
	}
	_, err = s.objectCache.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
		Key:      uniqueKey,
		InitFunc: initFunc,
	})
	if err != nil {
		return
	}
	return
}

func (s *techSpace) deriveSpaceViewID(ctx context.Context, spaceID string) (string, error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceView, spaceID)
	if err != nil {
		return "", err
	}
	payload, err := s.objectCache.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{
		Key: uniqueKey,
	})
	if err != nil {
		return "", err
	}
	return payload.RootRawChange.Id, nil
}

func (s *techSpace) doSpaceView(ctx context.Context, spaceID string, apply func(spaceView SpaceView) error) (err error) {
	viewId, err := s.getViewIdLocked(ctx, spaceID)
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

func (s *techSpace) getViewIdLocked(ctx context.Context, spaceId string) (viewId string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return nil
}
