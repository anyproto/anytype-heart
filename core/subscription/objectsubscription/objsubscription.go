package objectsubscription

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

type RelationKeyValue struct {
	Key   string
	Value domain.Value
}

type SubscriptionParams[T any] struct {
	// SetDetails transforms details to entry
	// It's mandatory
	SetDetails func(details *domain.Details) (id string, entry T)
	// UpdateKeys updates a value for a given key
	// It's mandatory
	UpdateKeys func(keyValues []RelationKeyValue, curEntry T) (updatedEntry T)
	// RemoveKeys removes keys
	// It's mandatory
	RemoveKeys func(keys []string, curEntry T) (updatedEntry T)

	// OnAdded called when object appears in subscription
	OnAdded func(id string, entry T)
	// OnRemove called when object is removed from subscription
	OnRemoved func(id string, entry T)

	// CustomFilter is an optional filter for objects that would be applied to subscription along with SubscribeRequest.Filters
	CustomFilter func(details *domain.Details) bool
}

type subState int32

const (
	stateNew subState = iota
	stateRunning
	stateClosed
)

type ObjectSubscription[T any] struct {
	request    subscription.SubscribeRequest
	service    subscription.Service
	ch         chan struct{}
	events     *mb.MB[*pb.EventMessage]
	filterKeys map[string]struct{}
	ctx        context.Context
	cancel     context.CancelFunc

	params SubscriptionParams[T]

	state atomic.Int32
	mx    sync.Mutex
	sub   map[string]T

	keyToId map[string]string
}

var IdSubscriptionParams = SubscriptionParams[struct{}]{
	SetDetails: func(t *domain.Details) (string, struct{}) {
		return t.GetString(bundle.RelationKeyId), struct{}{}
	},
	UpdateKeys: func(keyValues []RelationKeyValue, s struct{}) struct{} {
		return struct{}{}
	},
	RemoveKeys: func(strings []string, s struct{}) struct{} {
		return struct{}{}
	},
}

func NewIdSubscription(subService subscription.Service, req subscription.SubscribeRequest) *ObjectSubscription[struct{}] {
	return New(subService, req, IdSubscriptionParams)
}

func NewIdSubscriptionFromQueue(queue *mb.MB[*pb.EventMessage]) *ObjectSubscription[struct{}] {
	return NewFromQueue(queue, IdSubscriptionParams)
}

