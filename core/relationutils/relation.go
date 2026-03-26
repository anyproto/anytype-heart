package relationutils

import (
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/uri"
)

type RelationFormatFetcher interface {
	GetRelationFormatByKey(spaceId string, key domain.RelationKey) (model.RelationFormat, error)
	app.ComponentRunnable
}

func RelationFromDetails(det *domain.Details) *Relation {
	key := det.GetString(bundle.RelationKeyRelationKey)
	maxCount := int32(det.GetInt64(bundle.RelationKeyRelationMaxCount))
	return &Relation{
		Relation: &model.Relation{
			Id:               det.GetString(bundle.RelationKeyId),
			Key:              key,
			Format:           model.RelationFormat(det.GetFloat64(bundle.RelationKeyRelationFormat)),
			Name:             det.GetString(bundle.RelationKeyName),
			DataSource:       model.Relation_details,
			Hidden:           det.GetBool(bundle.RelationKeyIsHidden),
			ReadOnly:         det.GetBool(bundle.RelationKeyRelationReadonlyValue),
			ReadOnlyRelation: false,
			Multi:            maxCount > 1,
			ObjectTypes:      det.GetStringList(bundle.RelationKeyRelationFormatObjectTypes),
			MaxCount:         maxCount,
			Description:      det.GetString(bundle.RelationKeyDescription),
			Creator:          det.GetString(bundle.RelationKeyCreator),
			Revision:         det.GetInt64(bundle.RelationKeyRevision),
			IncludeTime:      det.GetBool(bundle.RelationKeyRelationFormatIncludeTime),
		},
	}

}

type Relation struct {
	*model.Relation
}

func (r *Relation) RelationLink() *model.RelationLink {
	return &model.RelationLink{
		Format: r.Format,
		Key:    r.Key,
	}
}

func (r *Relation) ToDetails() *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyCreator:                   domain.String(r.GetCreator()),
		bundle.RelationKeyDescription:               domain.String(r.GetDescription()),
		bundle.RelationKeyId:                        domain.String(r.Id),
		bundle.RelationKeyIsHidden:                  domain.Bool(r.GetHidden()),
		bundle.RelationKeyIsReadonly:                domain.Bool(r.GetReadOnlyRelation()),
		bundle.RelationKeyResolvedLayout:            domain.Int64(int64(model.ObjectType_relation)),
		bundle.RelationKeyLayout:                    domain.Int64(int64(model.ObjectType_relation)),
		bundle.RelationKeyName:                      domain.String(r.GetName()),
		bundle.RelationKeyRelationDefaultValue:      domain.ValueFromProto(r.GetDefaultValue()),
		bundle.RelationKeyRelationFormat:            domain.Float64(float64(r.GetFormat())),
		bundle.RelationKeyRelationFormatObjectTypes: domain.StringList(r.GetObjectTypes()),
		bundle.RelationKeyRelationKey:               domain.String(r.GetKey()),
		bundle.RelationKeyRelationMaxCount:          domain.Float64(float64(r.GetMaxCount())),
		bundle.RelationKeyRelationReadonlyValue:     domain.Bool(r.GetReadOnly()),
		bundle.RelationKeyType:                      domain.String(bundle.TypeKeyRelation.BundledURL()),
		// TODO Is it ok?
		bundle.RelationKeyUniqueKey:                 domain.String(domain.RelationKey(r.GetKey()).URL()),
		bundle.RelationKeyRevision:                  domain.Int64(r.GetRevision()),
		bundle.RelationKeyRelationFormatIncludeTime: domain.Bool(r.GetIncludeTime()),
	})
}

type Relations []*Relation

func (rs Relations) Models() []*model.Relation {
	res := make([]*model.Relation, 0, len(rs))
	for _, r := range rs {
		res = append(res, r.Relation)
	}
	return res
}

func (rs Relations) RelationLinks() []*model.RelationLink {
	res := make([]*model.RelationLink, 0, len(rs))
	for _, r := range rs {
		res = append(res, r.RelationLink())
	}
	return res
}

