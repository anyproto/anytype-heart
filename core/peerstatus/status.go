package peerstatus

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

const CName = "core.syncstatus.p2p"

var log = logging.Logger(CName)

type Status int32

var ErrClosed = errors.New("component is closing")

const (
	Unknown      Status = 0
	Connected    Status = 1
	NotPossible  Status = 2
	NotConnected Status = 3
	Restricted   Status = 4
)

func (s Status) ToPb() pb.EventP2PStatusStatus {
	switch s {
	case Connected:
		return pb.EventP2PStatus_Connected
	case NotConnected:
		return pb.EventP2PStatus_NotConnected
	case NotPossible:
		return pb.EventP2PStatus_NotPossible
	case Restricted:
		return pb.EventP2PStatus_Restricted

	}
	// default status is NotConnected
	return pb.EventP2PStatus_NotConnected
}

type LocalDiscoveryHook interface {
	app.Component
	RegisterDiscoveryPossibilityHook(hook func(state localdiscovery.DiscoveryPossibility))
}

type PeerToPeerStatus interface {
	app.ComponentRunnable
	RegisterSpace(spaceId string)
	UnregisterSpace(spaceId string)
}

type spaceStatus struct {
	status           Status
	connectionsCount int64
}

type p2pStatus struct {
	spaceIds      map[string]*spaceStatus
	eventSender   event.Sender
	contextCancel context.CancelFunc
	ctx           context.Context
	peerStore     peerstore.PeerStore

	sync.Mutex
	p2pLastState   localdiscovery.DiscoveryPossibility
	workerFinished chan struct{}
	refreshSpaceId chan string

	peersConnectionPool pool.Pool
}

func New() PeerToPeerStatus {
	p2pStatusService := &p2pStatus{
		workerFinished: make(chan struct{}),
		refreshSpaceId: make(chan string),
		spaceIds:       make(map[string]*spaceStatus),
	}

	return p2pStatusService
}

func (p *p2pStatus) Init(a *app.App) (err error) {
	p.eventSender = app.MustComponent[event.Sender](a)
	p.peerStore = app.MustComponent[peerstore.PeerStore](a)
	p.peersConnectionPool = app.MustComponent[pool.Service](a)
	localDiscoveryHook := app.MustComponent[LocalDiscoveryHook](a)
	sessionHookRunner := app.MustComponent[session.HookRunner](a)
	localDiscoveryHook.RegisterDiscoveryPossibilityHook(p.setNotPossibleStatus)
	sessionHookRunner.RegisterHook(p.sendStatusForNewSession)
	p.ctx, p.contextCancel = context.WithCancel(context.Background())

	// we need to update status for all spaces that were either added or removed to some local peer
	// because we start this observer on init we can be sure that the spaceIdsBefore is empty on the first run for peer
	go p.worker()
	p.peerStore.AddObserver(func(peerId string, spaceIdsBefore, spaceIdsAfter []string, peerRemoved bool) {
		removed, added := lo.Difference(spaceIdsBefore, spaceIdsAfter)
		err := p.refreshSpaces(lo.Union(removed, added))
		if errors.Is(err, ErrClosed) {
			return
		} else if err != nil {
			log.Errorf("refreshSpaces failed: %v", err)
		}
	})
	return nil
}

func (p *p2pStatus) sendStatusForNewSession(ctx session.Context) error {
	p.Lock()
	defer p.Unlock()
	for spaceId, space := range p.spaceIds {
		p.sendEvent(ctx.ID(), spaceId, space.status.ToPb(), space.connectionsCount)
	}
	return nil
}

func (p *p2pStatus) Run(ctx context.Context) error {
	return nil
}

func (p *p2pStatus) Close(ctx context.Context) error {
	if p.contextCancel != nil {
		p.contextCancel()
	}
	<-p.workerFinished
	return nil
}

func (p *p2pStatus) Name() (name string) {
	return CName
}

