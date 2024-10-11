package session

import (
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const sessionHookRunner = "sessionHookRunner"

var log = logging.Logger(sessionHookRunner)

type HookRunner interface {
	app.Component
	RegisterHook(hook OnNewSession)
	RunHooks(ctx Context)
}

type OnNewSession func(ctx Context) error

type hookRunner struct {
	onNewSessionHooks []OnNewSession
	sync.Mutex
}

func NewHookRunner() HookRunner {
	return &hookRunner{}
}

func (h *hookRunner) Name() (name string) {
	return sessionHookRunner
}

func (h *hookRunner) Init(a *app.App) (err error) {
	return
}

func (h *hookRunner) RegisterHook(hook OnNewSession) {
	h.Lock()
	defer h.Unlock()
	h.onNewSessionHooks = append(h.onNewSessionHooks, hook)
}

func (h *hookRunner) RunHooks(ctx Context) {
	h.Lock()
	defer h.Unlock()
	for _, hook := range h.onNewSessionHooks {
		err := hook(ctx)
		if err != nil {
			log.Errorf("session hook failed %s", err)
		}
	}
}
