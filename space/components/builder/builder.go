package builder

import (
	"context"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/components/dependencies"
	"github.com/anyproto/anytype-heart/space/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "client.common.builder"

type SpaceBuilder interface {
	app.Component
	BuildSpace(ctx context.Context, justCreated bool) (clientspace.Space, error)
}

func New() SpaceBuilder {
	return &spaceBuilder{}
}

type spaceBuilder struct {
	indexer         dependencies.SpaceIndexer
	installer       dependencies.BundledObjectsInstaller
	spaceCore       spacecore.SpaceCoreService
	techSpace       techspace.TechSpace
	accountService  accountservice.Service
	objectFactory   objectcache.ObjectFactory
	storageService  storage.ClientStorage
	personalSpaceId string
	state           *spacestatus.SpaceStatus

	ctx    context.Context
	cancel context.CancelFunc
}

func (b *spaceBuilder) Init(a *app.App) (err error) {
	b.ctx, b.cancel = context.WithCancel(context.Background())
	b.indexer = app.MustComponent[dependencies.SpaceIndexer](a)
	b.installer = app.MustComponent[dependencies.BundledObjectsInstaller](a)
	b.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	b.techSpace = app.MustComponent[techspace.TechSpace](a)
	b.accountService = app.MustComponent[accountservice.Service](a)
	b.objectFactory = app.MustComponent[objectcache.ObjectFactory](a)
	b.storageService = app.MustComponent[storage.ClientStorage](a)
	b.personalSpaceId, err = b.spaceCore.DeriveID(context.Background(), spacecore.SpaceType)
	return
}

func (b *spaceBuilder) Name() (name string) {
	return CName
}

func (b *spaceBuilder) Run(ctx context.Context) (err error) {
	return nil
}

func (b *spaceBuilder) Close(ctx context.Context) (err error) {
	b.cancel()
	return nil
}

func (b *spaceBuilder) BuildSpace(ctx context.Context, justCreated bool) (clientspace.Space, error) {
	coreSpace, err := b.spaceCore.Get(ctx, b.state.SpaceId)
	if err != nil {
		return nil, err
	}
	deps := clientspace.SpaceDeps{
		Indexer:         b.indexer,
		Installer:       b.installer,
		CommonSpace:     coreSpace,
		ObjectFactory:   b.objectFactory,
		AccountService:  b.accountService,
		PersonalSpaceId: b.personalSpaceId,
		LoadCtx:         b.ctx,
		JustCreated:     justCreated,
	}
	return clientspace.BuildSpace(ctx, deps)
}
