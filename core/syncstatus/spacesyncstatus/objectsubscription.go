package spacesyncstatus

import (
	"context"
	"sync"

	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"
	"github.com/huandu/skiplist"

	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type entry[T any] struct {
	id   string
	data T
}

func newEmptyEntry[T any](id string) *entry[T] {
	return &entry[T]{id: id}
}

func newEntry[T any](id string, data T) *entry[T] {
	return &entry[T]{id: id, data: data}
}

type (
	extract[T any] func(*types.Struct) (string, T)
	compare[T any] func(T, T) int
	update[T any]  func(string, *types.Value, T) T
	unset[T any]   func([]string, T) T
)

type SubscriptionParams[T any] struct {
	Request subscription.SubscribeRequest
	Extract extract[T]
	Order   compare[T]
	Update  update[T]
	Unset   unset[T]
}

func NewIdSubscription(service subscription.Service, request subscription.SubscribeRequest) *ObjectSubscription[struct{}] {
	return &ObjectSubscription[struct{}]{
		request: request,
		service: service,
		ch:      make(chan struct{}),
		order:   nil,
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

func NewObjectSubscription[T any](service subscription.Service, params SubscriptionParams[T]) *ObjectSubscription[T] {
	return &ObjectSubscription[T]{
		request: params.Request,
		service: service,
		ch:      make(chan struct{}),
		order:   params.Order,
		extract: params.Extract,
		update:  params.Update,
		unset:   params.Unset,
	}
}

type ObjectSubscription[T any] struct {
	request subscription.SubscribeRequest
	service subscription.Service
	ch      chan struct{}
	events  *mb.MB[*pb.EventMessage]
	ctx     context.Context
	cancel  context.CancelFunc
	skl     *skiplist.SkipList
	order   compare[T]
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
	o.skl = skiplist.New(o)
	for _, rec := range resp.Records {
		id, data := o.extract(rec)
		e := &entry[T]{id: id, data: data}
		o.skl.Set(e, nil)
	}
	go o.read()
	return nil
}

func (o *ObjectSubscription[T]) Close() {
	o.cancel()
	<-o.ch
	return
}

func (o *ObjectSubscription[T]) Len() int {
	o.mx.Lock()
	defer o.mx.Unlock()
	return o.skl.Len()
}

func (o *ObjectSubscription[T]) Iterate(iter func(id string, data T) bool) {
	o.mx.Lock()
	defer o.mx.Unlock()
	cur := o.skl.Front()
	for cur != nil {
		el := cur.Key().(*entry[T])
		if !iter(el.id, el.data) {
			return
		}
		cur = cur.Next()
	}
	return
}

func (o *ObjectSubscription[T]) Compare(lhs, rhs interface{}) (comp int) {
	le := lhs.(*entry[T])
	re := rhs.(*entry[T])
	if le.id == re.id {
		return 0
	}
	if o.order != nil {
		comp = o.order(le.data, re.data)
	}
	if comp == 0 {
		if le.id > re.id {
			return 1
		} else {
			return -1
		}
	}
	return comp
}

func (o *ObjectSubscription[T]) CalcScore(key interface{}) float64 {
	return 0
}

func (o *ObjectSubscription[T]) read() {
	defer close(o.ch)
	readEvent := func(event *pb.EventMessage) {
		o.mx.Lock()
		defer o.mx.Unlock()
		switch v := event.Value.(type) {
		case *pb.EventMessageValueOfSubscriptionAdd:
			o.skl.Set(newEmptyEntry[T](v.SubscriptionAdd.Id), nil)
		case *pb.EventMessageValueOfSubscriptionRemove:
			o.skl.Remove(newEmptyEntry[T](v.SubscriptionRemove.Id))
		case *pb.EventMessageValueOfObjectDetailsAmend:
			curEntry := o.skl.Get(newEmptyEntry[T](v.ObjectDetailsAmend.Id))
			if curEntry == nil {
				return
			}
			e := curEntry.Key().(*entry[T])
			for _, value := range v.ObjectDetailsAmend.Details {
				e.data = o.update(value.Key, value.Value, e.data)
			}
		case *pb.EventMessageValueOfObjectDetailsUnset:
			curEntry := o.skl.Get(newEmptyEntry[T](v.ObjectDetailsUnset.Id))
			if curEntry == nil {
				return
			}
			e := curEntry.Key().(*entry[T])
			e.data = o.unset(v.ObjectDetailsUnset.Keys, e.data)
		case *pb.EventMessageValueOfObjectDetailsSet:
			curEntry := o.skl.Get(newEmptyEntry[T](v.ObjectDetailsSet.Id))
			if curEntry == nil {
				return
			}
			e := curEntry.Key().(*entry[T])
			_, e.data = o.extract(v.ObjectDetailsSet.Details)
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
