package relation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/treegetter"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"

	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
)

const CName = "relation"

const blockServiceCName = treegetter.CName

var (
	ErrNotFound = errors.New("relation not found")
	log         = logging.Logger("anytype-relations")
)

func New() Service {
	return new(service)
}

type Service interface {
	FetchKeys(keys ...string) (relations relationutils.Relations, err error)
	FetchKey(key string, opts ...FetchOption) (relation *relationutils.Relation, err error)
	ListAll(opts ...FetchOption) (relations relationutils.Relations, err error)

	FetchLinks(links pbtypes.RelationLinks) (relations relationutils.Relations, err error)
	ValidateFormat(key string, v *types.Value) error
	app.Component
}
type subObjectCreator interface {
	CreateSubObjectInWorkspace(details *types.Struct, workspaceId string) (id string, newDetails *types.Struct, err error)
	CreateSubObjectsInWorkspace(details []*types.Struct) (ids []string, objects []*types.Struct, err error)
}

type service struct {
	objectStore     objectstore.ObjectStore
	relationCreator subObjectCreator
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.relationCreator = a.MustComponent("objectCreator").(subObjectCreator)

	return
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

func (s *service) ListAll(opts ...FetchOption) (relations relationutils.Relations, err error) {
	return s.listAll(opts...)
}

func (s *service) listAll(opts ...FetchOption) (relations relationutils.Relations, err error) {
	filters := []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyType.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(bundle.TypeKeyRelation.URL()),
		},
	}
	o := &fetchOptions{}
	for _, apply := range opts {
		apply(o)
	}
	if o.workspaceId != nil {
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyWorkspaceId.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(*o.workspaceId),
		})
	}

	relations2, _, err := s.objectStore.Query(nil, database.Query{
		Filters: filters,
	})
	if err != nil {
		return
	}

	for _, rec := range relations2 {
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
	f, err := database.NewFilters(q, nil, s.objectStore, nil)
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

		s := strings.TrimSpace(v.GetStringValue())
		if s != "" {
			err := uri.ValidateURI(strings.TrimSpace(v.GetStringValue()))
			if err != nil {
				return fmt.Errorf("failed to parse URL: %s", err.Error())
			}
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
