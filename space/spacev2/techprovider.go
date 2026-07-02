package spacev2

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// techProvider builds the tech space (v1: spacefactory.CreateAndSetTechSpace /
// LoadAndSetTechSpace). Object-cache dependencies are resolved lazily at first
// use so tests can fake the techSpaceProvider seam without assembling the
// object-cache stack; the components are mandatory in the production app.
type techProvider struct {
	a               *app.App
	spaceCore       spacecore.SpaceCoreService
	personalSpaceId string

	// childApp is the app the tech space is registered into; per-space
	// controllers are built with it as parent so they resolve the tech space
	// through the component chain (v1 spacefactory did the same by mutating
	// its own app pointer).
	childApp *app.App
}

func newTechProvider(a *app.App, spaceCore spacecore.SpaceCoreService, personalSpaceId string) *techProvider {
	return &techProvider{a: a, spaceCore: spaceCore, personalSpaceId: personalSpaceId}
}

func (p *techProvider) Create(ctx context.Context) (*clientspace.TechSpace, error) {
	techCoreSpace, err := p.spaceCore.Derive(ctx, spacedomain.SpaceTypeTech)
	if err != nil {
		return nil, fmt.Errorf("derive tech space: %w", err)
	}
	return p.run(techCoreSpace, true)
}

func (p *techProvider) Load(ctx context.Context) (*clientspace.TechSpace, error) {
	id, err := p.spaceCore.DeriveID(ctx, spacedomain.SpaceTypeTech)
	if err != nil {
		return nil, fmt.Errorf("derive tech space id: %w", err)
	}
	techCoreSpace, err := p.spaceCore.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get tech space: %w", err)
	}
	ts, err := p.run(techCoreSpace, false)
	if err != nil {
		return nil, err
	}
	if err = app.MustComponent[dependencies.SpaceIndexer](p.a).ReindexSpace(ts); err != nil {
		return nil, fmt.Errorf("reindex tech space: %w", err)
	}
	return ts, nil
}

func (p *techProvider) run(techCoreSpace *spacecore.AnySpace, create bool) (*clientspace.TechSpace, error) {
	techSpace := techspace.New()
	deps := clientspace.TechSpaceDeps{
		CommonSpace:      techCoreSpace,
		ObjectFactory:    app.MustComponent[objectcache.ObjectFactory](p.a),
		AccountService:   app.MustComponent[accountservice.Service](p.a),
		PersonalSpaceId:  p.personalSpaceId,
		Indexer:          app.MustComponent[dependencies.SpaceIndexer](p.a),
		Installer:        app.MustComponent[dependencies.BundledObjectsInstaller](p.a),
		TechSpace:        techSpace,
		KeyValueObserver: techCoreSpace.KeyValueObserver(),
	}
	ts, err := clientspace.NewTechSpace(deps)
	if err != nil {
		return nil, fmt.Errorf("build tech space: %w", err)
	}
	p.childApp = p.a.ChildApp()
	p.childApp.Register(ts)
	if err = ts.Run(techCoreSpace, ts.Cache, create); err != nil {
		return nil, fmt.Errorf("run tech space: %w", err)
	}
	return ts, nil
}
