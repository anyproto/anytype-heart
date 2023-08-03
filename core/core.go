package core

import (
	"context"
	"errors"
	"os"
	"runtime/debug"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/core/account"
	"github.com/anyproto/anytype-heart/core/event"
)

var log = logging.Logger("anytype-mw-api")

var (
	ErrNotLoggedIn = errors.New("not logged in")
)

type Middleware struct {
	accountService *account.Service

	m sync.RWMutex
}

func New() *Middleware {
	mw := &Middleware{
		accountService: account.New(),
	}
	return mw
}

func (mw *Middleware) AppShutdown(cctx context.Context, request *pb.RpcAppShutdownRequest) *pb.RpcAppShutdownResponse {
	mw.m.Lock()
	defer mw.m.Unlock()
	mw.accountService.Stop()
	return &pb.RpcAppShutdownResponse{
		Error: &pb.RpcAppShutdownResponseError{
			Code: pb.RpcAppShutdownResponseError_NULL,
		},
	}
}

func (mw *Middleware) AppSetDeviceState(cctx context.Context, req *pb.RpcAppSetDeviceStateRequest) *pb.RpcAppSetDeviceStateResponse {
	mw.accountService.GetApp().SetDeviceState(int(req.DeviceState))

	return &pb.RpcAppSetDeviceStateResponse{
		Error: &pb.RpcAppSetDeviceStateResponseError{
			Code: pb.RpcAppSetDeviceStateResponseError_NULL,
		},
	}
}

func (mw *Middleware) getBlockService() (bs *block.Service, err error) {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.accountService.GetApp() != nil {
		return mw.accountService.GetApp().MustComponent(block.CName).(*block.Service), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) getRelationService() (rs relation.Service, err error) {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.accountService.GetApp() != nil {
		return mw.accountService.GetApp().MustComponent(relation.CName).(relation.Service), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) getAccountService() (a space.Service, err error) {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.accountService.GetApp() != nil {
		return mw.accountService.GetApp().MustComponent(space.CName).(space.Service), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) doBlockService(f func(bs *block.Service) error) (err error) {
	bs, err := mw.getBlockService()
	if err != nil {
		return
	}
	return f(bs)
}

func (mw *Middleware) doCollectionService(f func(bs *collection.Service) error) (err error) {
	mw.m.RLock()
	a := mw.accountService.GetApp()
	mw.m.RUnlock()
	if a == nil {
		return ErrNotLoggedIn
	}
	return f(app.MustComponent[*collection.Service](a))
}

func getService[T any](mw *Middleware) T {
	mw.m.RLock()
	a := mw.accountService.GetApp()
	mw.m.RUnlock()
	requireApp(a)
	return app.MustComponent[T](a)
}

func requireApp(a *app.App) {
	if a == nil {
		panic(ErrNotLoggedIn)
	}
}

func (mw *Middleware) doRelationService(f func(rs relation.Service) error) (err error) {
	rs, err := mw.getRelationService()
	if err != nil {
		return
	}
	return f(rs)
}

func (mw *Middleware) doAccountService(f func(a space.Service) error) (err error) {
	bs, err := mw.getAccountService()
	if err != nil {
		return
	}
	return f(bs)
}

func (mw *Middleware) GetAnytype() core.Service {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.accountService.GetApp() != nil {
		return mw.accountService.GetApp().MustComponent("anytype").(core.Service)
	}
	return nil
}

func (mw *Middleware) GetApp() *app.App {
	mw.m.RLock()
	defer mw.m.RUnlock()
	return mw.accountService.GetApp()
}

func (mw *Middleware) OnPanic(v interface{}) {
	stack := debug.Stack()
	os.Stderr.Write(stack)
	log.With("stack", stack).Errorf("panic recovered: %v", v)
}

func (mw *Middleware) SetEventSender(sender event.Sender) {
	mw.accountService.SetEventSender(sender)
}
