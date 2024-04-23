package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"
)

type subscription struct {
	ids              []string
	quit             chan struct{}
	closed           bool
	ch               chan *types.Struct
	wg               sync.WaitGroup
	publishQueue     *mb.MB[*types.Struct]
	processQueueOnce sync.Once
	sync.RWMutex
}

type Subscription interface {
	Close()
	RecordChan() chan *types.Struct
	Subscribe(ids []string) (added []string)
	Subscriptions() []string
	// Publish is blocking
	// returns false if the subscription is closed or the id is not subscribed
	Publish(id string, msg *types.Struct) bool
	// PublishAsync is non-blocking and guarantees the order of messages
	// returns false if the subscription is closed or the id is not subscribed
	PublishAsync(id string, msg *types.Struct) bool
}

func (sub *subscription) RecordChan() chan *types.Struct {
	return sub.ch
}

func (sub *subscription) Close() {
	sub.Lock()
	if sub.closed {
		sub.Unlock()
		return
	}
	sub.closed = true
	sub.Unlock()
	close(sub.quit)

	sub.publishQueue.Close()
	sub.wg.Wait()
	close(sub.ch)
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

// should be called via sub.processQueueOnce
func (sub *subscription) processQueue() {
	go func() {
		select {
		case <-sub.quit:
			err := sub.publishQueue.Close()
			if err != nil {
				log.Errorf("subscription %p failed to close async queue: %s", sub, err)
			}
			unprocessed := sub.publishQueue.Len()
			if unprocessed > 0 {
				log.Errorf("subscription %p has %d unprocessed messages in the async queue", sub, unprocessed)
			}
		}
	}()

	var (
		msg *types.Struct
		err error
	)
	for {
		// no need for cancellation here, because we close the queue itself on quit and it will return
		msg, err = sub.publishQueue.WaitOne(context.Background())
		if err != nil {
			if !errors.Is(err, mb.ErrClosed) {
				log.Errorf("subscription %p failed to get message from async queue: %s", sub, err)
			}
			return
		}
		select {
		case sub.ch <- msg:
			continue
		}
	}
}

// PublishAsync is non-blocking and guarantees the order of messages
// returns false if the subscription is closed or the id is not subscribed
func (sub *subscription) PublishAsync(id string, msg *types.Struct) bool {
	sub.RLock()
	if sub.closed {
		sub.RUnlock()
		return false
	}
	if !slices.Contains(sub.ids, id) {
		sub.RUnlock()
		return false
	}
	sub.RUnlock()
	sub.processQueueOnce.Do(func() {
		go sub.processQueue()
	})
	log.Debugf("objStore subscription sendasync %s %p", id, sub)
	err := sub.publishQueue.Add(context.Background(), msg)
	return err == nil
}

func (sub *subscription) Publish(id string, msg *types.Struct) bool {
	sub.RLock()
	if sub.closed {
		sub.RUnlock()
		return false
	}
	if !slices.Contains(sub.ids, id) {
		sub.RUnlock()
		return false
	}
	sub.wg.Add(1)
	defer sub.wg.Done()
	sub.RUnlock()

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

func (sub *subscription) SubscribedForId(id string) bool {
	sub.RLock()
	defer sub.RUnlock()
	for _, idE := range sub.ids {
		if idE == id {
			return true
		}
	}
	return false
}

func (sub *subscription) Subscriptions() []string {
	sub.RLock()
	defer sub.RUnlock()
	return sub.ids
}

func NewSubscription(ids []string, ch chan *types.Struct) Subscription {
	return &subscription{ids: ids, ch: ch, quit: make(chan struct{})}
}