func (p *p2pStatus) setNotPossibleStatus(state localdiscovery.DiscoveryPossibility) {
	p.Lock()
	if p.p2pLastState == state {
		p.Unlock()
		return
	}
	p.p2pLastState = state
	p.Unlock()
	p.refreshAllSpaces()
}

// RegisterSpace registers spaceId to be monitored for p2p status changes
// must be called only when p2pStatus is Running
func (p *p2pStatus) RegisterSpace(spaceId string) {
	select {
	case <-p.ctx.Done():
		return
	case p.refreshSpaceId <- spaceId:
	}
}

// UnregisterSpace unregisters spaceId from monitoring
// must be called only when p2pStatus is Running
func (p *p2pStatus) UnregisterSpace(spaceId string) {
	p.Lock()
	defer p.Unlock()
	delete(p.spaceIds, spaceId)
}

func (p *p2pStatus) worker() {
	defer close(p.workerFinished)
	for {
		select {
		case <-p.ctx.Done():
			return
		case spaceId := <-p.refreshSpaceId:
			p.processSpaceStatusUpdate(spaceId)
		}
	}
}

func (p *p2pStatus) refreshAllSpaces() {
	p.Lock()
	var spaceIds = make([]string, 0, len(p.spaceIds))
	for spaceId := range p.spaceIds {
		spaceIds = append(spaceIds, spaceId)
	}
	p.Unlock()
	err := p.refreshSpaces(spaceIds)
	if errors.Is(err, ErrClosed) {
		return
	} else if err != nil {
		log.Errorf("refreshSpaces failed: %v", err)
	}
}

func (p *p2pStatus) refreshSpaces(spaceIds []string) error {
	for _, spaceId := range spaceIds {
		select {
		case <-p.ctx.Done():
			return ErrClosed
		case p.refreshSpaceId <- spaceId:

		}
	}
	return nil
}

// updateSpaceP2PStatus updates status for specific spaceId and sends event if status changed
func (p *p2pStatus) processSpaceStatusUpdate(spaceId string) {
	p.Lock()
	defer p.Unlock()
	var (
		currentStatus *spaceStatus
		ok            bool
	)
	if currentStatus, ok = p.spaceIds[spaceId]; !ok {
		currentStatus = &spaceStatus{
			status:           Unknown,
			connectionsCount: 0,
		}

		p.spaceIds[spaceId] = currentStatus
	}
	connectionCount := p.countOpenConnections(spaceId)
	newStatus := p.getResultStatus(p.p2pLastState, connectionCount)

	if currentStatus.status != newStatus || currentStatus.connectionsCount != connectionCount {
		p.sendEvent("", spaceId, newStatus.ToPb(), connectionCount)
		currentStatus.status = newStatus
		currentStatus.connectionsCount = connectionCount
	}
}

func (p *p2pStatus) getResultStatus(state localdiscovery.DiscoveryPossibility, connectionCount int64) Status {
	if connectionCount == 0 {
		if state == localdiscovery.DiscoveryNoInterfaces {
			return NotPossible
		}
		if state == localdiscovery.DiscoveryLocalNetworkRestricted {
			return Restricted
		}
		return NotConnected
	}

	return Connected
}

func (p *p2pStatus) countOpenConnections(spaceId string) int64 {
	peerIds := p.peerStore.LocalPeerIds(spaceId)
	return int64(len(peerIds))
}

// sendEvent sends event to session with sessionToken or broadcast to all sessions if sessionToken is empty
func (p *p2pStatus) sendEvent(sessionToken string, spaceId string, status pb.EventP2PStatusStatus, count int64) {
	ev := event.NewEventSingleMessage("", &pb.EventMessageValueOfP2PStatusUpdate{
		P2PStatusUpdate: &pb.EventP2PStatusUpdate{
			SpaceId:        spaceId,
			Status:         status,
			DevicesCounter: count,
		},
	})
	if sessionToken != "" {
		p.eventSender.SendToSession(sessionToken, ev)
		return
	}
	p.eventSender.Broadcast(ev)
}
