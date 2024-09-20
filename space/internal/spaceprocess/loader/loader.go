package loader

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclnotifications"
	"github.com/anyproto/anytype-heart/space/internal/components/aclobjectmanager"
	"github.com/anyproto/anytype-heart/space/internal/components/builder"
	"github.com/anyproto/anytype-heart/space/internal/components/invitemigrator"
	"github.com/anyproto/anytype-heart/space/internal/components/migration"
	"github.com/anyproto/anytype-heart/space/internal/components/participantwatcher"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/components/aclindexcleaner"
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
	WaitMigrations(ctx context.Context) error
}

type Params struct {
	SpaceId       string
	IsPersonal    bool
	OwnerMetadata []byte
}

func New(app *app.App, params Params) Loader {
	child := app.ChildApp()
	child.Register(aclindexcleaner.New()).
		Register(builder.New()).
		Register(spaceloader.New(params.IsPersonal, false)).
		Register(aclnotifications.NewAclNotificationSender()).
		Register(aclobjectmanager.New(params.OwnerMetadata)).
		Register(invitemigrator.New()).
		Register(participantwatcher.New()).
		Register(migration.New(params.IsPersonal))
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

func (l *loader) WaitMigrations(ctx context.Context) error {
	migrator := app.MustComponent[*migration.Runner](l.app)
	return migrator.Wait(ctx)
}
