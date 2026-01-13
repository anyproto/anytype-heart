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

	idToKey map[string]string
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

func NewIdSubscriptionFromQueue(queue *mb.MB[*pb.EventMessage], initialRecords []*domain.Details) *ObjectSubscription[struct{}] {
	return NewFromQueue(queue, IdSubscriptionParams, initialRecords)
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

// NewFromQueue creates an ObjectSubscription from an event queue with optional initial records.
// Use this when AsyncInit isn't available (e.g., with crossspacesub) and you need to track
// existing objects that were returned by the initial subscription response.
// Without initialRecords, the subscription will only track objects that appear after Run() is called.
func NewFromQueue[T any](queue *mb.MB[*pb.EventMessage], params SubscriptionParams[T], initialRecords []*domain.Details) *ObjectSubscription[T] {
	ctx, cancel := context.WithCancel(context.Background())
	o := &ObjectSubscription[T]{
		events: queue,
		ch:     make(chan struct{}),
		params: params,
		ctx:    ctx,
		cancel: cancel,
	}
	if len(initialRecords) > 0 {
		o.sub = make(map[string]T)
		for _, rec := range initialRecords {
			id, data := params.SetDetails(rec)
			o.sub[id] = data
		}
	}
	return o
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
	o.idToKey = map[string]string{}
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
			key, data := o.params.SetDetails(rec)
			if o.params.CustomFilter != nil && !o.params.CustomFilter(rec) {
				continue
			}
			o.sub[key] = data

			id := rec.GetString(bundle.RelationKeyId)
			o.keyToId[key] = id
			o.idToKey[id] = key
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
	key, ok := o.idToKey[id]
	if !ok {
		var defValue T
		return defValue, false
	}
	entry, ok := o.sub[key]
	return entry, ok
}

func (o *ObjectSubscription[T]) GetByKey(key string) (T, bool) {
	o.mx.Lock()
	defer o.mx.Unlock()
	entry, ok := o.sub[key]
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
	for key, ent := range o.sub {
		id := o.keyToId[key]
		if !iter(id, ent) {
			return
		}
	}
}

func (o *ObjectSubscription[T]) IterateWithKey(iter func(key string, data T) bool) {
	o.mx.Lock()
	defer o.mx.Unlock()
	for key, ent := range o.sub {
		if !iter(key, ent) {
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
			id := v.SubscriptionRemove.Id
			key := o.idToKey[id]
			curEntry, ok := o.sub[key]
			if ok {
				delete(o.sub, key)
				if o.params.OnRemoved != nil {
					o.params.OnRemoved(v.SubscriptionRemove.Id, curEntry)
				}
			}
		case *pb.EventMessageValueOfObjectDetailsAmend:
			id := v.ObjectDetailsAmend.Id
			key := o.idToKey[id]
			curEntry, ok := o.sub[key]
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
				o.sub[key] = curEntry
			}
		case *pb.EventMessageValueOfObjectDetailsUnset:
			id := v.ObjectDetailsUnset.Id
			key := o.idToKey[id]
			curEntry, ok := o.sub[key]
			if ok {
				curEntry = o.params.RemoveKeys(v.ObjectDetailsUnset.Keys, curEntry)
				o.sub[key] = curEntry
			}
		case *pb.EventMessageValueOfObjectDetailsSet:
			details := domain.NewDetailsFromProto(v.ObjectDetailsSet.Details)
			if o.params.CustomFilter != nil && !o.params.CustomFilter(details) {
				return
			}
			key, newEntry := o.params.SetDetails(details)
			o.keyToId[key] = v.ObjectDetailsSet.Id
			o.idToKey[v.ObjectDetailsSet.Id] = key

			if _, ok := o.sub[key]; !ok {
				if o.params.OnAdded != nil {
					o.params.OnAdded(v.ObjectDetailsSet.Id, newEntry)
				}
			}
			o.sub[key] = newEntry
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
