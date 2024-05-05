package marketplacespace

import (
	"context"
	"fmt"

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

func NewSpaceController(a *app.App, personalSpaceId string) spacecontroller.SpaceController {
	return &spaceController{
		app:             a,
		personalSpaceId: personalSpaceId,
	}
}

type spaceController struct {
	app             *app.App
	personalSpaceId string
	vs              clientspace.Space
}

func (s *spaceController) Start(ctx context.Context) (err error) {
	indexer := app.MustComponent[dependencies.SpaceIndexer](s.app)
	s.vs = clientspace.NewVirtualSpace(
		addr.AnytypeMarketplaceWorkspace,
		clientspace.VirtualSpaceDeps{
			ObjectFactory:   app.MustComponent[objectcache.ObjectFactory](s.app),
			AccountService:  app.MustComponent[accountservice.Service](s.app),
			PersonalSpaceId: s.personalSpaceId,
			Indexer:         app.MustComponent[dependencies.SpaceIndexer](s.app),
			Installer:       app.MustComponent[dependencies.BundledObjectsInstaller](s.app),
			TypePrefix:      addr.BundledObjectTypeURLPrefix,
			RelationPrefix:  addr.BundledRelationURLPrefix,
		})
	vsService := app.MustComponent[virtualspaceservice.VirtualSpaceService](s.app)
	bsService := app.MustComponent[dependencies.BuiltinTemplateService](s.app)
	err = vsService.RegisterVirtualSpace(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return fmt.Errorf("register virtual space: %w", err)
	}
	err = bsService.RegisterBuiltinTemplates(s.vs)
	if err != nil {
		return fmt.Errorf("register builtin templates: %w", err)
	}
	err = indexer.ReindexMarketplaceSpace(s.vs)
	if err != nil {
		return fmt.Errorf("reindex marketplace space: %w", err)
	}
	return err
}

func (s *spaceController) Mode() mode.Mode {
	return mode.ModeLoading
}

func (s *spaceController) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
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
