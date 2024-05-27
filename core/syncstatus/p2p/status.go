package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
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
	RegisterPeerDiscovered(hook func())
	RegisterP2PNotPossible(hook func())
}

type PeerUpdateHook interface {
	Register(hook func())
}

type StatusUpdateSender interface {
	app.ComponentRunnable
	CheckPeerStatus()
	P2PNotPossible()
}

type p2pStatus struct {
	spaceId       string
	eventSender   event.Sender
	contextCancel context.CancelFunc
	ctx           context.Context

	sync.Mutex
	status Status

	forceCheckSpace     chan struct{}
	updateStatus        chan Status
	peersConnectionPool pool.Service
}

func NewP2PStatus(spaceId string) StatusUpdateSender {
	return &p2pStatus{forceCheckSpace: make(chan struct{}), updateStatus: make(chan Status), spaceId: spaceId}
}

func (p *p2pStatus) Init(a *app.App) (err error) {
	p.eventSender = app.MustComponent[event.Sender](a)
	p.peersConnectionPool = app.MustComponent[pool.Service](a)

	hookRegister := app.MustComponent[HookRegister](a)

	hookRegister.RegisterP2PNotPossible(p.P2PNotPossible)
	hookRegister.RegisterPeerDiscovered(p.CheckPeerStatus)

	peerManager := app.MustComponent[PeerUpdateHook](a)
	peerManager.Register(p.CheckPeerStatus)
	return
}

func (p *p2pStatus) Name() (name string) {
	return CName
}

func (p *p2pStatus) Run(ctx context.Context) (err error) {
	p.ctx, p.contextCancel = context.WithCancel(context.Background())
	go p.checkP2PDevices()
	return nil
}

func (p *p2pStatus) Close(ctx context.Context) (err error) {
	if p.contextCancel != nil {
		p.contextCancel()
	}
	return
}

func (p *p2pStatus) CheckPeerStatus() {
	p.forceCheckSpace <- struct{}{}
}

func (p *p2pStatus) P2PNotPossible() {
	p.updateStatus <- NotPossible
}

func (p *p2pStatus) checkP2PDevices() {
	timer := time.NewTimer(20 * time.Second)
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
		}
	}
}

func (p *p2pStatus) updateSpaceP2PStatus() {
	p.Lock()
	defer p.Unlock()
	var connectionCount int64
	p.peersConnectionPool.ForEach(func(v ocache.Object) bool {
		connectionCount++
		return true
	})
	if p.status != Unknown {
		// avoiding sending of redundant event
		p.handleNonUnknownStatus(connectionCount)
	} else {
		p.handleUnknownStatus(connectionCount)
	}
}

func (p *p2pStatus) handleUnknownStatus(connectionCount int64) {
	if connectionCount > 0 {
		p.sendEvent(p.spaceId, pb.EventP2PStatus_Connected)
		p.status = Connected
	} else {
		p.sendEvent(p.spaceId, pb.EventP2PStatus_NotConnected)
		p.status = NotConnected
	}
}

func (p *p2pStatus) handleNonUnknownStatus(connectionCount int64) {
	if p.status == Connected && connectionCount == 0 {
		p.sendEvent(p.spaceId, pb.EventP2PStatus_NotConnected)
		p.status = NotConnected
	}
	if (p.status == NotConnected || p.status == NotPossible) && connectionCount > 0 {
		p.sendEvent(p.spaceId, pb.EventP2PStatus_Connected)
		p.status = Connected
	}
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
