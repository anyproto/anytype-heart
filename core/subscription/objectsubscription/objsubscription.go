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

type entry[T any] struct {
	data T
}

func newEmptyEntry[T any]() *entry[T] {
	return &entry[T]{}
}

func newEntry[T any](data T) *entry[T] {
	return &entry[T]{data: data}
}

type SubscriptionParams[T any] struct {
	// OnDetails called when details appears in subscription for the first time
	// It's mandatory
	OnDetailsSet func(details *domain.Details) (id string, entry T)
	// OnKeyUpdate called when value for given key is updated
	// It's mandatory
	OnKeyUpdated func(relationKey string, relationValue domain.Value, curEntry T) (updatedEntry T)
	// OnKeysRemoved called when specified keys are removed from the details
	// It's mandatory
	OnKeysRemoved func(keys []string, curEntry T) (updatedEntry T)

	// OnAdded called when object appears in subscription
	OnAdded func(id string)
	// OnRemove called when object is removed from subscription
	OnRemoved func(id string)
}

type ObjectSubscription[T any] struct {
	request subscription.SubscribeRequest
	service subscription.Service
	ch      chan struct{}
	events  *mb.MB[*pb.EventMessage]
	ctx     context.Context
	cancel  context.CancelFunc

	params SubscriptionParams[T]

	mx  sync.Mutex
	sub map[string]*entry[T]
}

var IdSubscriptionParams = SubscriptionParams[struct{}]{
	OnDetailsSet: func(t *domain.Details) (string, struct{}) {
		return t.GetString(bundle.RelationKeyId), struct{}{}
	},
	OnKeyUpdated: func(s string, value domain.Value, s2 struct{}) struct{} {
		return struct{}{}
	},
	OnKeysRemoved: func(strings []string, s struct{}) struct{} {
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
	return &ObjectSubscription[T]{
		request: req,
		service: subService,
		ch:      make(chan struct{}),
		params:  params,
	}
}

func NewFromQueue[T any](queue *mb.MB[*pb.EventMessage], params SubscriptionParams[T]) *ObjectSubscription[T] {
	return &ObjectSubscription[T]{
		events: queue,
		ch:     make(chan struct{}),
		params: params,
	}
}

func (o *ObjectSubscription[T]) Run() error {
	if o.service == nil && o.events == nil {
		return fmt.Errorf("subscription created with nil event queue")
	}
	if o.params.OnDetailsSet == nil {
		return fmt.Errorf("OnDetailsSet function not set")
	}
	if o.params.OnKeyUpdated == nil {
		return fmt.Errorf("OnKeyUpdated function not set")
	}
	if o.params.OnKeysRemoved == nil {
		return fmt.Errorf("OnKeysRemoved function not set")
	}

	o.request.Internal = true
	o.sub = map[string]*entry[T]{}
	if o.service != nil {
		resp, err := o.service.Search(o.request)
		if err != nil {
			return err
		}
		for _, rec := range resp.Records {
			id, data := o.params.OnDetailsSet(rec)
			o.sub[id] = newEntry(data)
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
	if !ok {
		var defVal T
		return defVal, false
	}
	return entry.data, true
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
		if !iter(id, ent.data) {
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
			if o.params.OnAdded != nil {
				o.params.OnAdded(v.SubscriptionAdd.Id)
			}
			o.sub[v.SubscriptionAdd.Id] = newEmptyEntry[T]()
		case *pb.EventMessageValueOfSubscriptionRemove:
			delete(o.sub, v.SubscriptionRemove.Id)
			if o.params.OnRemoved != nil {
				o.params.OnRemoved(v.SubscriptionRemove.Id)
			}
		case *pb.EventMessageValueOfObjectDetailsAmend:
			curEntry := o.sub[v.ObjectDetailsAmend.Id]
			if curEntry == nil {
				return
			}
			for _, value := range v.ObjectDetailsAmend.Details {
				curEntry.data = o.params.OnKeyUpdated(value.Key, domain.ValueFromProto(value.Value), curEntry.data)
			}
		case *pb.EventMessageValueOfObjectDetailsUnset:
			curEntry := o.sub[v.ObjectDetailsUnset.Id]
			if curEntry == nil {
				return
			}
			curEntry.data = o.params.OnKeysRemoved(v.ObjectDetailsUnset.Keys, curEntry.data)
		case *pb.EventMessageValueOfObjectDetailsSet:
			curEntry := o.sub[v.ObjectDetailsSet.Id]
			if curEntry == nil {
				return
			}
			// TODO Think about using domain layer structs for events with domain.Details inside
			_, curEntry.data = o.params.OnDetailsSet(domain.NewDetailsFromProto(v.ObjectDetailsSet.Details))
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
