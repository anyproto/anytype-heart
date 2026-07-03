package spacev2

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
)

const presetLoaderCName = "client.spacev2.presetloader"

// presetLoader satisfies the spaceloader.SpaceLoader seam that the reused
// post-load components (aclobjectmanager, migration, personalmigration) block
// on. In v2 the space is fully built before the pipeline app starts, so
// WaitLoad returns immediately. Close deliberately does NOT close the space —
// the backend owns residency and closes it after the pipeline app.
type presetLoader struct {
	sp clientspace.Space
}

var _ spaceloader.SpaceLoader = (*presetLoader)(nil)

func newPresetLoader(sp clientspace.Space) *presetLoader {
	return &presetLoader{sp: sp}
}

func (p *presetLoader) Init(a *app.App) error { return nil }

func (p *presetLoader) Name() string { return presetLoaderCName }

func (p *presetLoader) Run(ctx context.Context) error { return nil }

func (p *presetLoader) Close(ctx context.Context) error { return nil }

func (p *presetLoader) WaitLoad(ctx context.Context) (clientspace.Space, error) {
	return p.sp, nil
}
