package keyvalueobserver

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage/innerstorage"
	"github.com/cheggaaa/mb/v3"
)

const CName = keyvaluestorage.IndexerCName

type ObserverFunc func(decryptor keyvaluestorage.Decryptor, kvs []innerstorage.KeyValue)

type Observer interface {
	keyvaluestorage.Indexer
	SetObserver(observerFunc ObserverFunc)
}

func New() Observer {
	return &observer{}
}

type queueItem struct {
	decryptor keyvaluestorage.Decryptor
	keyValues []innerstorage.KeyValue
}

type observer struct {
	componentContext       context.Context
	componentContextCancel context.CancelFunc
	lock                   sync.RWMutex

	observerFunc ObserverFunc
	updateQueue  *mb.MB[queueItem]
}

func (o *observer) Init(a *app.App) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	o.componentContext = ctx
	o.componentContextCancel = cancel
	o.updateQueue = mb.New[queueItem](0)
	return nil
}

func (o *observer) Run(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-o.componentContext.Done():
				return
			default:
			}

			item, err := o.updateQueue.WaitOne(o.componentContext)
			if errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, mb.ErrClosed) {
				return
			}

			o.lock.RLock()
			obs := o.observerFunc
			o.lock.RUnlock()

			if obs != nil {
				obs(item.decryptor, item.keyValues)
			}
		}
	}()
	return nil
}

func (o *observer) Close(ctx context.Context) (err error) {
	if o.componentContextCancel != nil {
		o.componentContextCancel()
	}
	return nil
}

func (o *observer) Name() (name string) {
	return CName
}

func (o *observer) SetObserver(observerFunc ObserverFunc) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.observerFunc = observerFunc
}

func (o *observer) Index(decryptor keyvaluestorage.Decryptor, keyValue ...innerstorage.KeyValue) error {
	return o.updateQueue.Add(o.componentContext, queueItem{decryptor: decryptor, keyValues: keyValue})
}
