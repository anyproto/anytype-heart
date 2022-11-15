package relation

import (
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const CName = "relation"

const blockServiceCName = "blockService"

var (
	ErrNotFound = errors.New("relation not found")
	ErrExists   = errors.New("relation with given key already exists")
	log         = logging.Logger("anytype-relations")
)

func New() Service {
	return new(service)
}

type Service interface {
	FetchKeys(keys ...string) (relations relationutils.Relations, err error)
	FetchKey(key string, opts ...FetchOption) (relation *relationutils.Relation, err error)
	FetchLinks(links pbtypes.RelationLinks) (relations relationutils.Relations, err error)
	CreateBulkMigration() BulkMigration
	MigrateRelations(relations []*model.Relation) error
	MigrateObjectTypes(relations []*model.ObjectType) error
	ValidateFormat(key string, v *types.Value) error
	app.Component
}

type BulkMigration interface {
	AddRelations(relations []*model.Relation)
	AddObjectTypes(objectType []*model.ObjectType)
	Commit() error
}

type subObjectCreator interface {
	CreateSubObjectInWorkspace(details *types.Struct, workspaceId string) (id string, newDetails *types.Struct, err error)
	CreateSubObjectsInWorkspace(details []*types.Struct) (ids []string, objects []*types.Struct, err error)
}

var errSubobjectAlreadyExists = fmt.Errorf("subobject already exists in the collection")

type bulkMigration struct {
	cache     map[string]struct{}
	s         subObjectCreator
	relations []*types.Struct
	options   []*types.Struct
	types     []*types.Struct
	mu        sync.Mutex
}

func (b *bulkMigration) AddRelations(relations []*model.Relation) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, rel := range relations {
		for _, opt := range rel.SelectDict {
			if _, exists := b.cache["opt"+opt.Id]; exists {
				continue
			}
			opt.RelationKey = rel.Key
			b.options = append(b.options, (&relationutils.Option{RelationOption: opt}).ToStruct())
			b.cache["opt"+opt.Id] = struct{}{}
		}
		if _, exists := b.cache["rel"+rel.Key]; exists {
			continue
		}
		b.relations = append(b.relations, (&relationutils.Relation{Relation: rel}).ToStruct())
		b.cache["rel"+rel.Key] = struct{}{}
	}
}

func (b *bulkMigration) AddObjectTypes(objectTypes []*model.ObjectType) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ot := range objectTypes {
		if _, exists := b.cache["type"+ot.Url]; exists {
			continue
		}
		b.types = append(b.types, (&relationutils.ObjectType{ObjectType: ot}).ToStruct())
		b.cache["type"+ot.Url] = struct{}{}
	}
}

func (b *bulkMigration) Commit() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.relations) > 0 {
		ids, _, err1 := b.s.CreateSubObjectsInWorkspace(b.relations)
		if len(ids) == 0 && (err1 == nil || err1.Error() != errSubobjectAlreadyExists.Error()) {
			log.Errorf("relations migration done %d/%d: %v", len(ids), len(b.relations), err1)
		}

		if err1 != nil && err1.Error() != errSubobjectAlreadyExists.Error() {
			return err1
		}
	}
	if len(b.options) > 0 {
		ids, _, err1 := b.s.CreateSubObjectsInWorkspace(b.options)
		if len(ids) == 0 && (err1 == nil || err1.Error() != errSubobjectAlreadyExists.Error()) {
			log.Errorf("options migration done %d/%d: %v", len(ids), len(b.relations), err1)
		}

		if err1 != nil && err1.Error() != errSubobjectAlreadyExists.Error() {
			return err1
		}
	}
	if len(b.types) > 0 {
		ids, _, err1 := b.s.CreateSubObjectsInWorkspace(b.types)
		if len(ids) == 0 && (err1 == nil || err1.Error() != errSubobjectAlreadyExists.Error()) {
			log.Errorf("types migration done %d/%d: %v", len(ids), len(b.relations), err1)
		}
		if err1 != nil && err1.Error() != errSubobjectAlreadyExists.Error() {
			return err1
		}
	}
	b.options = nil
	b.relations = nil
	b.types = nil

	return nil
}

