package meta

import (
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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
	ps      *pubSub
	m       sync.Mutex
}

func (s *service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.ps = newPubSub(s.anytype, a.MustComponent(status.CName).(status.Service))
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
	var customOtypeIds = []string{}
	for _, otypeUrl := range objectTypeUrls {
		if strings.HasPrefix(otypeUrl, objects.BundledObjectTypeURLPrefix) {
			var err error
			objectType, err := bundle.GetTypeByUrl(otypeUrl)
			if err != nil {
				log.Errorf("failed to get objectType '%s': %s", otypeUrl, err.Error())
				continue
			}
			objectTypes = append(objectTypes, objectType)
		} else if !strings.HasPrefix(otypeUrl, "b") {
			log.Errorf("failed to get objectType %s: incorrect url", otypeUrl)
		} else {
			customOtypeIds = append(customOtypeIds, otypeUrl)
		}
	}

	if len(customOtypeIds) == 0 {
		return objectTypes
	}

	metas := s.FetchMeta(customOtypeIds)
	for _, meta := range metas {
		objectType := &model.ObjectType{}
		if name := pbtypes.GetString(meta.Details, bundle.RelationKeyName.String()); name != "" {
			objectType.Name = name
		}
		if layout := pbtypes.GetFloat64(meta.Details, bundle.RelationKeyRecommendedLayout.String()); layout != 0.0 {
			objectType.Layout = model.ObjectTypeLayout(int(layout))
		}

		if iconEmoji := pbtypes.GetString(meta.Details, bundle.RelationKeyIconEmoji.String()); iconEmoji != "" {
			objectType.IconEmoji = iconEmoji
		}

		recommendedRelationsKeys := pbtypes.GetStringList(meta.Details, bundle.RelationKeyRecommendedRelations.String())
		for _, rel := range bundle.RequiredInternalRelations {
			if slice.FindPos(recommendedRelationsKeys, rel.String()) == -1 {
				recommendedRelationsKeys = append(recommendedRelationsKeys, rel.String())
			}
		}

		var recommendedRelations []*model.Relation
		for _, rk := range recommendedRelationsKeys {
			rel := pbtypes.GetRelation(meta.Relations, rk)
			if rel == nil {
				rel, _ = bundle.GetRelation(bundle.RelationKey(rk))
				if rel == nil {
					continue
				}
			}

			relCopy := pbtypes.CopyRelation(rel)
			relCopy.Scope = model.Relation_type
			recommendedRelations = append(recommendedRelations, relCopy)
		}

		objectType.Url = meta.BlockId
		objectType.Relations = recommendedRelations

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
