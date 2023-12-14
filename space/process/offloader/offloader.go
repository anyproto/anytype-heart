package offloader

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/components/spaceoffloader"
	"github.com/anyproto/anytype-heart/space/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/process/modechanger"
)

type Offloader interface {
	modechanger.Process
	WaitOffload(ctx context.Context) error
}

type Params struct {
	Status *spacestatus.SpaceStatus
}

type offloader struct {
	app            *app.App
	spaceOffloader spaceoffloader.SpaceOffloader
}

func New(app *app.App, params Params) Offloader {
	child := app.ChildApp()
	so := spaceoffloader.New()
	child.Register(params.Status).
		Register(so)
	return &offloader{
		app:            child,
		spaceOffloader: so,
	}
}

func (o *offloader) Close(ctx context.Context) (err error) {
	return o.app.Close(ctx)
}

func (o *offloader) Start(ctx context.Context) error {
	return o.app.Start(ctx)
}

func (o *offloader) CanTransition(next modechanger.Mode) bool {
	return false
}

func (o *offloader) WaitOffload(ctx context.Context) error {
	return o.spaceOffloader.WaitOffload(ctx)
}
