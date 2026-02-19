package builder

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/clientspace"
	dependencies2 "github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.components.builder"

type SpaceBuilder interface {
	app.Component
	BuildSpace(ctx context.Context, disableRemoteLoad bool) (clientspace.Space, error)
}

func New(customAccountKey crypto.PrivKey) SpaceBuilder {
	return &spaceBuilder{
		customAccountKey: customAccountKey,
	}
}

type spaceBuilder struct {
	indexer          dependencies2.SpaceIndexer
	installer        dependencies2.BundledObjectsInstaller
	spaceCore        spacecore.SpaceCoreService
	accountService   accountservice.Service
	objectFactory    objectcache.ObjectFactory
	storageService   storage.ClientStorage
	personalSpaceId  string
	customAccountKey crypto.PrivKey
	status           spacestatus.SpaceStatus

	ctx    context.Context
	cancel context.CancelFunc
}

func (b *spaceBuilder) Init(a *app.App) (err error) {
	b.ctx, b.cancel = context.WithCancel(context.Background())
	b.status = app.MustComponent[spacestatus.SpaceStatus](a)
	b.indexer = app.MustComponent[dependencies2.SpaceIndexer](a)
	b.installer = app.MustComponent[dependencies2.BundledObjectsInstaller](a)
	b.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	b.accountService = app.MustComponent[accountservice.Service](a)
	b.objectFactory = app.MustComponent[objectcache.ObjectFactory](a)
	b.storageService = app.MustComponent[storage.ClientStorage](a)
	b.personalSpaceId, err = b.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeRegular)
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

func (b *spaceBuilder) BuildSpace(ctx context.Context, disableRemoteLoad bool) (clientspace.Space, error) {
	if disableRemoteLoad {
		st, err := b.storageService.WaitSpaceStorage(ctx, b.status.SpaceId())
		if err != nil {
			return nil, err
		}
		err = st.Close(ctx)
		if err != nil {
			return nil, err
		}
	}
	if b.customAccountKey != nil {
		ctx = context.WithValue(ctx, spacecore.OptsKey, spacecore.Opts{SignKey: b.customAccountKey})
	}
	coreSpace, err := b.spaceCore.Get(ctx, b.status.SpaceId())
	if err != nil {
		return nil, err
	}
	if disableRemoteLoad {
		coreSpace.TreeSyncer().StopSync()
	}
	deps := clientspace.SpaceDeps{
		Indexer:          b.indexer,
		Installer:        b.installer,
		CommonSpace:      coreSpace,
		ObjectFactory:    b.objectFactory,
		AccountService:   b.accountService,
		PersonalSpaceId:  b.personalSpaceId,
		StorageService:   b.storageService,
		SpaceCore:        b.spaceCore,
		LoadCtx:          b.ctx,
		KeyValueObserver: coreSpace.KeyValueObserver(),
	}
	space, err := clientspace.BuildSpace(ctx, deps)
	if err != nil {
		return nil, err
	}

	acc := spaceinfo.AccessTypePrivate
	if space.Id() == b.personalSpaceId {
		acc = spaceinfo.AccessTypePersonal
	}
	err = b.status.SetAccessType(acc)
	if err != nil {
		return nil, fmt.Errorf("set access type: %w", err)
	}

	return space, nil
}
