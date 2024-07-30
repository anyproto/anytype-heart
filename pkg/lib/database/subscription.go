package database

import (
	"context"
	"errors"
	"sync"

	"github.com/cheggaaa/mb/v3"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
)

type subscription struct {
	ids              []string
	quit             chan struct{}
	closed           bool
	ch               chan *domain.Details
	wg               sync.WaitGroup
	publishQueue     mb.MB[*domain.Details]
	processQueueOnce sync.Once
	sync.RWMutex
}

type Subscription interface {
	Close()
	RecordChan() chan *domain.Details
	Subscribe(ids []string) (added []string)
	Subscriptions() []string
	// PublishAsync is non-blocking and guarantees the order of messages
	// returns false if the subscription is closed or the id is not subscribed
	PublishAsync(id string, msg *domain.Details) bool
}

func (sub *subscription) RecordChan() chan *domain.Details {
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
		<-sub.quit
		err := sub.publishQueue.Close()
		if err != nil && !errors.Is(err, mb.ErrClosed) {
			log.Errorf("subscription %p failed to close async queue: %s", sub, err)
		}
		unprocessed := sub.publishQueue.Len()
		if unprocessed > 0 {
			log.Warnf("subscription %p has %d unprocessed messages in the async queue", sub, unprocessed)
		}
	}()
	defer sub.wg.Done()
	var (
		msg *domain.Details
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
		case <-sub.quit:
			log.Warnf("subscription %p is closed, dropping message", sub)
		case sub.ch <- msg:
		}
	}
}

// PublishAsync is non-blocking and guarantees the order of messages
// returns false if the subscription is closed or the id is not subscribed
func (sub *subscription) PublishAsync(id string, msg *domain.Details) bool {
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
		sub.wg.Add(1)
		go sub.processQueue()
	})
	log.Debugf("objStore subscription sendasync %s %p", id, sub)
	// we have unlimited buffer, so it should never block, no need for context cancellation
	err := sub.publishQueue.Add(context.Background(), msg)
	return err == nil
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

func NewSubscription(ids []string, ch chan *domain.Details) Subscription {
	return &subscription{ids: ids, ch: ch, quit: make(chan struct{})}
}
