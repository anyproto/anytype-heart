package space

import (
	"context"
	"fmt"

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

func newTechSpace(s *service, spaceCore *spacecore.AnySpace) *techSpace {
	return &techSpace{
		service:  s,
		techCore: spaceCore,
		info:     map[string]spaceinfo.SpaceInfo{},
	}
}

type techSpace struct {
	service  *service
	techCore *spacecore.AnySpace
	info     map[string]spaceinfo.SpaceInfo
}

func (s *techSpace) wakeUpViews(ctx context.Context) (err error) {
	for _, id := range s.techCore.StoredIds() {
		_ = s.doSpaceView(ctx, "", id, func(spaceView *editor.SpaceView) error {
			return nil
		})
	}
	return
}

func (s *techSpace) CreateSpaceView(ctx context.Context, spaceID string) (spaceView *editor.SpaceView, err error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceObject, "")
	if err != nil {
		return
	}
	obj, err := s.service.objectCache.DeriveTreeObject(ctx, s.techCore.Id(), objectcache.TreeDerivationParams{
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
	payload, err := s.service.objectCache.DeriveTreePayload(ctx, s.techCore.Id(), payloadcreator.PayloadDerivationParams{
		Key:           uniqueKey,
		TargetSpaceID: spaceID,
	})
	if err != nil {
		return "", err
	}
	return payload.RootRawChange.Id, nil
}

func (s *techSpace) getOrCreate(ctx context.Context, spaceID, viewID string) (*editor.SpaceView, error) {
	obj, err := s.service.objectCache.GetObject(ctx, domain.FullID{
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
	info, ok := s.info[spaceID]
	if !ok {
		return fmt.Errorf("not status")
	}
	info.LocalStatus = local
	info.RemoteStatus = remote
	return s.SetInfo(ctx, info)
}

func (s *techSpace) SetInfo(ctx context.Context, info spaceinfo.SpaceInfo) (err error) {
	// do nothing if it's identical
	if s.info[info.SpaceID] == info {
		return nil
	}
	s.info[info.SpaceID] = info
	return s.doSpaceView(ctx, info.SpaceID, info.ViewID, func(spaceView *editor.SpaceView) error {
		return spaceView.SetSpaceInfo(info)
	})
}

func (s *techSpace) GetInfo(spaceID string) spaceinfo.SpaceInfo {
	return s.info[spaceID]
}