func New[T any](subService subscription.Service, req subscription.SubscribeRequest, params SubscriptionParams[T]) *ObjectSubscription[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &ObjectSubscription[T]{
		request:    req,
		service:    subService,
		filterKeys: make(map[string]struct{}),
		ch:         make(chan struct{}),
		params:     params,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func NewFromQueue[T any](queue *mb.MB[*pb.EventMessage], params SubscriptionParams[T]) *ObjectSubscription[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &ObjectSubscription[T]{
		events: queue,
		ch:     make(chan struct{}),
		params: params,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (o *ObjectSubscription[T]) Run() error {
	if !o.state.CompareAndSwap(int32(stateNew), int32(stateRunning)) {
		switch subState(o.state.Load()) {
		case stateRunning:
			return fmt.Errorf("already running")
		case stateClosed:
			return fmt.Errorf("already closed")
		default:
			return fmt.Errorf("invalid state")
		}
	}

	if o.service == nil && o.events == nil {
		close(o.ch)
		return fmt.Errorf("subscription created with nil event queue")
	}
	if o.params.SetDetails == nil {
		close(o.ch)
		return fmt.Errorf("SetDetails function not set")
	}
	if o.params.UpdateKeys == nil {
		close(o.ch)
		return fmt.Errorf("UpdateKeys function not set")
	}
	if o.params.RemoveKeys == nil {
		close(o.ch)
		return fmt.Errorf("RemoveKeys function not set")
	}

	o.request.Internal = true
	o.sub = map[string]T{}
	o.keyToId = map[string]string{}
	if o.service != nil {
		resp, err := o.service.Search(o.request)
		if err != nil {
			close(o.ch)
			return err
		}
		for _, key := range o.request.Keys {
			o.filterKeys[key] = struct{}{}
		}
		for _, rec := range resp.Records {
			id, data := o.params.SetDetails(rec)
			if o.params.CustomFilter != nil && !o.params.CustomFilter(rec) {
				continue
			}
			o.sub[id] = data
			o.addKey(id, rec)
		}
		o.events = resp.Output
	}
	go o.read()
	return nil
}

func (o *ObjectSubscription[T]) Close() {
	if o.state.Swap(int32(stateClosed)) == int32(stateRunning) {
		o.cancel()
		<-o.ch
	}
}

func (o *ObjectSubscription[T]) Len() int {
	o.mx.Lock()
	defer o.mx.Unlock()
	return len(o.sub)
}

func (o *ObjectSubscription[T]) Get(id string) (T, bool) {
	o.mx.Lock()
	defer o.mx.Unlock()
	entry, ok := o.sub[id]
	return entry, ok
}

func (o *ObjectSubscription[T]) GetByKey(key string) (T, bool) {
	o.mx.Lock()
	defer o.mx.Unlock()
	id, ok := o.keyToId[key]
	if !ok {
		var defValue T
		return defValue, false
	}
	entry, ok := o.sub[id]
	return entry, ok
}

func (o *ObjectSubscription[T]) Has(id string) bool {
	o.mx.Lock()
	defer o.mx.Unlock()
	_, ok := o.sub[id]
	return ok
}

func (o *ObjectSubscription[T]) Iterate(iter func(id string, data T) bool) {
	o.mx.Lock()
	defer o.mx.Unlock()
	for id, ent := range o.sub {
		if !iter(id, ent) {
			return
		}
	}
}

func (o *ObjectSubscription[T]) read() {
	defer close(o.ch)
	readEvent := func(event *pb.EventMessage) {
		o.mx.Lock()
		defer o.mx.Unlock()
		switch v := event.Value.(type) {
		case *pb.EventMessageValueOfSubscriptionAdd:
			// Nothing to do here, add logic is in ObjectDetailsSet case
		case *pb.EventMessageValueOfSubscriptionRemove:
			curEntry, ok := o.sub[v.SubscriptionRemove.Id]
			if ok {
				delete(o.sub, v.SubscriptionRemove.Id)
				if o.params.OnRemoved != nil {
					o.params.OnRemoved(v.SubscriptionRemove.Id, curEntry)
				}
			}
		case *pb.EventMessageValueOfObjectDetailsAmend:
			curEntry, ok := o.sub[v.ObjectDetailsAmend.Id]
			if ok {
				keyValues := make([]RelationKeyValue, 0, len(v.ObjectDetailsAmend.Details))
				for _, value := range v.ObjectDetailsAmend.Details {
					if o.filterKeys != nil {
						if _, ok := o.filterKeys[value.Key]; !ok {
							continue
						}
					}
					keyValues = append(keyValues, RelationKeyValue{
						Key:   value.Key,
						Value: domain.ValueFromProto(value.Value),
					})
				}
				if len(keyValues) != 0 {
					curEntry = o.params.UpdateKeys(keyValues, curEntry)
				}
				o.sub[v.ObjectDetailsAmend.Id] = curEntry
			}
		case *pb.EventMessageValueOfObjectDetailsUnset:
			curEntry, ok := o.sub[v.ObjectDetailsUnset.Id]
			if ok {
				curEntry = o.params.RemoveKeys(v.ObjectDetailsUnset.Keys, curEntry)
				o.sub[v.ObjectDetailsUnset.Id] = curEntry
			}
		case *pb.EventMessageValueOfObjectDetailsSet:
			details := domain.NewDetailsFromProto(v.ObjectDetailsSet.Details)
			if o.params.CustomFilter != nil && !o.params.CustomFilter(details) {
				return
			}
			_, newEntry := o.params.SetDetails(details)
			if _, ok := o.sub[v.ObjectDetailsSet.Id]; !ok {
				if o.params.OnAdded != nil {
					o.params.OnAdded(v.ObjectDetailsSet.Id, newEntry)
				}
				o.addKey(v.ObjectDetailsSet.Id, details)
			}
			o.sub[v.ObjectDetailsSet.Id] = newEntry
		}
	}
	for {
		event, err := o.events.WaitOne(o.ctx)
		if err != nil {
			return
		}
		readEvent(event)
	}
}

func (o *ObjectSubscription[T]) addKey(id string, details *domain.Details) {
	if key := details.GetString(bundle.RelationKeyRelationKey); key != "" {
		o.keyToId[key] = id
	}
}