func (rs Relations) GetByKey(key string) *Relation {
	for _, r := range rs {
		if r.Key == key {
			return r
		}
	}
	return nil
}

func (rs Relations) GetModelByKey(key string) *model.Relation {
	if r := rs.GetByKey(key); r != nil {
		return r.Relation
	}
	return nil
}

func MigrateRelationModels(rels []*model.Relation) (relLinks []*model.RelationLink) {
	relLinks = make([]*model.RelationLink, 0, len(rels))
	for _, rel := range rels {
		relLinks = append(relLinks, &model.RelationLink{
			Key:    rel.Key,
			Format: rel.Format,
		})
	}
	return
}

func ValidateRelationFormat(key domain.RelationKey, v domain.Value, format model.RelationFormat, maxCount int32) error {
	if !v.Ok() {
		return fmt.Errorf("invalid value")
	}
	if v.IsNull() {
		// allow null value for any field
		return nil
	}

	if maxCount == 0 {
		// let's try bundle relation
		relation, _ := bundle.PickRelation(key) // nolint:errcheck
		maxCount = relation.MaxCount
	}

	switch format {
	case model.RelationFormat_longtext, model.RelationFormat_shorttext:
		if !v.IsString() {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}
		return nil
	case model.RelationFormat_number:
		if !v.IsFloat64() {
			return fmt.Errorf("incorrect type: %v instead of number", v)
		}
		return nil
	case model.RelationFormat_status:
		vals, ok := v.TryWrapToStringList()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string list", v)
		}
		if len(vals) > 1 {
			return fmt.Errorf("status should not contain more than one value")
		}
		return validateOptions(vals)

	case model.RelationFormat_tag:
		vals, ok := v.TryWrapToStringList()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string list", v)
		}
		if maxCount > 0 && len(vals) > int(maxCount) {
			return fmt.Errorf("maxCount exceeded")
		}

		return validateOptions(vals)
	case model.RelationFormat_date:
		if !v.IsFloat64() {
			return fmt.Errorf("incorrect type: %v instead of number", v)
		}

		return nil
	case model.RelationFormat_file, model.RelationFormat_object:
		vals, ok := v.TryWrapToStringList()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string list", v)
		}
		if maxCount > 0 && len(vals) > int(maxCount) {
			return fmt.Errorf("relation %s(%s) has maxCount exceeded", key, format.String())
		}

		for i, lv := range vals {
			if lv == "" {
				return fmt.Errorf("empty option at index %d", i)
			}
		}
		return nil

	case model.RelationFormat_checkbox:
		if !v.IsBool() {
			return fmt.Errorf("incorrect type: %v instead of bool", v)
		}

		return nil
	case model.RelationFormat_url:
		val, ok := v.TryString()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}
		s := strings.TrimSpace(val)
		if s != "" {
			err := uri.ValidateURI(s)
			if err != nil {
				return fmt.Errorf("failed to parse URL: %w", err)
			}
		}
		// todo: should we allow schemas other than http/https?
		// if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		//	return fmt.Errorf("url scheme %s not supported", u.Scheme)
		// }
		return nil
	case model.RelationFormat_email:
		_, ok := v.TryString()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}
		// todo: revise regexp and reimplement
		/*valid := uri.ValidateEmail(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate email")
		}*/
		return nil
	case model.RelationFormat_phone:
		_, ok := v.TryString()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}

		// todo: revise regexp and reimplement
		/*valid := uri.ValidatePhone(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate phone")
		}*/
		return nil
	case model.RelationFormat_emoji:
		_, ok := v.TryString()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}

		// check if the symbol is emoji
		return nil
	case model.RelationFormat_map:
		_, ok := v.TryMapValue()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of map", v)
		}
		return nil
	default:
		return fmt.Errorf("unsupported rel format: %s", format.String())
	}
}

func validateOptions(v []string) error {
	// TODO:
	return nil
}
