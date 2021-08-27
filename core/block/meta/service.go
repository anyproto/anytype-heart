package meta

import (
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/util/slice"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const CName = "meta"

type Meta struct {
	BlockId string
	core.SmartBlockMeta
}

type Service interface {
	PubSub() PubSub

	ReportChange(m Meta)
	FetchMeta(ids []string) (metas []Meta)
	FetchObjectTypes(objectTypeUrls []string) []*model.ObjectType
	app.ComponentRunnable
}

func New() Service {
	return new(service)
}

type service struct {
	anytype core.Service
	indexer indexer.Indexer
	ps      *pubSub
	m       sync.Mutex
}


// SetLocalDetails inject local details into the meta pubsub
func (s *service) SetLocalDetails(id string, st *types.Struct) {
	s.ps.m.Lock()
	defer s.ps.m.Unlock()
	if c, ok := s.ps.collectors[id]; ok {
		m := copyMeta(c.GetMeta())
		for k, v := range st.GetFields() {
			if slice.FindPos(bundle.LocalRelationsKeys, k) > -1 {
				m.Details.Fields[k] = v
			}
		}
		c.setMeta(m)
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.indexer = a.MustComponent(indexer.CName).(indexer.Indexer)
	s.ps = newPubSub(s.anytype, a.MustComponent(source.CName).(source.Service))
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run() (err error) {
	var newSmartblockCh = make(chan string)
	if err := s.anytype.InitNewSmartblocksChan(newSmartblockCh); err != nil {
		log.Errorf("can't init new smartblock chan: %v", err)
	} else {
		go s.newSmartblockListener(newSmartblockCh)
	}
	return
}

func (s *service) PubSub() PubSub {
	return s.ps
}

func (s *service) ReportChange(m Meta) {
	m = copyMeta(m)
	s.ps.setMeta(m)
}

func (s *service) FetchMeta(ids []string) (metas []Meta) {
	if len(ids) == 0 {
		return
	}
	var (
		filled = make(chan struct{})
		done   bool
		m      sync.Mutex
	)
	sub := s.PubSub().NewSubscriber().Callback(func(d Meta) {
		m.Lock()
		defer m.Unlock()
		if done {
			return
		}
		metas = append(metas, d)
		if len(metas) == len(ids) {
			close(filled)
			done = true
		}
	}).Subscribe(ids...)
	defer sub.Close()
	select {
	case <-time.After(time.Second):
	case <-filled:
	}
	return
}

func (s *service) FetchObjectTypes(objectTypeUrls []string) []*model.ObjectType {
	if len(objectTypeUrls) == 0 {
		return nil
	}
	var objectTypes = []*model.ObjectType{}
	for _, otypeUrl := range objectTypeUrls {
		ot, err := objectstore.GetObjectType(s.anytype.ObjectStore(), otypeUrl)
		if err != nil {
			log.Errorf("FetchObjectTypes failed to get objectType %s", otypeUrl)
			continue
		}
		objectTypes = append(objectTypes, ot)
	}

	return objectTypes
}

func (s *service) newSmartblockListener(ch chan string) {
	for newId := range ch {
		s.ps.onNewThread(newId)
	}
}

func (s *service) Close() (err error) {
	return s.ps.Close()
}
