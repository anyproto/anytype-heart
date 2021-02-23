package state

import (
	"fmt"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
	"github.com/gogo/protobuf/types"
	"net/url"
	"strings"
)

func validateRelationFormat(rel *pbrelation.Relation, v *types.Value) error {
	switch rel.Format {
	case pbrelation.RelationFormat_longtext, pbrelation.RelationFormat_shorttext:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of string", v.Kind)
		}
		return nil
	case pbrelation.RelationFormat_number:
		if _, ok := v.Kind.(*types.Value_NumberValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of number", v.Kind)
		}
		return nil
	case pbrelation.RelationFormat_status, pbrelation.RelationFormat_tag:
		if _, ok := v.Kind.(*types.Value_ListValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of list", v.Kind)
		}

		if rel.MaxCount > 0 && len(v.GetListValue().Values) > int(rel.MaxCount) {
			return fmt.Errorf("maxCount exceeded")
		}

		return validateOptions(rel.SelectDict, v.GetListValue().Values)
	case pbrelation.RelationFormat_date:
		if _, ok := v.Kind.(*types.Value_NumberValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of number", v.Kind)
		}

		return nil
	case pbrelation.RelationFormat_file, pbrelation.RelationFormat_object:
		switch s := v.Kind.(type) {
		case *types.Value_StringValue:
			if rel.MaxCount != 1 {
				return fmt.Errorf("incorrect type: %t instead of list(maxCount!=1)", v.Kind)
			}
			return nil
		case *types.Value_ListValue:
			if rel.MaxCount > 0 && len(s.ListValue.Values) > int(rel.MaxCount) {
				return fmt.Errorf("relation %s(%s) has maxCount exceeded", rel.Key, rel.Format.String())
			}

			for i, lv := range s.ListValue.Values {
				if optId, ok := lv.Kind.(*types.Value_StringValue); !ok {
					return fmt.Errorf("incorrect list item value at index %d: %t instead of string", i, lv.Kind)
				} else if optId.StringValue == "" {
					return fmt.Errorf("empty option at index %d", i)
				}
			}
			return nil
		default:
			return fmt.Errorf("incorrect type: %t instead of list/string", v.Kind)
		}
	case pbrelation.RelationFormat_checkbox:
		if _, ok := v.Kind.(*types.Value_BoolValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of bool", v.Kind)
		}

		return nil
	case pbrelation.RelationFormat_url:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of string", v.Kind)
		}

		u, err := url.Parse(v.GetStringValue())
		if err != nil {
			return fmt.Errorf("failed to parse URL: %s", err.Error())
		}
		if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
			return fmt.Errorf("url scheme %s not supported", u.Scheme)
		}
		return nil
	case pbrelation.RelationFormat_email:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of string", v.Kind)
		}

		valid := uri.ValidateEmail(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate email")
		}
		return nil
	case pbrelation.RelationFormat_phone:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of string", v.Kind)
		}

		valid := uri.ValidatePhone(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate phone")
		}
		return nil
	case pbrelation.RelationFormat_emoji:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %t instead of string", v.Kind)
		}

		// check if the symbol is emoji
		return nil
	default:
		return fmt.Errorf("unsupported rel format: %s", rel.Format.String())
	}
}

func validateOptions(opts []*pbrelation.RelationOption, vals []*types.Value) error {
	for i, lv := range vals {
		if optId, ok := lv.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect list item value at index %d: %t", i, lv.Kind)
		} else if optId.StringValue == "" {
			return fmt.Errorf("empty option at index %d", i)
		} else if opt := pbtypes.GetOption(opts, optId.StringValue); opt == nil {
			return fmt.Errorf("option with id %s not found", i)
		}
	}

	return nil
}
