package database

import (
	"sync"

	"github.com/gogo/protobuf/types"
)

type subscription struct {
	ids  []string
	quit chan struct{}
	ch   chan *types.Struct
	sync.RWMutex
}

type Subscription interface {
	QuitChan() chan struct{}
	RecordChan() chan *types.Struct
	Subscribe(id string)
	Subscriptions() []string
	SubscribedForId(id string) bool
}

func (sub *subscription) RecordChan() chan *types.Struct {
	return sub.ch
}

func (sub *subscription) QuitChan() chan struct{} {
	return sub.quit
}

func (sub *subscription) Subscribe(id string) {
	sub.Lock()
	defer sub.Unlock()
	for _, idEx := range sub.ids {
		if idEx == id {
			return
		}
	}
	sub.ids = append(sub.ids, id)
}

func (sub *subscription) SubscribedForId(id string) bool {
	sub.RLock()
	defer sub.RUnlock()
	for _, idE := range sub.ids {
		return idE == id
	}
	return false
}

func (sub *subscription) Subscriptions() []string {
	sub.RLock()
	defer sub.RUnlock()
	return sub.ids
}

func NewSubscription(ids []string, ch chan *types.Struct, quit chan struct{}) Subscription {
	return &subscription{ids: ids, ch: ch, quit: quit}
}
