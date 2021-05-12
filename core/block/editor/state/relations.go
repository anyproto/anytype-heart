package state

import (
	"fmt"
	"net/url"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func validateRelationFormat(rel *model.Relation, v *types.Value) error {
	if _, isNull := v.Kind.(*types.Value_NullValue); isNull {
		// allow null value for any field
		return nil
	}

	switch rel.Format {
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
	case model.RelationFormat_status, model.RelationFormat_tag:
		if _, ok := v.Kind.(*types.Value_ListValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of list", v.Kind)
		}

		if rel.MaxCount > 0 && len(v.GetListValue().Values) > int(rel.MaxCount) {
			return fmt.Errorf("maxCount exceeded")
		}

		return validateOptions(rel.SelectDict, v.GetListValue().Values)
	case model.RelationFormat_date:
		if _, ok := v.Kind.(*types.Value_NumberValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of number", v.Kind)
		}

		return nil
	case model.RelationFormat_file, model.RelationFormat_object:
		switch s := v.Kind.(type) {
		case *types.Value_StringValue:
			if rel.MaxCount != 1 {
				return fmt.Errorf("incorrect type: %T instead of list(maxCount!=1)", v.Kind)
			}
			return nil
		case *types.Value_ListValue:
			if rel.MaxCount > 0 && len(s.ListValue.Values) > int(rel.MaxCount) {
				return fmt.Errorf("relation %s(%s) has maxCount exceeded", rel.Key, rel.Format.String())
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
		return fmt.Errorf("unsupported rel format: %s", rel.Format.String())
	}
}

func validateOptions(opts []*model.RelationOption, vals []*types.Value) error {
	for i, lv := range vals {
		if optId, ok := lv.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect list item value at index %d: %T", i, lv.Kind)
		} else if optId.StringValue == "" {
			return fmt.Errorf("empty option at index %d", i)
		} else if opt := pbtypes.GetOption(opts, optId.StringValue); opt == nil {
			return fmt.Errorf("option with id %s not found", optId.StringValue)
		}
	}

	return nil
}
