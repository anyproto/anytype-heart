package space

import (
	"context"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceobject"
	"github.com/anyproto/anytype-heart/space/spaceobject/objectprovider"
)

const CName = "client.space"

var log = logger.NewNamed(CName)

var (
	personalSpaceTypes = []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeHome,
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeWidget,
		smartblock.SmartBlockTypeWorkspace,
		smartblock.SmartBlockTypeAnytypeProfile,
	}
	spaceTypes = []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeHome,
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeWidget,
		smartblock.SmartBlockTypeWorkspace,
	}
)

type spaceIndexer interface {
	ReindexCommonObjects() error
	ReindexSpace(spaceID string) error
}

type bundledObjectsInstaller interface {
	InstallBundledObjects(ctx context.Context, spaceID string, ids []string) ([]string, []*types.Struct, error)
}

type SpaceParams struct {
	IDs           threads.DerivedSmartblockIds
	SpaceObjectID string
}

type SpaceService interface {
	SpaceParams(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error)
	Do(ctx context.Context, spaceID string, perform func(spaceObject spaceobject.SpaceObject) error) error
	Create(ctx context.Context) (spaceObject spaceobject.SpaceObject, err error)

	app.ComponentRunnable
}

type service struct {
	indexer     spaceIndexer
	spaceCore   spacecore.SpaceCoreService
	provider    objectprovider.ObjectProvider
	objectCache objectcache.Cache

	cache      *idCache
	techSpace  *spacecore.AnySpace
	newAccount bool

	repKey uint64
}

func (s *service) SpaceParams(ctx context.Context, spaceID string) (params SpaceParams, err error) {
	params, ok := s.cache.Get(spaceID)
	if ok {
		return
	}
	ids, err := s.provider.DeriveObjectIDs(ctx, spaceID, spaceTypes)
	if err != nil {
		return
	}
	spaceObjID, err := s.deriveSpaceObjectId(ctx, s.techSpace.Id(), spaceID)
	if err != nil {
		return
	}
	s.cache.Set(spaceID, SpaceParams{
		IDs:           ids,
		SpaceObjectID: spaceObjID,
	})
	return
}

func (s *service) Do(ctx context.Context, spaceID string, perform func(spaceObject spaceobject.SpaceObject) error) error {
	params, err := s.SpaceParams(ctx, spaceID)
	if err != nil {
		return err
	}
	spaceObject, err := s.objectCache.GetObject(ctx, domain.FullID{
		ObjectID: params.SpaceObjectID,
		SpaceID:  spaceID,
	})
	if err != nil {
		return err
	}
	spaceObject.Lock()
	defer spaceObject.Unlock()
	return perform(spaceObject.(spaceobject.SpaceObject))
}

func (s *service) Create(ctx context.Context) (spaceObject spaceobject.SpaceObject, err error) {
	space, err := s.spaceCore.Create(ctx, s.repKey)
	if err != nil {
		return
	}
	_, err = s.provider.DeriveObjectIDs(ctx, space.Id(), spaceTypes)
	if err != nil {
		return
	}
	err = s.provider.CreateMandatoryObjects(ctx, space.Id(), spaceTypes)
	if err != nil {
		return
	}
	return s.deriveSpaceObject(ctx, space.Id(), s.techSpace.Id())
}

func (s *service) Init(a *app.App) (err error) {
	s.indexer = app.MustComponent[spaceIndexer](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.objectCache = app.MustComponent[objectcache.Cache](a)
	s.cache = newCache()
	installer := app.MustComponent[bundledObjectsInstaller](a)
	s.provider = objectprovider.NewObjectProvider(s.objectCache, installer)
	s.newAccount = a.MustComponent(config.CName).(*config.Config).NewAccount
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	s.techSpace, err = s.spaceCore.Derive(ctx, spacecore.TechSpaceType)
	if err != nil {
		return
	}
	spaceLoader := spaceLoader{
		spaceCore: s.spaceCore,
		deriver:   s,
		provider:  s.provider,
		cache:     s.objectCache,
		techSpace: s.techSpace,
	}
	if s.newAccount {
		return spaceLoader.CreateSpaces(ctx)
	}
	return spaceLoader.LoadSpaces(ctx)
}

func (s *service) Close(ctx context.Context) (err error) {
	return nil
}

func (s *service) deriveSpaceObject(ctx context.Context, spaceID, targetSpaceID string) (spaceobject.SpaceObject, error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceObject, "")
	if err != nil {
		return nil, err
	}
	obj, err := s.objectCache.DeriveTreeObject(ctx, spaceID, objectcache.TreeDerivationParams{
		Key: uniqueKey,
		InitFunc: func(id string) *editorsb.InitContext {
			return &editorsb.InitContext{Ctx: ctx, SpaceID: spaceID, State: state.NewDoc(id, nil).(*state.State)}
		},
		TargetSpaceID: targetSpaceID,
	})
	if err != nil {
		return nil, err
	}
	return obj.(spaceobject.SpaceObject), nil
}

func (s *service) deriveSpaceObjectId(ctx context.Context, spaceID, targetSpaceID string) (string, error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeSpaceObject, "")
	if err != nil {
		return "", err
	}
	payload, err := s.objectCache.DeriveTreePayload(ctx, spaceID, payloadcreator.PayloadDerivationParams{
		Key:           uniqueKey,
		TargetSpaceID: targetSpaceID,
	})
	if err != nil {
		return "", err
	}
	return payload.RootRawChange.Id, nil
}
