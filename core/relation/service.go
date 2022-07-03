package relation

import (
	"context"
	"errors"
	"fmt"
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
	"net/url"
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
	FetchIds(ids ...string) (relations Relations, err error)
	FetchId(id string) (relation *Relation, err error)
	FetchKey(key string) (relation *Relation, err error)
	FetchLinks(links pbtypes.RelationLinks) (relations Relations, err error)

	Create(rel *model.Relation) (rl *model.RelationLink, err error)

	ValidateFormat(key string, v *types.Value) error
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

func (s *service) FetchLinks(links pbtypes.RelationLinks) (relations Relations, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.Id)
	}
	return s.fetchIds(ids...)
}

func (s *service) FetchIds(ids ...string) (relations Relations, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fetchIds(ids...)
}

func (s *service) fetchIds(ids ...string) (relations []*Relation, err error) {
	records, err := s.objectStore.QueryById(ids)
	if err != nil {
		return
	}
	relations = make(Relations, 0, len(records))
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

func (s *service) fetchOptionsKey(key string) (relation *Relation, err error) {
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
	id, _, err := s.objectCreator.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeIndexedRelation, nil, nil, st)
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

func (s *service) ValidateFormat(key string, v *types.Value) error {
	r, err := s.FetchKey(key)
	if err != nil {
		return err
	}
	if _, isNull := v.Kind.(*types.Value_NullValue); isNull {
		// allow null value for any field
		return nil
	}

	switch r.Format {
	case model.RelationFormat_longtext, model.RelationFormat_shorttext:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}
		return nil
	case model.RelationFormat_number:
		if _, ok := v.Kind.(*types.Value_NumberValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of number", v.Kind)
		}
		return nil
	case model.RelationFormat_status:
		if _, ok := v.Kind.(*types.Value_StringValue); ok {

		} else if _, ok := v.Kind.(*types.Value_ListValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of list", v.Kind)
		}

		vals := pbtypes.GetStringListValue(v)
		if len(vals) > 1 {
			return fmt.Errorf("status should not contain more than one value")
		}
		return s.validateOptions(r, vals)

	case model.RelationFormat_tag:
		if _, ok := v.Kind.(*types.Value_ListValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of list", v.Kind)
		}

		vals := pbtypes.GetStringListValue(v)
		if r.MaxCount > 0 && len(vals) > int(r.MaxCount) {
			return fmt.Errorf("maxCount exceeded")
		}

		return s.validateOptions(r, vals)
	case model.RelationFormat_date:
		if _, ok := v.Kind.(*types.Value_NumberValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of number", v.Kind)
		}

		return nil
	case model.RelationFormat_file, model.RelationFormat_object:
		switch s := v.Kind.(type) {
		case *types.Value_StringValue:
			if r.MaxCount != 1 {
				return fmt.Errorf("incorrect type: %T instead of list(maxCount!=1)", v.Kind)
			}
			return nil
		case *types.Value_ListValue:
			if r.MaxCount > 0 && len(s.ListValue.Values) > int(r.MaxCount) {
				return fmt.Errorf("relation %s(%s) has maxCount exceeded", r.Key, r.Format.String())
			}

			for i, lv := range s.ListValue.Values {
				if optId, ok := lv.Kind.(*types.Value_StringValue); !ok {
					return fmt.Errorf("incorrect list item value at index %d: %T instead of string", i, lv.Kind)
				} else if optId.StringValue == "" {
					return fmt.Errorf("empty option at index %d", i)
				}
			}
			return nil
		default:
			return fmt.Errorf("incorrect type: %T instead of list/string", v.Kind)
		}
	case model.RelationFormat_checkbox:
		if _, ok := v.Kind.(*types.Value_BoolValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of bool", v.Kind)
		}

		return nil
	case model.RelationFormat_url:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}

		_, err := url.Parse(v.GetStringValue())
		if err != nil {
			return fmt.Errorf("failed to parse URL: %s", err.Error())
		}
		// todo: should we allow schemas other than http/https?
		//if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		//	return fmt.Errorf("url scheme %s not supported", u.Scheme)
		//}
		return nil
	case model.RelationFormat_email:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}
		// todo: revise regexp and reimplement
		/*valid := uri.ValidateEmail(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate email")
		}*/
		return nil
	case model.RelationFormat_phone:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}

		// todo: revise regexp and reimplement
		/*valid := uri.ValidatePhone(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate phone")
		}*/
		return nil
	case model.RelationFormat_emoji:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}

		// check if the symbol is emoji
		return nil
	default:
		return fmt.Errorf("unsupported rel format: %s", r.Format.String())
	}
}

func (s *service) validateOptions(rel *Relation, v []string) error {
	//TODO:
	return nil
}
