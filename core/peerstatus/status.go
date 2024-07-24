package peerstatus

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
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
)

func (s Status) ToPb() pb.EventP2PStatusStatus {
	switch s {
	case Connected:
		return pb.EventP2PStatus_Connected
	case NotConnected:
		return pb.EventP2PStatus_NotConnected
	case NotPossible:
		return pb.EventP2PStatus_NotPossible
	}
	// default status is NotConnected
	return pb.EventP2PStatus_NotConnected
}

type LocalDiscoveryHook interface {
	app.Component
	RegisterP2PNotPossible(hook func())
	RegisterResetNotPossible(hook func())
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
	p2pNotPossible bool // global flag means p2p is not possible because of network
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
	localDiscoveryHook.RegisterP2PNotPossible(p.setNotPossibleStatus)
	localDiscoveryHook.RegisterResetNotPossible(p.resetNotPossibleStatus)
	sessionHookRunner.RegisterHook(p.sendStatusForNewSession)
	p.ctx, p.contextCancel = context.WithCancel(context.Background())
	p.peerStore.AddObserver(func(peerId string, spaceIdsBefore, spaceIdsAfter []string, peerRemoved bool) {
		go func() {
			// we need to update status for all spaces that were either added or removed to some local peer
			// because we start this observer on init we can be sure that the spaceIdsBefore is empty on the first run for peer
			removed, added := lo.Difference(spaceIdsBefore, spaceIdsAfter)
			err := p.refreshSpaces(lo.Union(removed, added))
			if errors.Is(err, ErrClosed) {
				return
			} else if err != nil {
				log.Errorf("refreshSpaces failed: %v", err)
			}
		}()
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
	go p.worker()
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

func (p *p2pStatus) setNotPossibleStatus() {
	p.Lock()
	p.p2pNotPossible = true
	p.Unlock()
	p.refreshAllSpaces()
}

func (p *p2pStatus) resetNotPossibleStatus() {
	p.Lock()
	p.p2pNotPossible = false
	p.Unlock()
	p.refreshAllSpaces()
}

func (p *p2pStatus) RegisterSpace(spaceId string) {
	select {
	case <-p.ctx.Done():
		return
	case p.refreshSpaceId <- spaceId:
	}
}

func (p *p2pStatus) UnregisterSpace(spaceId string) {
	p.Lock()
	defer p.Unlock()
	delete(p.spaceIds, spaceId)
}

func (p *p2pStatus) worker() {
	defer close(p.workerFinished)
	timer := time.NewTicker(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case spaceId := <-p.refreshSpaceId:
			p.processSpaceStatusUpdate(spaceId)
		case <-timer.C:
			// todo: looks like we don't need this anymore because we use observer
			p.refreshAllSpaces()
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
	newStatus := p.getResultStatus(p.p2pNotPossible, connectionCount)

	if currentStatus.status != newStatus || currentStatus.connectionsCount != connectionCount {
		p.sendEvent("", spaceId, newStatus.ToPb(), connectionCount)
		currentStatus.status = newStatus
		currentStatus.connectionsCount = connectionCount
	}
}

func (p *p2pStatus) getResultStatus(notPossible bool, connectionCount int64) Status {
	if notPossible && connectionCount == 0 {
		return NotPossible
	}
	if connectionCount == 0 {
		return NotConnected
	} else {
		return Connected
	}
}

func (p *p2pStatus) countOpenConnections(spaceId string) int64 {
	var connectionCount int64
	ctx, cancelFunc := context.WithTimeout(p.ctx, time.Second*10)
	defer cancelFunc()
	peerIds := p.peerStore.LocalPeerIds(spaceId)
	for _, peerId := range peerIds {
		_, err := p.peersConnectionPool.Pick(ctx, peerId)
		if err != nil {
			continue
		}
		connectionCount++
	}
	return connectionCount
}

// sendEvent sends event to session with sessionToken or broadcast to all sessions if sessionToken is empty
func (p *p2pStatus) sendEvent(sessionToken string, spaceId string, status pb.EventP2PStatusStatus, count int64) {
	event := &pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfP2PStatusUpdate{
					P2PStatusUpdate: &pb.EventP2PStatusUpdate{
						SpaceId:        spaceId,
						Status:         status,
						DevicesCounter: count,
					},
				},
			},
		},
	}
	if sessionToken != "" {
		p.eventSender.SendToSession(sessionToken, event)
		return
	}
	p.eventSender.Broadcast(event)
}
