package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/peerstatus"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

const CName = "core.syncstatus.p2p"

type StatusUpdateSender interface {
	app.ComponentRunnable
	SendPeerUpdate()
	SendNewStatus(status peerstatus.Status)
}

type p2pStatus struct {
	spaceId       string
	peerStore     peerstore.PeerStore
	eventSender   event.Sender
	contextCancel context.CancelFunc
	ctx           context.Context

	sync.Mutex
	status peerstatus.Status

	forceCheckSpace chan struct{}
	updateStatus    chan peerstatus.Status
}

func NewP2PStatus(spaceId string) StatusUpdateSender {
	return &p2pStatus{forceCheckSpace: make(chan struct{}), updateStatus: make(chan peerstatus.Status), spaceId: spaceId}
}

func (p *p2pStatus) Init(a *app.App) (err error) {
	p.peerStore = app.MustComponent[peerstore.PeerStore](a)
	p.eventSender = app.MustComponent[event.Sender](a)
	observerComponent := app.MustComponent[ObserverComponent](a)
	observerComponent.AddObserver(p.spaceId, p)
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

func (p *p2pStatus) SendPeerUpdate() {
	p.forceCheckSpace <- struct{}{}
}

func (p *p2pStatus) SendNewStatus(status peerstatus.Status) {
	p.updateStatus <- status
}

func (p *p2pStatus) checkP2PDevices() {
	timer := time.NewTimer(10 * time.Second)
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
	peerIds := p.peerStore.LocalPeerIds(p.spaceId)
	if p.status != peerstatus.Unknown {
		// avoiding sending of redundant event
		if p.status == peerstatus.Connected && len(peerIds) == 0 {
			p.sendEvent(p.spaceId, pb.EventP2PStatus_NotConnected)
			p.status = peerstatus.NotConnected
		}
		if (p.status == peerstatus.NotConnected || p.status == peerstatus.NotPossible) && len(peerIds) > 0 {
			p.sendEvent(p.spaceId, pb.EventP2PStatus_Connected)
			p.status = peerstatus.Connected
		}
	} else {
		if len(peerIds) > 0 {
			p.sendEvent(p.spaceId, pb.EventP2PStatus_Connected)
			p.status = peerstatus.Connected
		} else {
			p.sendEvent(p.spaceId, pb.EventP2PStatus_NotConnected)
			p.status = peerstatus.NotConnected
		}
	}
}

func (p *p2pStatus) sendNewStatus(status peerstatus.Status) {
	var pbStatus pb.EventP2PStatusStatus
	switch status {
	case peerstatus.Connected:
		pbStatus = pb.EventP2PStatus_Connected
	case peerstatus.NotConnected:
		pbStatus = pb.EventP2PStatus_NotConnected
	case peerstatus.NotPossible:
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
