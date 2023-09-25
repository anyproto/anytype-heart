package spaceobject

import (
	"context"
	"fmt"
	"sync"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceobject/objectprovider"
)

type SpaceObject interface {
	TargetSpaceID() string
	Space() (*spacecore.AnySpace, error)
	TryDerivedIDs() (threads.DerivedSmartblockIds, error)
	Run(spaceID string) error
	Close() error
	WaitLoad() error
}

type bundledObjectsInstaller interface {
	InstallBundledObjects(ctx context.Context, spaceID string, ids []string) ([]string, []*types.Struct, error)
}

type spaceIndexer interface {
	ReindexSpace(spaceID string) error
}

type personalIDProvider interface {
	PersonalSpaceID() string
}

type Deps struct {
	Installer          bundledObjectsInstaller
	Cache              objectcache.Cache
	SpaceCore          spacecore.SpaceCoreService
	Indexer            spaceIndexer
	PersonalIDProvider personalIDProvider
}

func NewSpaceObject(deps Deps) SpaceObject {
	return &spaceObject{
		cache:            deps.Cache,
		spaceCore:        deps.SpaceCore,
		objectProvider:   objectprovider.NewObjectProvider(deps.Cache, deps.Installer),
		indexer:          deps.Indexer,
		loadWaiter:       make(chan struct{}),
		personalProvider: deps.PersonalIDProvider,
	}
}

type spaceObject struct {
	spaceID          string
	cache            objectcache.Cache
	spaceCore        spacecore.SpaceCoreService
	objectProvider   objectprovider.ObjectProvider
	indexer          spaceIndexer
	personalProvider personalIDProvider
	ctx              context.Context
	cancel           context.CancelFunc
	loadWaiter       chan struct{}
	loadErr          error
	derivedIds       threads.DerivedSmartblockIds
	derivedLock      sync.Mutex
}

func (s *spaceObject) Run(spaceID string) error {
	s.spaceID = spaceID
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return nil
}

func (s *spaceObject) Close() error {
	s.cancel()
	<-s.loadWaiter
	return s.loadErr
}

func (s *spaceObject) run() {
	defer close(s.loadWaiter)
	var sbTypes []coresb.SmartBlockType
	if s.personalProvider.PersonalSpaceID() == s.spaceID {
		sbTypes = threads.PersonalSpaceTypes
	} else {
		sbTypes = threads.SpaceTypes
	}
	ids, err := s.objectProvider.DeriveObjectIDs(s.ctx, s.spaceID, sbTypes)
	if err != nil {
		s.loadErr = err
		return
	}
	s.derivedLock.Lock()
	s.derivedIds = ids
	s.derivedLock.Unlock()
	s.loadErr = s.objectProvider.LoadObjects(s.ctx, s.spaceID, ids.IDs())
	if s.loadErr != nil {
		return
	}
	s.loadErr = s.objectProvider.InstallBundledObjects(s.ctx, s.spaceID)
	if s.loadErr != nil {
		return
	}
	s.loadErr = s.indexer.ReindexSpace(s.spaceID)
	return
}

func (s *spaceObject) TargetSpaceID() string {
	return s.spaceID
}

func (s *spaceObject) Space() (*spacecore.AnySpace, error) {
	return s.spaceCore.Get(s.ctx, s.spaceID)
}

func (s *spaceObject) TryDerivedIDs() (threads.DerivedSmartblockIds, error) {
	s.derivedLock.Lock()
	defer s.derivedLock.Unlock()
	if s.derivedIds.IsFilled() {
		return s.derivedIds, nil
	}
	return threads.DerivedSmartblockIds{}, fmt.Errorf("derived ids not ready")
}

func (s *spaceObject) WaitLoad() error {
	<-s.loadWaiter
	return s.loadErr
}
