package space

import (
	"context"

	"github.com/anyproto/any-sync/commonspace"

	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/objectprovider"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type Space interface {
	commonspace.Space
	ViewID() string
	DerivedIDs() threads.DerivedSmartblockIds

	WaitMandatoryObjects(ctx context.Context) (err error)
}

func newSpace(s *service, coreSpace *spacecore.AnySpace, derivedIDs threads.DerivedSmartblockIds) *space {
	sp := &space{
		service:                s,
		objectProvider:         s.provider,
		AnySpace:               coreSpace,
		derivedIDs:             derivedIDs,
		loadMandatoryObjectsCh: make(chan struct{}),
	}
	go sp.mandatoryObjectsLoad(s.ctx)
	return sp
}

type space struct {
	service        *service
	status         spaceinfo.SpaceInfo
	derivedIDs     threads.DerivedSmartblockIds
	objectProvider objectprovider.ObjectProvider

	*spacecore.AnySpace

	loadMandatoryObjectsCh  chan struct{}
	loadMandatoryObjectsErr error
}

func (s *space) mandatoryObjectsLoad(ctx context.Context) {
	defer close(s.loadMandatoryObjectsCh)

	s.loadMandatoryObjectsErr = s.objectProvider.LoadObjects(ctx, s.Id(), s.derivedIDs.IDs())
	if s.loadMandatoryObjectsErr != nil {
		return
	}
	s.loadMandatoryObjectsErr = s.objectProvider.InstallBundledObjects(ctx, s.Id())
	if s.loadMandatoryObjectsErr != nil {
		return
	}

	// TODO: move to service
	s.loadMandatoryObjectsErr = s.service.indexer.ReindexSpace(s.Id())
	if s.loadMandatoryObjectsErr != nil {
		return
	}
	s.TreeSyncer().StartSync()
}

func (s *space) DerivedIDs() threads.DerivedSmartblockIds {
	<-s.loadMandatoryObjectsCh
	return s.derivedIDs
}

func (s *space) ViewID() string {
	return s.status.ViewID
}

func (s *space) WaitMandatoryObjects(ctx context.Context) (err error) {
	select {
	case <-s.loadMandatoryObjectsCh:
		return s.loadMandatoryObjectsErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
