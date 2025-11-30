package techspace

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/util/crypto"

	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.space.techspace"

var log = logger.NewNamed(CName)

const spaceViewCheckTimeout = time.Second * 15

var (
	ErrSpaceViewExists        = errors.New("spaceView exists")
	ErrSpaceViewNotExists     = errors.New("spaceView not exists")
	ErrAccountObjectNotExists = errors.New("accountObject not exists")
	ErrNotASpaceView          = errors.New("smartblock not a spaceView")
	ErrNotAnAccountObject     = errors.New("smartblock not an accountObject")
	ErrNotStarted             = errors.New("techspace not started")
)

type AccountObject interface {
	editorsb.SmartBlock
	SetSharedSpacesLimit(limit int) (err error)
	SetProfileDetails(details *domain.Details) (err error)
	MigrateIconImage(image string) (err error)
	IsIconMigrated() (bool, error)
	SetAnalyticsId(analyticsId string) (err error)
	GetAnalyticsId() (string, error)
	SetInboxOffset(offset string) (err error)
	GetInboxOffset() (string, error)
}

type TechSpace interface {
	app.Component
	Run(techCoreSpace commonspace.Space, objectCache objectcache.Cache, create bool) (err error)
	Close(ctx context.Context) (err error)

	TechSpaceId() string
	StartSync()
	DoSpaceView(ctx context.Context, spaceID string, apply func(spaceView SpaceView) error) (err error)
	DoAccountObject(ctx context.Context, apply func(accountObject AccountObject) error) (err error)
	SpaceViewCreate(ctx context.Context, spaceId string, force bool, info spaceinfo.SpacePersistentInfo, desc *spaceinfo.SpaceDescription) (err error)
	GetSpaceView(ctx context.Context, spaceId string) (SpaceView, error)
	SpaceViewExists(ctx context.Context, spaceId string) (exists bool, err error)
	SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error)
	SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) (err error)
	SpaceViewSetData(ctx context.Context, spaceId string, details *domain.Details) (err error)
	SpaceViewId(id string) (string, error)
	AccountObjectId() (string, error)
}

type SpaceView interface {
	sync.Locker
	GetPersistentInfo() spaceinfo.SpacePersistentInfo
	GetLocalInfo() spaceinfo.SpaceLocalInfo
	SetSpaceData(details *domain.Details) error
	SetSpaceLocalInfo(info spaceinfo.SpaceLocalInfo) error
	SetAccessType(acc spaceinfo.AccessType) error
	SetAclInfo(empty bool, pushKey crypto.PrivKey, pushEncKey crypto.SymKey, joinedDate int64) (err error)
	SetOwner(ownerId string, createdDate int64) (err error)
	SetSpacePersistentInfo(info spaceinfo.SpacePersistentInfo) error
	SetMyParticipantStatus(status model.ParticipantStatus) error
	GetSpaceDescription() (data spaceinfo.SpaceDescription)
	SetSharedSpacesLimit(limits int) (err error)
	GetSharedSpacesLimit() (limits int)
	SetPushNotificationMode(ctx session.Context, mode pb.RpcPushNotificationMode) (err error)
	SetPushNotificationForceModeIds(ctx session.Context, chatIds []string, mode pb.RpcPushNotificationMode) (err error)
	ResetPushNotificationIds(ctx session.Context, allIds []string) error
}

func New() TechSpace {
	s := &techSpace{
		viewIds: make(map[string]string),
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	return s
}

type techSpace struct {
	techCore        commonspace.Space
	objectCache     objectcache.Cache
	accountObjectId string

	mu sync.Mutex

	ctx        context.Context
	ctxCancel  context.CancelFunc
	idsWokenUp chan struct{}
	isClosed   bool
	viewIds    map[string]string
}

func (s *techSpace) Init(a *app.App) (err error) {
	return nil
}

func (s *techSpace) Name() (name string) {
	return CName
}

func (s *techSpace) Run(techCoreSpace commonspace.Space, objectCache objectcache.Cache, create bool) (err error) {
	s.techCore = techCoreSpace
	s.objectCache = objectCache
	if !create {
		exists, err := s.accountObjectExists(s.ctx)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}
	return s.accountObjectCreate(s.ctx)
}

func (s *techSpace) StartSync() {
	s.techCore.TreeSyncer().StartSync()
}

func (s *techSpace) TechSpaceId() string {
	return s.techCore.Id()
}

func (s *techSpace) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error) {
	return s.DoSpaceView(ctx, info.SpaceId, func(spaceView SpaceView) error {
		return spaceView.SetSpaceLocalInfo(info)
	})
}

func (s *techSpace) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) (err error) {
	return s.DoSpaceView(ctx, info.SpaceID, func(spaceView SpaceView) error {
		return spaceView.SetSpacePersistentInfo(info)
	})
}

