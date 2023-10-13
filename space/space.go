package space

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/headsync"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/objectprovider"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type Space interface {
	Id() string
	TreeBuilder() objecttreebuilder.TreeBuilder
	DebugAllHeads() []headsync.TreeHeads
	DeleteTree(ctx context.Context, id string) (err error)
	StoredIds() []string
	Storage() spacestorage.SpaceStorage

	DerivedIDs() threads.DerivedSmartblockIds

	WaitMandatoryObjects(ctx context.Context) (err error)

	Do(objectId string, apply func(sb smartblock.SmartBlock) error) error
	objectcache.Cache
	objectprovider.ObjectProvider

	GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error)

	Close(ctx context.Context) error
}

type space struct {
	objectcache.Cache
	objectprovider.ObjectProvider

	service    *service
	status     spaceinfo.SpaceInfo
	derivedIDs threads.DerivedSmartblockIds
	installer  bundledObjectsInstaller

	commonspace.Space

	loadMandatoryObjectsCh  chan struct{}
	loadMandatoryObjectsErr error
}

func (s *service) newSpace(ctx context.Context, coreSpace *spacecore.AnySpace, justCreated bool) (*space, error) {
	sp := &space{
		service:                s,
		Space:                  coreSpace,
		loadMandatoryObjectsCh: make(chan struct{}),
		installer:              s.bundledObjectsInstaller,
	}
	sp.Cache = objectcache.New(s.accountService, s.objectFactory, s.personalSpaceID, sp)
	sp.ObjectProvider = objectprovider.NewObjectProvider(coreSpace.Id(), s.personalSpaceID, sp.Cache)
	var err error
	sp.derivedIDs, err = sp.ObjectProvider.DeriveObjectIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("derive object ids: %w", err)
	}
	if justCreated {
		err = sp.ObjectProvider.CreateMandatoryObjects(ctx, sp)
		if err != nil {
			return nil, fmt.Errorf("create mandatory objects: %w", err)
		}
	}
	go sp.mandatoryObjectsLoad(s.ctx)
	return sp, nil
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
	s.loadMandatoryObjectsErr = s.service.indexer.ReindexSpace(s)
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

func (s *space) Do(objectId string, apply func(sb smartblock.SmartBlock) error) error {
	sb, err := s.GetObject(context.Background(), objectId)
	if err != nil {
		return err
	}
	sb.Lock()
	defer sb.Unlock()
	return apply(sb)
}

func (s *space) GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, key.String())
	if err != nil {
		return "", err
	}
	return s.DeriveObjectID(ctx, uk)
}

func (s *space) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key.String())
	if err != nil {
		return "", err
	}
	return s.DeriveObjectID(ctx, uk)
}

func (s *space) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	err := s.Cache.Close(ctx)
	if err != nil {
		return err
	}
	return nil
}
