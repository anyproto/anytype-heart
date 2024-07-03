package peerstatus

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

const CName = "core.syncstatus.p2p"

type Status int32

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
	SendNotPossibleStatus()
	CheckPeerStatus()
	ResetNotPossibleStatus()
	RegisterSpace(spaceId string)
	UnregisterSpace(spaceId string)
}

type p2pStatus struct {
	spaceIds      map[string]struct{}
	eventSender   event.Sender
	contextCancel context.CancelFunc
	ctx           context.Context
	peerStore     peerstore.PeerStore

	sync.Mutex
	status           Status
	connectionsCount int64

	forceCheckSpace        chan struct{}
	updateStatus           chan Status
	resetNotPossibleStatus chan struct{}
	finish                 chan struct{}

	peersConnectionPool pool.Pool
}

func New() PeerToPeerStatus {
	p2pStatusService := &p2pStatus{
		forceCheckSpace:        make(chan struct{}, 1),
		updateStatus:           make(chan Status, 1),
		resetNotPossibleStatus: make(chan struct{}, 1),
		finish:                 make(chan struct{}),
		spaceIds:               make(map[string]struct{}),
	}

	return p2pStatusService
}

func (p *p2pStatus) Init(a *app.App) (err error) {
	p.eventSender = app.MustComponent[event.Sender](a)
	p.peerStore = app.MustComponent[peerstore.PeerStore](a)
	p.peersConnectionPool = app.MustComponent[pool.Service](a)
	localDiscoveryHook := app.MustComponent[LocalDiscoveryHook](a)
	sessionHookRunner := app.MustComponent[session.HookRunner](a)
	localDiscoveryHook.RegisterP2PNotPossible(p.SendNotPossibleStatus)
	localDiscoveryHook.RegisterResetNotPossible(p.ResetNotPossibleStatus)
	sessionHookRunner.RegisterHook(p.sendStatusForNewSession)
	return nil
}

func (p *p2pStatus) sendStatusForNewSession(ctx session.Context) error {
	p.sendStatus(p.status)
	return nil
}

func (p *p2pStatus) Run(ctx context.Context) error {
	p.ctx, p.contextCancel = context.WithCancel(context.Background())
	go p.checkP2PDevices()
	return nil
}

func (p *p2pStatus) Close(ctx context.Context) error {
	if p.contextCancel != nil {
		p.contextCancel()
	}
	<-p.finish
	return nil
}

func (p *p2pStatus) Name() (name string) {
	return CName
}

func (p *p2pStatus) CheckPeerStatus() {
	p.forceCheckSpace <- struct{}{}
}

func (p *p2pStatus) SendNotPossibleStatus() {
	p.updateStatus <- NotPossible
}

func (p *p2pStatus) ResetNotPossibleStatus() {
	p.resetNotPossibleStatus <- struct{}{}
}

func (p *p2pStatus) RegisterSpace(spaceId string) {
	p.Lock()
	defer p.Unlock()
	p.spaceIds[spaceId] = struct{}{}
	p.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfP2PStatusUpdate{
					P2PStatusUpdate: &pb.EventP2PStatusUpdate{
						SpaceId:        spaceId,
						Status:         p.mapStatusToEvent(p.status),
						DevicesCounter: p.connectionsCount,
					},
				},
			},
		},
	})
}

func (p *p2pStatus) UnregisterSpace(spaceId string) {
	p.Lock()
	defer p.Unlock()
	delete(p.spaceIds, spaceId)
}

func (p *p2pStatus) checkP2PDevices() {
	defer close(p.finish)
	timer := time.NewTicker(10 * time.Second)
	defer timer.Stop()
	p.updateSpaceP2PStatus()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-timer.C:
			p.updateSpaceP2PStatus()
		case <-p.forceCheckSpace:
			p.updateSpaceP2PStatus()
		case newStatus := <-p.updateStatus:
			p.sendStatus(newStatus)
		case <-p.resetNotPossibleStatus:
			p.resetNotPossible()
		}
	}
}

func (p *p2pStatus) updateSpaceP2PStatus() {
	p.Lock()
	defer p.Unlock()
	connectionCount := p.countOpenConnections()
	newStatus, event := p.getResultStatus(connectionCount)
	if newStatus == NotPossible {
		return
	}
	connectionCount++ // count current device
	if p.status != newStatus || p.connectionsCount != connectionCount {
		p.sendEvent(event, connectionCount)
		p.status = newStatus
		p.connectionsCount = connectionCount
	}
}

func (p *p2pStatus) getResultStatus(connectionCount int64) (Status, pb.EventP2PStatusStatus) {
	var (
		newStatus Status
		event     pb.EventP2PStatusStatus
	)
	if p.status == NotPossible && connectionCount == 0 {
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
func (p *p2pStatus) countOpenConnections() int64 {
	var connectionCount int64
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*20)
	defer cancelFunc()
	peerIds := p.peerStore.AllLocalPeers()
	for _, peerId := range peerIds {
		_, err := p.peersConnectionPool.Pick(ctx, peerId)
		if err != nil {
			continue
		}
		connectionCount++
	}
	return connectionCount
}

func (p *p2pStatus) sendStatus(status Status) {
	p.Lock()
	defer p.Unlock()
	pbStatus := p.mapStatusToEvent(status)
	p.status = status
	p.sendEvent(pbStatus, p.connectionsCount)
}

func (p *p2pStatus) mapStatusToEvent(status Status) pb.EventP2PStatusStatus {
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

func (p *p2pStatus) sendEvent(status pb.EventP2PStatusStatus, count int64) {
	for spaceId := range p.spaceIds {
		p.eventSender.Broadcast(&pb.Event{
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
		})
	}
}

func (p *p2pStatus) resetNotPossible() {
	p.Lock()
	defer p.Unlock()
	if p.status == NotPossible {
		p.status = NotConnected
	}
}
