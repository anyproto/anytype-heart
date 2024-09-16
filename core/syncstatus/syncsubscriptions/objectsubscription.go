package syncsubscriptions

import (
	"context"
	"sync"

	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

type (
	extract[T any] func(*types.Struct) (string, T)
	update[T any]  func(string, *types.Value, T) T
	unset[T any]   func([]string, T) T
)

type SubscriptionParams[T any] struct {
	Request subscription.SubscribeRequest
	Extract extract[T]
	Update  update[T]
	Unset   unset[T]
}

func NewIdSubscription(service subscription.Service, request subscription.SubscribeRequest) *ObjectSubscription[struct{}] {
	return &ObjectSubscription[struct{}]{
		request: request,
		service: service,
		ch:      make(chan struct{}),
		extract: func(t *types.Struct) (string, struct{}) {
			return pbtypes.GetString(t, bundle.RelationKeyId.String()), struct{}{}
		},
		update: func(s string, value *types.Value, s2 struct{}) struct{} {
			return struct{}{}
		},
		unset: func(strings []string, s struct{}) struct{} {
			return struct{}{}
		},
	}
}

type ObjectSubscription[T any] struct {
	request subscription.SubscribeRequest
	service subscription.Service
	ch      chan struct{}
	events  *mb.MB[*pb.EventMessage]
	ctx     context.Context
	cancel  context.CancelFunc
	sub     map[string]*entry[T]
	extract extract[T]
	update  update[T]
	unset   unset[T]
	mx      sync.Mutex
}

func (o *ObjectSubscription[T]) Run() error {
	resp, err := o.service.Search(o.request)
	if err != nil {
		return err
	}
	o.ctx, o.cancel = context.WithCancel(context.Background())
	o.events = resp.Output
	o.sub = map[string]*entry[T]{}
	for _, rec := range resp.Records {
		id, data := o.extract(rec)
		o.sub[id] = newEntry(data)
	}
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
			o.sub[v.SubscriptionAdd.Id] = newEmptyEntry[T]()
		case *pb.EventMessageValueOfSubscriptionRemove:
			delete(o.sub, v.SubscriptionRemove.Id)
		case *pb.EventMessageValueOfObjectDetailsAmend:
			curEntry := o.sub[v.ObjectDetailsAmend.Id]
			if curEntry == nil {
				return
			}
			for _, value := range v.ObjectDetailsAmend.Details {
				curEntry.data = o.update(value.Key, value.Value, curEntry.data)
			}
		case *pb.EventMessageValueOfObjectDetailsUnset:
			curEntry := o.sub[v.ObjectDetailsUnset.Id]
			if curEntry == nil {
				return
			}
			curEntry.data = o.unset(v.ObjectDetailsUnset.Keys, curEntry.data)
		case *pb.EventMessageValueOfObjectDetailsSet:
			curEntry := o.sub[v.ObjectDetailsSet.Id]
			if curEntry == nil {
				return
			}
			_, curEntry.data = o.extract(v.ObjectDetailsSet.Details)
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
