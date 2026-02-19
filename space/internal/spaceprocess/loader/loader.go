package loader

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclnotifications"
	"github.com/anyproto/anytype-heart/space/internal/components/aclobjectmanager"
	"github.com/anyproto/anytype-heart/space/internal/components/builder"
	"github.com/anyproto/anytype-heart/space/internal/components/migration"
	"github.com/anyproto/anytype-heart/space/internal/components/participantwatcher"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
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
	SpaceId         string
	IsPersonal      bool
	OwnerMetadata   []byte
	AdditionalComps []app.Component
	GuestKey        crypto.PrivKey
}

func New(app *app.App, params Params) Loader {
	child := app.ChildApp()
	child.Register(builder.New(params.GuestKey)).
		Register(spaceloader.New(params.IsPersonal, false)).
		Register(aclnotifications.NewAclNotificationSender()).
		Register(aclobjectmanager.New(params.OwnerMetadata, params.GuestKey)).
		Register(participantwatcher.New()).
		Register(migration.New())
	for _, comp := range params.AdditionalComps {
		child.Register(comp)
	}
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

// wait load starts this spaceloader sub app component
func (l *loader) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	spaceLoader := app.MustComponent[spaceloader.SpaceLoader](l.app)
	return spaceLoader.WaitLoad(ctx)
}
