package spacev2

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/virtualspaceservice"
)

// TODO: GO-6259 the marketplace space is deprecated; it survives in v2 only
// because objectcreator/installer and template services still Get() it. Unlike
// v1 it is a static registry entry with no state machine — drop this file once
// the consumers migrate off bundled-object installation.
type marketplaceController struct {
	a               *app.App
	personalSpaceId string
	vs              clientspace.Space
	reindexOnce     sync.Once
	indexer         dependencies.SpaceIndexer
}

func newMarketplaceController(a *app.App, personalSpaceId string) *marketplaceController {
	return &marketplaceController{
		a:               a,
		personalSpaceId: personalSpaceId,
		indexer:         app.MustComponent[dependencies.SpaceIndexer](a),
	}
}

func (m *marketplaceController) SpaceId() string { return addr.AnytypeMarketplaceWorkspace }

func (m *marketplaceController) Start(ctx context.Context) error {
	m.vs = clientspace.NewVirtualSpace(
		addr.AnytypeMarketplaceWorkspace,
		clientspace.VirtualSpaceDeps{
			ObjectFactory:   app.MustComponent[objectcache.ObjectFactory](m.a),
			AccountService:  app.MustComponent[accountservice.Service](m.a),
			PersonalSpaceId: m.personalSpaceId,
			Indexer:         m.indexer,
			Installer:       app.MustComponent[dependencies.BundledObjectsInstaller](m.a),
			TypePrefix:      addr.BundledObjectTypeURLPrefix,
			RelationPrefix:  addr.BundledRelationURLPrefix,
		})
	vsService := app.MustComponent[virtualspaceservice.VirtualSpaceService](m.a)
	if err := vsService.RegisterVirtualSpace(addr.AnytypeMarketplaceWorkspace); err != nil {
		return fmt.Errorf("register virtual space: %w", err)
	}
	return nil
}

func (m *marketplaceController) Mode() Mode { return ModeLoading }

func (m *marketplaceController) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	m.reindexOnce.Do(func() {
		err = m.indexer.ReindexMarketplaceSpace(m.vs)
	})
	if err != nil {
		return nil, err
	}
	return m.vs, nil
}

func (m *marketplaceController) Update() error { return nil }

func (m *marketplaceController) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	return nil
}

func (m *marketplaceController) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	return nil
}

func (m *marketplaceController) GetStatus() spaceinfo.AccountStatus {
	return spaceinfo.AccountStatusUnknown
}

func (m *marketplaceController) GetLocalStatus() spaceinfo.LocalStatus {
	return spaceinfo.LocalStatusOk
}

func (m *marketplaceController) Close(ctx context.Context) error { return nil }