func (s *techSpace) SpaceViewCreate(ctx context.Context, spaceId string, force bool, info spaceinfo.SpacePersistentInfo, desc *spaceinfo.SpaceDescription) (err error) {
	if force {
		return s.spaceViewCreate(ctx, spaceId, info, desc)
	}
	viewId, err := s.getViewIdLocked(ctx, spaceId)
	if err != nil {
		return err
	}
	_, err = s.objectCache.GetObject(ctx, viewId)
	if err != nil { // TODO: check specific error
		return s.spaceViewCreate(ctx, spaceId, info, desc)
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

func (s *techSpace) accountObjectExists(ctx context.Context) (exists bool, err error) {
	objId, err := s.AccountObjectId()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, spaceViewCheckTimeout)
	defer cancel()
	ctx = peer.CtxWithPeerId(ctx, peer.CtxResponsiblePeers)
	_, getErr := s.objectCache.GetObject(ctx, objId)
	return getErr == nil, nil
}

func (s *techSpace) GetSpaceView(ctx context.Context, spaceId string) (SpaceView, error) {
	viewId, err := s.getViewIdLocked(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	obj, err := s.objectCache.GetObject(ctx, viewId)
	if err != nil {
		return nil, errors.Join(ErrSpaceViewNotExists, err)
	}
	spaceView, ok := obj.(SpaceView)
	if !ok {
		return nil, ErrNotASpaceView
	}
	return spaceView, nil
}

func (s *techSpace) SpaceViewSetData(ctx context.Context, spaceId string, details *domain.Details) (err error) {
	return s.DoSpaceView(ctx, spaceId, func(spaceView SpaceView) error {
		return spaceView.SetSpaceData(details)
	})
}

func (s *techSpace) SpaceViewId(spaceId string) (string, error) {
	return s.getViewIdLocked(context.TODO(), spaceId)
}

func (s *techSpace) spaceViewCreate(ctx context.Context, spaceID string, info spaceinfo.SpacePersistentInfo, description *spaceinfo.SpaceDescription) (err error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceView, spaceID)
	if err != nil {
		return
	}
	initFunc := func(id string) *editorsb.InitContext {
		st := state.NewDoc(id, nil).(*state.State)
		info.UpdateDetails(st)
		if description != nil {
			description.UpdateDetails(st)
		}
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

func (s *techSpace) accountObjectCreate(ctx context.Context) (err error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeAccountObject, s.techCore.Id())
	if err != nil {
		return
	}
	initFunc := func(id string) *editorsb.InitContext {
		st := state.NewDoc(id, nil).(*state.State)
		return &editorsb.InitContext{Ctx: ctx, SpaceID: s.techCore.Id(), State: st}
	}
	_, err = s.objectCache.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
		Key:      uniqueKey,
		InitFunc: initFunc,
	})
	if errors.Is(err, treestorage.ErrTreeExists) {
		accId, err := s.AccountObjectId()
		if err != nil {
			return err
		}
		_, err = s.objectCache.GetObject(ctx, accId)
		return err
	}
	return
}

func (s *techSpace) AccountObjectId() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountObjectId != "" {
		return s.accountObjectId, nil
	}
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeAccountObject, s.techCore.Id())
	if err != nil {
		return "", err
	}
	payload, err := s.objectCache.DeriveTreePayload(context.Background(), payloadcreator.PayloadDerivationParams{
		Key: uniqueKey,
	})
	if err != nil {
		return "", err
	}
	s.accountObjectId = payload.RootRawChange.Id
	return payload.RootRawChange.Id, nil
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

func (s *techSpace) DoSpaceView(ctx context.Context, spaceID string, apply func(spaceView SpaceView) error) (err error) {
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

func (s *techSpace) DoAccountObject(ctx context.Context, apply func(accountObject AccountObject) error) (err error) {
	id, err := s.AccountObjectId()
	if err != nil {
		return err
	}
	obj, err := s.objectCache.GetObject(ctx, id)
	if err != nil {
		return fmt.Errorf("account object not exists %w: %w", ErrAccountObjectNotExists, err)
	}
	accountObject, ok := obj.(AccountObject)
	if !ok {
		return ErrNotAnAccountObject
	}

	accountObject.Lock()
	defer accountObject.Unlock()
	return apply(accountObject)
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

func (s *techSpace) KeyValueStore() keyvaluestorage.Storage {
	return s.techCore.KeyValue().DefaultStore()
}

func (s *techSpace) Close(ctx context.Context) (err error) {
	s.ctxCancel()
	s.mu.Lock()
	s.isClosed = true
	wokenUp := s.idsWokenUp
	s.mu.Unlock()
	if wokenUp != nil {
		<-s.idsWokenUp
	}
	return nil
}
