package relation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"
)

const CName = "relation"

const blockServiceCName = treemanager.CName

var (
	ErrNotFound = errors.New("relation not found")
	log         = logging.Logger("anytype-relations")
)

func New() Service {
	return new(service)
}

type Service interface {
	FetchKeys(spaceId string, keys ...string) (relations relationutils.Relations, err error)
	FetchKey(spaceId string, key string, opts ...FetchOption) (relation *relationutils.Relation, err error)
	ListAll(spaceId string, opts ...FetchOption) (relations relationutils.Relations, err error)
	GetRelationId(ctx context.Context, spaceId string, key bundle.RelationKey) (id string, err error)
	// GetSystemTypeId is the optimized version of GetTypeId,
	// cause all system types are precalculated
	GetSystemTypeId(spaceId string, key bundle.TypeKey) (id string, err error)
	GetTypeId(ctx context.Context, spaceId string, key bundle.TypeKey) (id string, err error)

	FetchLinks(spaceId string, links pbtypes.RelationLinks) (relations relationutils.Relations, err error)
	ValidateFormat(spaceId string, key string, v *types.Value) error
	app.Component
}

type service struct {
	objectStore objectstore.ObjectStore
	core        core.Service
}

func (s *service) GetTypeId(ctx context.Context, spaceId string, key bundle.TypeKey) (id string, err error) {
	uk, err := uniquekey.NewUniqueKey(model.SmartBlockType_STType, key.String())
	if err != nil {
		return "", err
	}

	return s.core.DeriveObjectId(ctx, spaceId, uk)
}

func (s *service) GetSystemTypeId(spaceId string, key bundle.TypeKey) (id string, err error) {
	predefined := s.core.PredefinedObjects(spaceId)
	if len(predefined.SystemTypes) == 0 {
		return "", fmt.Errorf("predefined not found for the space")
	}
	if v, ok := predefined.SystemTypes[key]; !ok {
		return "", fmt.Errorf("system type not found")
	} else {
		return v, nil
	}
}

func (s *service) GetRelationId(ctx context.Context, spaceId string, key bundle.RelationKey) (id string, err error) {
	uk, err := uniquekey.NewUniqueKey(model.SmartBlockType_STRelation, key.String())
	if err != nil {
		return "", err
	}

	return s.core.DeriveObjectId(ctx, spaceId, uk)
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.core = a.MustComponent(core.CName).(core.Service)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) FetchLinks(spaceId string, links pbtypes.RelationLinks) (relations relationutils.Relations, err error) {
	keys := make([]string, 0, len(links))
	for _, l := range links {
		keys = append(keys, l.Key)
	}
	return s.fetchKeys(spaceId, keys...)
}

func (s *service) FetchKeys(spaceId string, keys ...string) (relations relationutils.Relations, err error) {
	return s.fetchKeys(spaceId, keys...)
}

func (s *service) fetchKeys(spaceId string, keys ...string) (relations []*relationutils.Relation, err error) {
	uks := make([]string, 0, len(keys))

	for _, key := range keys {
		uk, err := uniquekey.NewUniqueKey(model.SmartBlockType_STRelation, key)
		if err != nil {
			return nil, err
		}
		uks = append(uks, uk.String())
	}
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(uks),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		return
	}

	for _, rec := range records {
		relations = append(relations, relationutils.RelationFromStruct(rec.Details))
	}
	return
}

func (s *service) ListAll(spaceId string, opts ...FetchOption) (relations relationutils.Relations, err error) {
	return s.listAll(spaceId, opts...)
}

func (s *service) listAll(spaceId string, opts ...FetchOption) (relations relationutils.Relations, err error) {
	filters := []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyLayout.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.Float64(float64(model.ObjectType_relation)),
		},
	}
	o := &fetchOptions{}
	for _, apply := range opts {
		apply(o)
	}
	filters = append(filters, &model.BlockContentDataviewFilter{
		RelationKey: bundle.RelationKeyWorkspaceId.String(),
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       pbtypes.String(spaceId),
	})

	relations2, _, err := s.objectStore.Query(database.Query{
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
}

type FetchOption func(options *fetchOptions)

func (s *service) FetchKey(spaceID string, key string, opts ...FetchOption) (relation *relationutils.Relation, err error) {
	return s.fetchKey(spaceID, key, opts...)
}

func (s *service) fetchKey(spaceID string, key string, opts ...FetchOption) (relation *relationutils.Relation, err error) {
	o := &fetchOptions{}
	for _, apply := range opts {
		apply(o)
	}
	uk, err := uniquekey.NewUniqueKey(model.SmartBlockType_STRelation, key)
	if err != nil {
		return nil, err
	}
	q := database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Value:       pbtypes.String(uk.String()),
			},
		},
	}
	q.Filters = append(q.Filters, &model.BlockContentDataviewFilter{
		Condition:   model.BlockContentDataviewFilter_Equal,
		RelationKey: bundle.RelationKeySpaceId.String(),
		Value:       pbtypes.String(spaceID),
	})

	records, _, err := s.objectStore.Query(q)
	if err != nil {
		return
	}
	for _, rec := range records {
		return relationutils.RelationFromStruct(rec.Details), nil
	}
	return nil, ErrNotFound
}

func (s *service) ValidateFormat(spaceID string, key string, v *types.Value) error {
	r, err := s.FetchKey(spaceID, key)
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