type service struct {
	objectStore     objectstore.ObjectStore
	relationCreator subObjectCreator

	mu sync.RWMutex
}

func (s *service) MigrateRelations(relations []*model.Relation) error {
	b := s.CreateBulkMigration()
	b.AddRelations(relations)
	return b.Commit()
}

func (s *service) MigrateObjectTypes(types []*model.ObjectType) error {
	b := s.CreateBulkMigration()
	b.AddObjectTypes(types)
	return b.Commit()
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.relationCreator = a.MustComponent("objectCreator").(subObjectCreator)

	return
}

func (s *service) CreateBulkMigration() BulkMigration {
	return &bulkMigration{cache: map[string]struct{}{}, s: s.relationCreator}
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) FetchLinks(links pbtypes.RelationLinks) (relations relationutils.Relations, err error) {
	keys := make([]string, 0, len(links))
	for _, l := range links {
		keys = append(keys, l.Key)
	}
	return s.fetchKeys(keys...)
}

func (s *service) FetchKeys(keys ...string) (relations relationutils.Relations, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fetchKeys(keys...)
}

func (s *service) fetchKeys(keys ...string) (relations []*relationutils.Relation, err error) {
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, addr.RelationKeyToIdPrefix+key)
	}
	records, err := s.objectStore.QueryById(ids)
	if err != nil {
		return
	}

	for _, rec := range records {
		if pbtypes.GetString(rec.Details, bundle.RelationKeyType.String()) != bundle.TypeKeyRelation.URL() {
			continue
		}
		relations = append(relations, relationutils.RelationFromStruct(rec.Details))
	}
	return
}

type fetchOptions struct {
	workspaceId *string
}

type FetchOption func(options *fetchOptions)

func WithWorkspaceId(id string) FetchOption {
	return func(options *fetchOptions) {
		options.workspaceId = &id
	}
}

func (s *service) FetchKey(key string, opts ...FetchOption) (relation *relationutils.Relation, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fetchKey(key, opts...)
}

func (s *service) fetchKey(key string, opts ...FetchOption) (relation *relationutils.Relation, err error) {
	o := &fetchOptions{}
	for _, apply := range opts {
		apply(o)
	}
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
				Value:       pbtypes.String(bundle.TypeKeyRelation.URL()),
			},
		},
	}
	if o.workspaceId != nil {
		q.Filters = append(q.Filters, &model.BlockContentDataviewFilter{
			Condition:   model.BlockContentDataviewFilter_Equal,
			RelationKey: bundle.RelationKeyWorkspaceId.String(),
			Value:       pbtypes.String(*o.workspaceId),
		})
	}
	f, err := database.NewFilters(q, nil, nil)
	if err != nil {
		return
	}
	records, err := s.objectStore.QueryRaw(query.Query{
		Filters: []query.Filter{f},
	})
	for _, rec := range records {
		return relationutils.RelationFromStruct(rec.Details), nil
	}
	return nil, ErrNotFound
}

func (s *service) fetchOptionsByKey(key string) (relation *relationutils.Relation, err error) {
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
				Value:       pbtypes.String(bundle.TypeKeyRelationOption.String()),
			},
		},
	}
	f, err := database.NewFilters(q, nil, nil)
	if err != nil {
		return
	}
	records, err := s.objectStore.QueryRaw(query.Query{
		Filters: []query.Filter{f},
	})
	for _, rec := range records {
		return relationutils.RelationFromStruct(rec.Details), nil
	}
	return nil, ErrNotFound
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
		// if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		//	return fmt.Errorf("url scheme %s not supported", u.Scheme)
		// }
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

func (s *service) validateOptions(rel *relationutils.Relation, v []string) error {
	// TODO:
	return nil
}

func generateRelationKey() string {
	return bson.NewObjectId().Hex()
}
