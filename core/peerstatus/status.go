package peerstatus

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool"

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

type LocalDiscoveryHook interface {
	app.Component
	RegisterP2PNotPossible(hook func())
	RegisterResetNotPossible(hook func())
}

type PeerToPeerStatus interface {
	app.ComponentRunnable
	RefreshPeerStatus(spaceId string) error
	SetNotPossibleStatus()
	ResetNotPossibleStatus()
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
	localDiscoveryHook.RegisterP2PNotPossible(p.SetNotPossibleStatus)
	localDiscoveryHook.RegisterResetNotPossible(p.ResetNotPossibleStatus)
	sessionHookRunner.RegisterHook(p.sendStatusForNewSession)
	p.peerStore.AddObserver(func(peerId string, spaceIds []string) {
		for _, spaceId := range spaceIds {
			err = p.RefreshPeerStatus(spaceId)
			if err == ErrClosed {
				return
			}
			if err != nil {
				log.Error("failed to refresh peer status", "peerId", peerId, "spaceId", spaceId, "error", err)
			}
		}
	})
	return nil
}

func (p *p2pStatus) sendStatusForNewSession(ctx session.Context) error {
	p.Lock()
	defer p.Unlock()
	for spaceId, space := range p.spaceIds {
		p.sendEvent(ctx.ID(), spaceId, mapStatusToEvent(space.status), space.connectionsCount)
	}
	return nil
}

func (p *p2pStatus) Run(ctx context.Context) error {
	p.ctx, p.contextCancel = context.WithCancel(context.Background())
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

func (p *p2pStatus) RefreshPeerStatus(spaceId string) error {
	select {
	case <-p.ctx.Done():
		return ErrClosed
	case p.refreshSpaceId <- spaceId:

	}
	return nil
}

func (p *p2pStatus) SetNotPossibleStatus() {
	p.Lock()
	p.p2pNotPossible = true
	p.Unlock()
	p.updateAllSpacesP2PStatus()
}

func (p *p2pStatus) ResetNotPossibleStatus() {
	p.Lock()
	p.p2pNotPossible = false
	p.Unlock()
	p.updateAllSpacesP2PStatus()
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
			p.updateSpaceP2PStatus(spaceId)
		case <-timer.C:
			// todo: looks like we don't need this anymore because we use observer
			p.updateAllSpacesP2PStatus()
		}
	}
}

func (p *p2pStatus) updateAllSpacesP2PStatus() {
	p.Lock()
	var spaceIds = make([]string, 0, len(p.spaceIds))
	for spaceId, _ := range p.spaceIds {
		spaceIds = append(spaceIds, spaceId)
	}
	p.Unlock()
	for _, spaceId := range spaceIds {
		select {
		case <-p.ctx.Done():
			return
		case p.refreshSpaceId <- spaceId:

		}
	}
}

// updateSpaceP2PStatus updates status for specific spaceId and sends event if status changed
func (p *p2pStatus) updateSpaceP2PStatus(spaceId string) {
	p.Lock()
	defer p.Unlock()
	var (
		currentStatus *spaceStatus
		ok            bool
	)
	if currentStatus, ok = p.spaceIds[spaceId]; !ok {
		p.spaceIds[spaceId] = &spaceStatus{
			status:           NotConnected,
			connectionsCount: -1,
		}
	}
	connectionCount := p.countOpenConnections(spaceId)
	newStatus, event := p.getResultStatus(p.p2pNotPossible, connectionCount)

	if currentStatus.status != newStatus || currentStatus.connectionsCount != connectionCount {
		p.sendEvent("", spaceId, event, connectionCount)
		currentStatus.status = newStatus
		currentStatus.connectionsCount = connectionCount
	}
}

func (p *p2pStatus) getResultStatus(notPossible bool, connectionCount int64) (Status, pb.EventP2PStatusStatus) {
	var (
		newStatus Status
		event     pb.EventP2PStatusStatus
	)

	if notPossible && connectionCount == 0 {
		return NotPossible, pb.EventP2PStatus_NotPossible
	}
	if connectionCount == 0 {
		event = pb.EventP2PStatus_NotConnected
		newStatus = NotConnected
	} else {
		event = pb.EventP2PStatus_Connected
		newStatus = Connected
	}
	return newStatus, event
}

func (p *p2pStatus) countOpenConnections(spaceId string) int64 {
	var connectionCount int64
	ctx, cancelFunc := context.WithTimeout(p.ctx, time.Second*20)
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

func mapStatusToEvent(status Status) pb.EventP2PStatusStatus {
	var pbStatus pb.EventP2PStatusStatus
	switch status {
	case Connected:
		pbStatus = pb.EventP2PStatus_Connected
	case NotConnected:
		pbStatus = pb.EventP2PStatus_NotConnected
	case NotPossible:
		pbStatus = pb.EventP2PStatus_NotPossible
	}
	return pbStatus
}

// sendEvent sends event to session with sessionToken or broadcast if sessionToken is empty
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
