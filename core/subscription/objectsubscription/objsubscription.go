package objectsubscription

import (
	"context"
	"fmt"
	"sync"

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
}

type ObjectSubscription[T any] struct {
	request    subscription.SubscribeRequest
	service    subscription.Service
	ch         chan struct{}
	events     *mb.MB[*pb.EventMessage]
	filterKeys map[string]struct{}
	ctx        context.Context
	cancel     context.CancelFunc

	params SubscriptionParams[T]

	mx  sync.Mutex
	sub map[string]T
}

var IdSubscriptionParams = SubscriptionParams[struct{}]{
	SetDetails: func(t *domain.Details) (string, struct{}) {
		return t.GetString(bundle.RelationKeyId), struct{}{}
	},
	UpdateKeys: func(keyValues []RelationKeyValue, s2 struct{}) struct{} {
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
	return &ObjectSubscription[T]{
		request:    req,
		service:    subService,
		filterKeys: make(map[string]struct{}),
		ch:         make(chan struct{}),
		params:     params,
	}
}

// NewFromQueue creates an ObjectSubscription from an event queue with optional initial records.
// Use this when AsyncInit isn't available (e.g., with crossspacesub) and you need to track
// existing objects that were returned by the initial subscription response.
// Without initialRecords, the subscription will only track objects that appear after Run() is called.
func NewFromQueue[T any](queue *mb.MB[*pb.EventMessage], params SubscriptionParams[T], initialRecords []*domain.Details) *ObjectSubscription[T] {
	o := &ObjectSubscription[T]{
		events: queue,
		ch:     make(chan struct{}),
		params: params,
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
	if o.service == nil && o.events == nil {
		return fmt.Errorf("subscription created with nil event queue")
	}
	if o.params.SetDetails == nil {
		return fmt.Errorf("SetDetails function not set")
	}
	if o.params.UpdateKeys == nil {
		return fmt.Errorf("UpdateKeys function not set")
	}
	if o.params.RemoveKeys == nil {
		return fmt.Errorf("RemoveKeys function not set")
	}

	o.request.Internal = true
	if o.sub == nil {
		o.sub = map[string]T{}
	}
	if o.service != nil {
		resp, err := o.service.Search(o.request)
		if err != nil {
			return err
		}
		for _, key := range o.request.Keys {
			o.filterKeys[key] = struct{}{}
		}
		for _, rec := range resp.Records {
			id, data := o.params.SetDetails(rec)
			o.sub[id] = data
		}
		o.events = resp.Output
	}
	o.ctx, o.cancel = context.WithCancel(context.Background())
	go o.read()
	return nil
}

func (o *ObjectSubscription[T]) Close() {
	o.cancel()
	<-o.ch
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
			_, newEntry := o.params.SetDetails(domain.NewDetailsFromProto(v.ObjectDetailsSet.Details))
			if _, ok := o.sub[v.ObjectDetailsSet.Id]; !ok {
				if o.params.OnAdded != nil {
					o.params.OnAdded(v.ObjectDetailsSet.Id, newEntry)
				}
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
