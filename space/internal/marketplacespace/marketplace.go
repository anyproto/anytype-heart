package marketplacespace

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/clientspace"
	dependencies "github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/virtualspaceservice"
)

// TODO: GO-6259 MarketplaceSpace is deprecated and no longer used on clients. We need to get rid of code that installs types/relations from marketplace space and remove it
func NewSpaceController(a *app.App, personalSpaceId string) spacecontroller.SpaceController {
	return &spaceController{
		app:             a,
		personalSpaceId: personalSpaceId,
		indexer:         app.MustComponent[dependencies.SpaceIndexer](a),
	}
}

type spaceController struct {
	app             *app.App
	personalSpaceId string
	vs              clientspace.Space
	reindexOnce     sync.Once
	indexer         dependencies.SpaceIndexer
}

func (s *spaceController) Start(context.Context) (err error) {
	s.vs = clientspace.NewVirtualSpace(
		addr.AnytypeMarketplaceWorkspace,
		clientspace.VirtualSpaceDeps{
			ObjectFactory:   app.MustComponent[objectcache.ObjectFactory](s.app),
			AccountService:  app.MustComponent[accountservice.Service](s.app),
			PersonalSpaceId: s.personalSpaceId,
			Indexer:         s.indexer,
			Installer:       app.MustComponent[dependencies.BundledObjectsInstaller](s.app),
			TypePrefix:      addr.BundledObjectTypeURLPrefix,
			RelationPrefix:  addr.BundledRelationURLPrefix,
		})
	vsService := app.MustComponent[virtualspaceservice.VirtualSpaceService](s.app)
	// bsService := app.MustComponent[dependencies.BuiltinTemplateService](s.app)
	err = vsService.RegisterVirtualSpace(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return fmt.Errorf("register virtual space: %w", err)
	}

	return err
}

func (s *spaceController) Mode() mode.Mode {
	return mode.ModeLoading
}

func (s *spaceController) WaitLoad(context.Context) (sp clientspace.Space, err error) {
	s.reindexOnce.Do(func() {
		// TODO: GO-3557 Need to confirm moving ReindexMarketplaceSpace from Start to WaitLoad with mcrakhman
		err = s.indexer.ReindexMarketplaceSpace(s.vs)
	})
	if err != nil {
		return nil, err
	}
	return s.vs, nil
}

func (s *spaceController) Current() any {
	return s
}

func (s *spaceController) SpaceId() string {
	return addr.AnytypeMarketplaceWorkspace
}

func (s *spaceController) Update() error {
	return nil
}

func (s *spaceController) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	return nil
}

func (s *spaceController) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	return nil
}

func (s *spaceController) Close(ctx context.Context) error {
	return nil
}

func (s *spaceController) GetStatus() spaceinfo.AccountStatus {
	return spaceinfo.AccountStatusUnknown
}

func (s *spaceController) GetLocalStatus() spaceinfo.LocalStatus {
	return spaceinfo.LocalStatusOk
}
