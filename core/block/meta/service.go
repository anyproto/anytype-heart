package meta

import (
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type Meta struct {
	BlockId string
	core.SmartBlockMeta
}

type Service interface {
	PubSub() PubSub
	ReportChange(m Meta)
	Close() (err error)
	FetchMeta(ids []string) (details []Meta)
	FetchObjectTypes(objectTypeUrls []string) []*pbrelation.ObjectType
}

func NewService(a anytype.Service, ss status.Service) Service {
	s := &service{
		ps: newPubSub(a, ss),
	}
	var newSmartblockCh = make(chan string)
	if err := a.InitNewSmartblocksChan(newSmartblockCh); err != nil {
		log.Errorf("can't init new smartblock chan: %v", err)
	} else {
		go s.newSmartblockListener(newSmartblockCh)
	}
	return s
}

type service struct {
	ps *pubSub
	m  sync.Mutex
}

func (s *service) PubSub() PubSub {
	return s.ps
}

func (s *service) ReportChange(m Meta) {
	m = copyMeta(m)
	s.ps.setMeta(m)
}

func (s *service) FetchMeta(ids []string) (details []Meta) {
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
		details = append(details, d)
		if len(details) == len(ids) {
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

func (s *service) FetchObjectTypes(objectTypeUrls []string) []*pbrelation.ObjectType {
	if len(objectTypeUrls) == 0 {
		return nil
	}
	var objectTypes = []*pbrelation.ObjectType{}
	var customOtypeIds = []string{}
	for _, otypeUrl := range objectTypeUrls {
		if strings.HasPrefix(otypeUrl, objects.BundledObjectTypeURLPrefix) {
			var err error
			objectType, err := bundle.GetTypeByUrl(otypeUrl)
			if err != nil {
				log.Errorf("failed to get objectType %s: %s", otypeUrl, err.Error())
				continue
			}
			objectTypes = append(objectTypes, objectType)
		} else if !strings.HasPrefix(otypeUrl, objects.CustomObjectTypeURLPrefix) {
			log.Errorf("failed to get objectType %s: incorrect url", otypeUrl)
		} else {
			customOtypeIds = append(customOtypeIds, strings.TrimPrefix(otypeUrl, objects.CustomObjectTypeURLPrefix))
		}
	}

	if len(customOtypeIds) == 0 {
		return objectTypes
	}

	metas := s.FetchMeta(customOtypeIds)
	for _, meta := range metas {
		objectType := &pbrelation.ObjectType{}
		if name := pbtypes.GetString(meta.Details, "name"); name != "" {
			objectType.Name = name
		}
		if layout := pbtypes.GetFloat64(meta.Details, "layout"); layout != 0.0 {
			objectType.Layout = pbrelation.ObjectTypeLayout(int(layout))
		}

		if iconEmoji := pbtypes.GetString(meta.Details, "iconEmoji"); iconEmoji != "" {
			objectType.IconEmoji = iconEmoji
		}

		objectType.Url = objects.CustomObjectTypeURLPrefix + meta.BlockId
		objectType.Relations = meta.Relations

		objectTypes = append(objectTypes, objectType)
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
