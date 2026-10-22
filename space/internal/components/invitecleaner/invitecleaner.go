package invitecleaner

/*
Name: Revoked Invite Cleaner
Scope: space

## Responsibility
- Triggers the deletion of the space's revoked invite files from the file node, once per space.

## Background Tasks
- process: waits for space load, then a jittered delay, then runs the cleanup unless the spaceview
  says it already ran.

## Documentation
The work itself lives in core/invitecleanup, which cannot be imported from here: it depends on
core/invitestore, which depends on the space package. It is resolved through the cleaner interface
below, which app.MustComponent type-asserts against the registered components.

The outcome is recorded on the spaceview, which syncs across the account's devices, so exactly one
device does the work.
*/

import (
	"context"
	"math/rand"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
)

const CName = "client.components.invitecleaner"

var log = logger.NewNamed(CName)

// The cleanup waits somewhere in this range before it starts, which keeps it off the critical path
// of a freshly loaded space. The wait is drawn per space rather than fixed: an account can have
// hundreds of spaces, and they all load at once, so a fixed delay would fire every one of their
// coordinator requests in the same second.
var (
	startDelayMin = 20 * time.Second
	startDelayMax = 80 * time.Second
)

func startDelay() time.Duration {
	// nolint: gosec // no need for a cryptographic random here
	return startDelayMin + time.Duration(rand.Int63n(int64(startDelayMax-startDelayMin)))
}

type InviteCleaner interface {
	app.ComponentRunnable
}

// cleaner is core/invitecleanup.Service.
type cleaner interface {
	CleanupSpace(ctx context.Context, sp clientspace.Space) error
}

func New() InviteCleaner {
	return &inviteCleaner{}
}

type inviteCleaner struct {
	ctx     context.Context
	cancel  context.CancelFunc
	wait    chan struct{}
	started bool

	spaceLoader spaceloader.SpaceLoader
	cleaner     cleaner
}

func (i *inviteCleaner) Init(a *app.App) (err error) {
	i.spaceLoader = app.MustComponent[spaceloader.SpaceLoader](a)
	i.cleaner = app.MustComponent[cleaner](a)
	i.wait = make(chan struct{})
	return nil
}

func (i *inviteCleaner) Name() (name string) {
	return CName
}

func (i *inviteCleaner) Run(ctx context.Context) (err error) {
	i.started = true
	i.ctx, i.cancel = context.WithCancel(context.Background())
	go i.process()
	return nil
}

func (i *inviteCleaner) Close(ctx context.Context) (err error) {
	if !i.started {
		return nil
	}
	i.cancel()
	<-i.wait
	return nil
}

func (i *inviteCleaner) process() {
	defer close(i.wait)

	sp, err := i.spaceLoader.WaitLoad(i.ctx)
	if err != nil {
		return
	}
	select {
	case <-i.ctx.Done():
		return
	case <-time.After(startDelay()):
	}

	if err = i.cleaner.CleanupSpace(i.ctx, sp); err != nil {
		log.Warn("cleanup revoked invites", zap.String("spaceId", sp.Id()), zap.Error(err))
	}
}
