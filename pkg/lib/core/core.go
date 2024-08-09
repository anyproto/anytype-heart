package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-core")

const (
	CName = "anytype"
)

type Service interface {
	Stop() error
	IsStarted() bool
	app.ComponentRunnable
}

var _ app.Component = (*Anytype)(nil)

var _ Service = (*Anytype)(nil)

type Anytype struct {
	lock             sync.RWMutex
	isStarted        bool // use under the lock
	shutdownStartsCh chan struct {
	} // closed when node shutdown starts

}

func New() *Anytype {
	return &Anytype{
		shutdownStartsCh: make(chan struct{}),
	}
}

func (a *Anytype) Init(ap *app.App) (err error) {
	return
}

func (a *Anytype) Name() string {
	return CName
}

func (a *Anytype) Run(ctx context.Context) (err error) {
	a.start()
	return nil
}

// TODO: refactor to call tech space
func (a *Anytype) IsStarted() bool {
	return true
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.isStarted
}

func (a *Anytype) HandlePeerFound(p peer.AddrInfo) {
	// TODO: [MR] mdns
}

func (a *Anytype) start() {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.isStarted {
		return
	}

	a.isStarted = true
}

func (a *Anytype) Close(ctx context.Context) (err error) {
	metrics.Service.Close()
	return a.Stop()
}

func (a *Anytype) Stop() error {
	fmt.Printf("stopping the library...\n")
	defer fmt.Println("library has been successfully stopped")
	a.lock.Lock()
	defer a.lock.Unlock()
	a.isStarted = false

	if a.shutdownStartsCh != nil {
		close(a.shutdownStartsCh)
	}

	return nil
}
