package aclobjectmanager

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
)

const CName = "common.components.aclobjectmanager"

type AclObjectManager interface {
	app.ComponentRunnable
}

type aclObjectManager struct {
	ctx         context.Context
	cancel      context.CancelFunc
	wait        chan struct{}
	waitLoad    chan struct{}
	sp          clientspace.Space
	loadErr     error
	spaceLoader spaceloader.SpaceLoader
}

func (a *aclObjectManager) Init(app *app.App) (err error) {
	a.spaceLoader = app.MustComponent(spaceloader.CName).(spaceloader.SpaceLoader)
	a.waitLoad = make(chan struct{})
	a.wait = make(chan struct{})
}

func (a *aclObjectManager) Name() (name string) {
	return CName
}

func (a *aclObjectManager) Run(ctx context.Context) (err error) {
	//TODO implement me
	panic("implement me")
}

func (a *aclObjectManager) Close(ctx context.Context) (err error) {
	a.cancel()
	<-a.wait
	return
}

func (a *aclObjectManager) loadSpace() {
	a.sp, a.loadErr = a.spaceLoader.WaitLoad(a.ctx)
	close(a.waitLoad)
}

func (a *aclObjectManager) process() {
	defer close(a.wait)
	select {
	case <-a.ctx.Done():
		return
	case <-a.waitLoad:
		if a.loadErr != nil {
			return
		}
		break
	}
	a.createParticipantObjects()
}

func (a *aclObjectManager) createParticipantObjects() {
	sp := a.sp.CommonSpace()
	sp.Acl().SetAclUpdater()
	sp.Acl().RLock()
	defer sp.Acl().RUnlock()
}
