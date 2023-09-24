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
	Run(ctx context.Context) error
	Close() error
	WaitLoad() error
}

type bundledObjectsInstaller interface {
	InstallBundledObjects(ctx context.Context, spaceID string, ids []string) ([]string, []*types.Struct, error)
}

type Deps struct {
	Installer  bundledObjectsInstaller
	Cache      objectcache.Cache
	SpaceCore  spacecore.SpaceCoreService
	SpaceID    string
	IsPersonal bool
}

func NewSpaceObject(id string, deps Deps) SpaceObject {
	return &spaceObject{
		id:             id,
		spaceID:        deps.SpaceID,
		cache:          deps.Cache,
		spaceCore:      deps.SpaceCore,
		objectProvider: objectprovider.NewObjectProvider(deps.Cache, deps.Installer),
		loadWaiter:     make(chan struct{}),
		isPersonal:     deps.IsPersonal,
	}
}

type spaceObject struct {
	id             string
	spaceID        string
	cache          objectcache.Cache
	spaceCore      spacecore.SpaceCoreService
	objectProvider objectprovider.ObjectProvider
	ctx            context.Context
	cancel         context.CancelFunc
	isPersonal     bool
	loadWaiter     chan struct{}
	loadErr        error
	derivedIds     threads.DerivedSmartblockIds
	derivedLock    sync.Mutex
}

func (s *spaceObject) Run(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
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
	if s.isPersonal {
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
	if err != nil {
		return
	}
	s.loadErr = s.objectProvider.InstallBundledObjects(s.ctx, s.spaceID)
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
