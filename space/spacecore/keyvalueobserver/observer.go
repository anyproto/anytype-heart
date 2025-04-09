package keyvalueobserver

import (
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage"
	"github.com/anyproto/any-sync/commonspace/object/keyvalue/keyvaluestorage/innerstorage"
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

type observer struct {
	lock sync.RWMutex

	observerFunc ObserverFunc
}

func (o *observer) Init(a *app.App) (err error) {
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
	o.lock.RLock()
	obs := o.observerFunc
	o.lock.RUnlock()

	if obs != nil {
		obs(decryptor, keyValue)
	}
	return nil
}
