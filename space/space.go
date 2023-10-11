package space

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/objectprovider"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type Space interface {
	commonspace.Space
	DerivedIDs() threads.DerivedSmartblockIds

	WaitMandatoryObjects(ctx context.Context) (err error)

	objectcache.Cache
	objectprovider.ObjectProvider
}

func (s *service) newSpace(ctx context.Context, coreSpace *spacecore.AnySpace) (*space, error) {
	sp := &space{
		service:                s,
		AnySpace:               coreSpace,
		loadMandatoryObjectsCh: make(chan struct{}),
	}
	sp.Cache = objectcache.New(coreSpace, s.accountService, s.objectFactory, s.personalSpaceID, sp)
	sp.ObjectProvider = objectprovider.NewObjectProvider(coreSpace.Id(), s.personalSpaceID, sp.Cache, s.bundledObjectsInstaller)
	var err error
	sp.derivedIDs, err = sp.ObjectProvider.DeriveObjectIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("derive object ids: %w", err)
	}

	// TODO BEGIN RUN ONLY ON CREATE
	// create mandatory objects
	// err = sp.CreateMandatoryObjects(ctx)
	// if err != nil {
	// 	return nil, fmt.Errorf("CreateMandatoryObjects error: %w; spaceId: %v", err, coreSpace.Id())
	// }
	// TODO END RUN ONLY ON CREATE

	go sp.mandatoryObjectsLoad(s.ctx)
	return sp, nil
}

type space struct {
	objectcache.Cache
	objectprovider.ObjectProvider

	service    *service
	status     spaceinfo.SpaceInfo
	derivedIDs threads.DerivedSmartblockIds

	*spacecore.AnySpace

	loadMandatoryObjectsCh  chan struct{}
	loadMandatoryObjectsErr error
}

func (s *space) mandatoryObjectsLoad(ctx context.Context) {
	defer close(s.loadMandatoryObjectsCh)

	s.loadMandatoryObjectsErr = s.LoadObjects(ctx, s.derivedIDs.IDs())
	if s.loadMandatoryObjectsErr != nil {
		return
	}
	s.loadMandatoryObjectsErr = s.InstallBundledObjects(ctx)
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

func (s *space) WaitMandatoryObjects(ctx context.Context) (err error) {
	select {
	case <-s.loadMandatoryObjectsCh:
		return s.loadMandatoryObjectsErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
