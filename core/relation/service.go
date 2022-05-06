package relation

import (
	"errors"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/ipfs/go-datastore/query"
)

const CName = "relation"

var (
	ErrNotFound = errors.New("relation not found")
)

func New() Service {
	return new(service)
}

type Service interface {
	FetchIds(ids ...string) (relations []*Relation, err error)
	FetchId(id string) (relation *Relation, err error)
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

func (s *service) FetchIds(ids ...string) (relations []*Relation, err error) {
	records, err := s.objectStore.QueryById(ids)
	if err != nil {
		return
	}
	relations = make([]*Relation, 0, len(records))
	for _, rec := range records {
		if pbtypes.GetString(rec.Details, bundle.RelationKeyType.String()) != bundle.TypeKeyRelation.String() {
			continue
		}
		relations = append(relations, RelationFromStruct(rec.Details))
	}
	return
}

func (s *service) FetchId(id string) (relation *Relation, err error) {
	rels, err := s.FetchIds(id)
	if err != nil {
		return
	}
	if len(rels) == 0 {
		return nil, ErrNotFound
	}
	return rels[0], nil
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
