package core

import (
	"context"
	"errors"
	"github.com/anytypeio/go-anytype-middleware/core/account"
	"os"
	"runtime/debug"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var log = logging.Logger("anytype-mw-api")

var (
	ErrNotLoggedIn = errors.New("not logged in")
)

type Middleware struct {
	rootPath            string
	pin                 string
	mnemonic            string
	accountSearchCancel context.CancelFunc

	foundAccounts []*model.Account // found local&remote account for the current mnemonic

	EventSender event.Sender

	app *app.App

	m sync.RWMutex
}

func New() *Middleware {
	mw := &Middleware{accountSearchCancel: func() {}}
	return mw
}

func (mw *Middleware) AppShutdown(request *pb.RpcAppShutdownRequest) *pb.RpcAppShutdownResponse {
	mw.m.Lock()
	defer mw.m.Unlock()
	mw.stop()
	return &pb.RpcAppShutdownResponse{
		Error: &pb.RpcAppShutdownResponseError{
			Code: pb.RpcAppShutdownResponseError_NULL,
		},
	}
}

func (mw *Middleware) AppSetDeviceState(req *pb.RpcAppSetDeviceStateRequest) *pb.RpcAppSetDeviceStateResponse {
	mw.app.SetDeviceState(int(req.DeviceState))

	return &pb.RpcAppSetDeviceStateResponse{
		Error: &pb.RpcAppSetDeviceStateResponseError{
			Code: pb.RpcAppSetDeviceStateResponseError_NULL,
		},
	}
}

func (mw *Middleware) getBlockService() (bs block.Service, err error) {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.app != nil {
		return mw.app.MustComponent(block.CName).(block.Service), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) getAccountService() (a account.Service, err error) {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.app != nil {
		return mw.app.MustComponent(account.CName).(account.Service), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) doBlockService(f func(bs block.Service) error) (err error) {
	bs, err := mw.getBlockService()
	if err != nil {
		return
	}
	return f(bs)
}

func (mw *Middleware) doAccountService(f func(a account.Service) error) (err error) {
	bs, err := mw.getAccountService()
	if err != nil {
		return
	}
	return f(bs)
}

// Stop stops the anytype node and HTTP gateway
func (mw *Middleware) stop() error {
	if mw != nil && mw.app != nil {
		err := mw.app.Close()
		if err != nil {
			log.Warnf("error while stop anytype: %v", err)
		}

		mw.app = nil
		mw.accountSearchCancel()
	}
	return nil
}

func (mw *Middleware) GetAnytype() core.Service {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.app != nil {
		return mw.app.MustComponent("anytype").(core.Service)
	}
	return nil
}

func (mw *Middleware) GetApp() *app.App {
	mw.m.RLock()
	defer mw.m.RUnlock()
	return mw.app
}

func (mw *Middleware) OnPanic(v interface{}) {
	stack := debug.Stack()
	os.Stderr.Write(stack)
	log.With("stack", stack).Errorf("panic recovered: %v", v)
}

func init() {
	// let leave it here so it will work in all types of distribution and tests
	logging.SetVersion(app.GitSummary)
}
