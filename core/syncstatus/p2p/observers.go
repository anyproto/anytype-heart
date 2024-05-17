package p2p

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/peerstatus"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger(observerServiceName)

const observerServiceName = "core.syncstatus.p2p.observers"

type ObserverComponent interface {
	app.Component
	AddObserver(spaceId string, observer StatusUpdateSender)
	BroadcastStatus(status peerstatus.Status)
	SendPeerUpdate(spaceIds []string)
	BroadcastPeerUpdate()
}

type Observers struct {
	observer map[string]StatusUpdateSender
}

func (o *Observers) SendPeerUpdate(spaceIds []string) {
	for _, spaceId := range spaceIds {
		if observer, ok := o.observer[spaceId]; ok {
			observer.SendPeerUpdate()
			return
		}
		log.Errorf("observer not registered for space %s", spaceIds)
	}
}

func (o *Observers) BroadcastStatus(status peerstatus.Status) {
	for _, observer := range o.observer {
		observer.SendNewStatus(status)
	}
}

func (o *Observers) BroadcastPeerUpdate() {
	for _, observer := range o.observer {
		observer.SendPeerUpdate()
	}
}

func (o *Observers) Init(a *app.App) (err error) {
	return
}

func (o *Observers) Name() (name string) {
	return observerServiceName
}

func (o *Observers) AddObserver(spaceId string, observer StatusUpdateSender) {
	o.observer[spaceId] = observer
}

func NewObservers() *Observers {
	return &Observers{observer: make(map[string]StatusUpdateSender, 0)}
}
