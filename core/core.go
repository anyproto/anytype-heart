package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	utildebug "github.com/anyproto/anytype-heart/util/debug"
)

var log = logging.Logger("anytype-mw-api")

var (
	ErrNotLoggedIn = errors.New("not logged in")
)

type Middleware struct {
	applicationService *application.Service
}

func New() *Middleware {
	mw := &Middleware{
		applicationService: application.New(),
	}
	return mw
}

func (mw *Middleware) AppShutdown(cctx context.Context, request *pb.RpcAppShutdownRequest) *pb.RpcAppShutdownResponse {
	mw.applicationService.Stop()
	return &pb.RpcAppShutdownResponse{
		Error: &pb.RpcAppShutdownResponseError{
			Code: pb.RpcAppShutdownResponseError_NULL,
		},
	}
}

func (mw *Middleware) AppSetDeviceState(cctx context.Context, req *pb.RpcAppSetDeviceStateRequest) *pb.RpcAppSetDeviceStateResponse {
	mw.applicationService.GetApp().SetDeviceState(int(req.DeviceState))

	return &pb.RpcAppSetDeviceStateResponse{
		Error: &pb.RpcAppSetDeviceStateResponseError{
			Code: pb.RpcAppSetDeviceStateResponseError_NULL,
		},
	}
}

func (mw *Middleware) getBlockService() (bs *block.Service, err error) {
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent(block.CName).(*block.Service), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) getRelationService() (rs relation.Service, err error) {
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent(relation.CName).(relation.Service), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) getAccountService() (a space.Service, err error) {
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent(space.CName).(space.Service), nil
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
	a := mw.applicationService.GetApp()
	if a == nil {
		return ErrNotLoggedIn
	}
	return f(app.MustComponent[*collection.Service](a))
}

func getService[T any](mw *Middleware) T {
	a := mw.applicationService.GetApp()
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
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent("anytype").(core.Service)
	}
	return nil
}

func (mw *Middleware) GetApp() *app.App {
	return mw.applicationService.GetApp()
}

func (mw *Middleware) OnPanic(v interface{}) {
	stack := debug.Stack()
	os.Stderr.Write(stack)
	log.With("stack", stack).Errorf("panic recovered: %v", v)
}

func (mw *Middleware) SetEventSender(sender event.Sender) {
	mw.applicationService.SetEventSender(sender)
}

func (mw *Middleware) SaveGoroutinesStack(path string) (err error) {
	if path == "" {
		a := mw.GetApp()
		if a == nil {
			return fmt.Errorf("failed to save stacktrace: need to start app first")
		}
		wl := a.Component(wallet.CName)
		if wl == nil {
			return fmt.Errorf("failed to save stacktrace: need to start wallet first")
		}
		path = wl.(wallet.Wallet).RepoPath()
	}
	return utildebug.SaveStackToRepo(path, true)
}
