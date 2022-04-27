package relation

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/ipfs/go-datastore/query"
)

const CName = "relation"

func New() Service {
	return new(service)
}

type Service interface {
	IdsToKeys(ids ...string) (keys []string, err error)
	KeysToIds(keys ...string) (ids []string, err error)
	CheckExistsId(id string) (err error)
	app.Component
}

type service struct {
	objectStore objectstore.ObjectStore
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) IdsToKeys(ids ...string) (keys []string, err error) {
	records, err := s.objectStore.QueryById(ids)
	if err != nil {
		return
	}
	keys = make([]string, 0, len(records))
	for _, rec := range records {
		if pbtypes.GetString(rec.Details, bundle.RelationKeyType.String()) != bundle.TypeKeyRelation.String() {
			continue
		}
		if key := pbtypes.GetString(rec.Details, bundle.RelationKeyRelationKey.String()); key != "" {
			keys = append(keys, key)
		}
	}
	return
}

func (s *service) KeysToIds(keys ...string) (ids []string, err error) {
	q := database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_In,
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Value:       pbtypes.StringList(keys),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyType.String(),
				Value:       pbtypes.String(bundle.TypeKeyRelation.String()),
			},
		},
	}
	f, err := database.NewFilters(q, nil)
	if err != nil {
		return
	}
	records, err := s.objectStore.QueryRaw(query.Query{
		Filters: []query.Filter{f},
	})
	if err != nil {
		return
	}
	ids = make([]string, 0, len(records))
	for _, rec := range records {
		if id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()); id != "" {
			ids = append(ids, id)
		}
	}
	return
}

func (s *service) CheckExistsId(id string) (err error) {
	//TODO implement me
	panic("implement me")
}
