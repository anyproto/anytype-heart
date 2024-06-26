package peerstatus

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/core/event"
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

type HookRegister interface {
	RegisterP2PNotPossible(hook func())
	RegisterResetNotPossible(hook func())
}

type PeerToPeerStatus interface {
	Run()
	Close()
	SendNotPossibleStatus()
	CheckPeerStatus()
	ResetNotPossibleStatus()
}

type p2pStatus struct {
	spaceId       string
	eventSender   event.Sender
	contextCancel context.CancelFunc
	ctx           context.Context
	peerStore     peerstore.PeerStore

	sync.Mutex
	status Status

	forceCheckSpace        chan struct{}
	updateStatus           chan Status
	resetNotPossibleStatus chan struct{}
	finish                 chan struct{}

	peersConnectionPool pool.Pool
}

func NewP2PStatus(spaceId string, eventSender event.Sender, peersConnectionPool pool.Pool, hookRegister HookRegister, peerStore peerstore.PeerStore) PeerToPeerStatus {
	p2pStatusService := &p2pStatus{
		eventSender:            eventSender,
		peersConnectionPool:    peersConnectionPool,
		forceCheckSpace:        make(chan struct{}, 1),
		updateStatus:           make(chan Status, 1),
		resetNotPossibleStatus: make(chan struct{}, 1),
		spaceId:                spaceId,
		finish:                 make(chan struct{}),
		peerStore:              peerStore,
	}
	hookRegister.RegisterP2PNotPossible(p2pStatusService.SendNotPossibleStatus)
	hookRegister.RegisterResetNotPossible(p2pStatusService.ResetNotPossibleStatus)
	return p2pStatusService
}

func (p *p2pStatus) Run() {
	p.ctx, p.contextCancel = context.WithCancel(context.Background())
	go p.checkP2PDevices()
}

func (p *p2pStatus) Close() {
	if p.contextCancel != nil {
		p.contextCancel()
	}
	<-p.finish
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

func (p *p2pStatus) checkP2PDevices() {
	defer close(p.finish)
	timer := time.NewTicker(20 * time.Second)
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
			p.sendNewStatus(newStatus)
		case <-p.resetNotPossibleStatus:
			p.resetNotPossible()
		}
	}
}

func (p *p2pStatus) updateSpaceP2PStatus() {
	p.Lock()
	defer p.Unlock()
	connectionCount := p.countOpenConnections()
	var (
		newStatus Status
		event     pb.EventP2PStatusStatus
	)
	if p.status == NotPossible && connectionCount == 0 {
		return
	}
	if connectionCount == 0 {
		event = pb.EventP2PStatus_NotConnected
		newStatus = NotConnected
	} else {
		event = pb.EventP2PStatus_Connected
		newStatus = Connected
	}
	if p.status != newStatus {
		p.sendEvent(p.spaceId, event)
		p.status = newStatus
	}
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

func (p *p2pStatus) sendNewStatus(status Status) {
	var pbStatus pb.EventP2PStatusStatus
	switch status {
	case Connected:
		pbStatus = pb.EventP2PStatus_Connected
	case NotConnected:
		pbStatus = pb.EventP2PStatus_NotConnected
	case NotPossible:
		pbStatus = pb.EventP2PStatus_NotPossible
	}
	p.Lock()
	p.status = status
	p.Unlock()
	p.sendEvent(p.spaceId, pbStatus)
}

func (p *p2pStatus) sendEvent(spaceId string, status pb.EventP2PStatusStatus) {
	p.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfP2PStatusUpdate{
					P2PStatusUpdate: &pb.EventP2PStatusUpdate{
						SpaceId: spaceId,
						Status:  status,
					},
				},
			},
		},
	})
}

func (p *p2pStatus) resetNotPossible() {
	p.Lock()
	defer p.Unlock()
	if p.status == NotPossible {
		p.status = NotConnected
	}
}
