package relation

import (
	"context"
	"errors"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"
	"sync"
)

const CName = "relation"

const blockServiceCName = "blockService"

var (
	ErrNotFound = errors.New("relation not found")
	ErrExists   = errors.New("relation with given key already exists")
)

func New() Service {
	return new(service)
}

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State) (id string, newDetails *types.Struct, err error)
}

type Service interface {
	FetchIds(ids ...string) (relations []*Relation, err error)
	FetchId(id string) (relation *Relation, err error)
	Create(rel *model.Relation) (rl *model.RelationLink, err error)
	app.Component
}

type service struct {
	objectStore   objectstore.ObjectStore
	objectCreator objectCreator
	mu            sync.RWMutex

	migrateCache map[string]*model.RelationLink
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.objectCreator = a.MustComponent(blockServiceCName).(objectCreator)
	s.migrateCache = make(map[string]*model.RelationLink)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) FetchIds(ids ...string) (relations []*Relation, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fetchIds(ids...)
}

func (s *service) fetchIds(ids ...string) (relations []*Relation, err error) {
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

func (s *service) FetchKey(key string) (relation *Relation, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fetchKey(key)
}

func (s *service) fetchKey(key string) (relation *Relation, err error) {
	q := database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Value:       pbtypes.String(key),
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
	for _, rec := range records {
		return RelationFromStruct(rec.Details), nil
	}
	return nil, ErrNotFound
}

func (s *service) Create(rel *model.Relation) (rl *model.RelationLink, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.create(rel, true)
}

func (s *service) create(rel *model.Relation, checkForExists bool) (rl *model.RelationLink, err error) {
	if checkForExists {
		if _, e := s.fetchKey(rel.Key); e != ErrNotFound {
			return nil, ErrExists
		}
	}
	st := state.NewDoc("", nil).(*state.State)
	st.SetObjectType(bundle.TypeKeyRelation.URL())
	r := &Relation{Relation: rel}
	details := r.ToStruct()
	for k, v := range details.Fields {
		st.SetDetailAndBundledRelation(bundle.RelationKey(k), v)
	}
	id, _, err := s.objectCreator.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeBundledRelation, nil, nil, st)
	if err != nil {
		return
	}
	return &model.RelationLink{
		Id:  id,
		Key: rel.Key,
	}, nil
}

func (s *service) MigrateRelations(rels []*model.Relation) (relLinks []*model.RelationLink, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	relLinks = make([]*model.RelationLink, 0, len(rels))

	for _, rel := range rels {
		link, ok := s.migrateCache[rel.Key]
		if !ok {
			link, err = s.migrateRelation(rel)
			if err != nil {
				return
			}
			s.migrateCache[rel.Key] = link
		}
		relLinks = append(relLinks, link)
		if len(rel.SelectDict) > 0 {
			if err = s.migrateOptions(rel); err != nil {
				return
			}
		}
	}
	return
}

func (s *service) migrateRelation(rel *model.Relation) (rl *model.RelationLink, err error) {
	dbRel, e := s.fetchKey(rel.Key)
	if e == nil {
		return dbRel.RelationLink(), nil
	}
	return s.create(rel, false)
}

func (s *service) migrateOptions(rel *model.Relation) (err error) {

	return
}
