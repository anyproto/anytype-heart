package loader

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/builder"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
)

type loader struct {
	app *app.App
}

type LoadWaiter interface {
	WaitLoad(ctx context.Context) (sp clientspace.Space, err error)
}

type Loader interface {
	mode.Process
	LoadWaiter
}

type Params struct {
	SpaceId             string
	Status              spacestatus.SpaceStatus
	StopIfMandatoryFail bool
}

func New(app *app.App, params Params) Loader {
	child := app.ChildApp()
	child.Register(params.Status).
		Register(builder.New()).
		Register(spaceloader.New(params.StopIfMandatoryFail))
	return &loader{
		app: child,
	}
}

func (l *loader) Start(ctx context.Context) error {
	return l.app.Start(ctx)
}

func (l *loader) Close(ctx context.Context) error {
	return l.app.Close(ctx)
}

func (l *loader) CanTransition(next mode.Mode) bool {
	return true
}

func (l *loader) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	spaceLoader := app.MustComponent[spaceloader.SpaceLoader](l.app)
	return spaceLoader.WaitLoad(ctx)
}
