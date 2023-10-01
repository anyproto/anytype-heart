package space

import (
	"context"
	"github.com/anyproto/any-sync/commonspace"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

func newSpace(s *service, coreSpace *spacecore.AnySpace) *space {
	sp := &space{
		service:                s,
		objectProvider:         s.provider,
		AnySpace:               coreSpace,
		loadMandatoryObjectsCh: make(chan struct{}),
	}
	go sp.mandatoryObjectsLoad(context.TODO())
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
	var sbTypes []coresb.SmartBlockType
	if s.service.IsPersonal(s.Id()) {
		sbTypes = threads.PersonalSpaceTypes
	} else {
		sbTypes = threads.SpaceTypes
	}
	ids, err := s.objectProvider.DeriveObjectIDs(ctx, s.Id(), sbTypes)
	if err != nil {
		s.loadMandatoryObjectsErr = err
		return
	}
	s.derivedIDs = ids
	s.loadMandatoryObjectsErr = s.objectProvider.LoadObjects(ctx, s.Id(), ids.IDs())
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
