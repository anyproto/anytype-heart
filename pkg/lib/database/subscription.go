package database

import (
	"fmt"
	"sync"
	"time"

	"github.com/gogo/protobuf/types"
)

type subscription struct {
	ids    []string
	quit   chan struct{}
	closed bool
	ch     chan *types.Struct
	sync.RWMutex
}

type Subscription interface {
	Close()
	Subscribe(ids []string) (added []string)
	Publish(id string, msg *types.Struct) bool
}

func (sub *subscription) Close() {
	sub.Lock()
	defer sub.Unlock()
	if sub.closed {
		return
	}

	close(sub.quit)
	close(sub.ch)

	sub.closed = true
}

func (sub *subscription) Subscribe(ids []string) (added []string) {
	sub.Lock()
	defer sub.Unlock()
loop:
	for _, id := range ids {
		for _, idEx := range sub.ids {
			if idEx == id {
				continue loop
			}
		}
		added = append(added, id)
		sub.ids = append(sub.ids, id)
	}
	return
}

func (sub *subscription) Publish(id string, msg *types.Struct) bool {
	sub.RLock()
	defer sub.RUnlock()
	if sub.closed {
		return false
	}

	for _, idE := range sub.ids {
		if idE == id {
			log.Debugf("objStore subscription send %s %p", id, sub)
			var total time.Duration
			for {
				select {
				case <-sub.quit:
					return false
				case sub.ch <- msg:
					return true
				case <-time.After(time.Second * 3):
					total += time.Second * 3
					log.Errorf(fmt.Sprintf("subscription %p is blocked for %.0f seconds, failed to send %s", sub, total.Seconds(), id))
					continue
				}
			}
		}
	}
	return false
}

func NewSubscription(ids []string, ch chan *types.Struct) Subscription {
	return &subscription{ids: ids, ch: ch, quit: make(chan struct{})}
}
