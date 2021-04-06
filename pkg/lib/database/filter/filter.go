package filter

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

var (
	ErrValueMustBeList = errors.New("value must be list")
)

func MakeAndFilter(protoFilters []*model.BlockContentDataviewFilter) (Filter, error) {
	var and AndFilters
	for _, pf := range protoFilters {
		f, err := MakeFilter(pf)
		if err != nil {
			return nil, err
		}
		and = append(and, f)
	}
	return and, nil
}

func MakeFilter(proto *model.BlockContentDataviewFilter) (Filter, error) {
	// replaces "value == false" to "value != true" for expected work with checkboxes
	if proto.Condition == model.BlockContentDataviewFilter_Equal && proto.Value != nil && proto.Value.Equal(pbtypes.Bool(false)) {
		proto = &model.BlockContentDataviewFilter{
			RelationKey:      proto.RelationKey,
			RelationProperty: proto.RelationProperty,
			Condition:        model.BlockContentDataviewFilter_NotEqual,
			Value:            pbtypes.Bool(true),
		}
	}
	switch proto.Condition {
	case model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_Greater,
		model.BlockContentDataviewFilter_Less,
		model.BlockContentDataviewFilter_GreaterOrEqual,
		model.BlockContentDataviewFilter_LessOrEqual:
		return Eq{
			Key:   proto.RelationKey,
			Cond:  proto.Condition,
			Value: proto.Value,
		}, nil
	case model.BlockContentDataviewFilter_NotEqual:
		return Not{Eq{
			Key:   proto.RelationKey,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: proto.Value,
		}}, nil
	case model.BlockContentDataviewFilter_Like:
		return Like{
			Key:   proto.RelationKey,
			Value: proto.Value,
		}, nil
	case model.BlockContentDataviewFilter_NotLike:
		return Not{Like{
			Key:   proto.RelationKey,
			Value: proto.Value,
		}}, nil
	case model.BlockContentDataviewFilter_In:
		list := proto.Value.GetListValue()
		if list == nil {
			return nil, ErrValueMustBeList
		}
		return In{
			Key:   proto.RelationKey,
			Value: list,
		}, nil
	case model.BlockContentDataviewFilter_NotIn:
		list := proto.Value.GetListValue()
		if list == nil {
			return nil, ErrValueMustBeList
		}
		return Not{In{
			Key:   proto.RelationKey,
			Value: list,
		}}, nil
	case model.BlockContentDataviewFilter_Empty:
		return Empty{
			Key: proto.RelationKey,
		}, nil
	case model.BlockContentDataviewFilter_NotEmpty:
		return Not{Empty{
			Key: proto.RelationKey,
		}}, nil
	case model.BlockContentDataviewFilter_AllIn:
		list := proto.Value.GetListValue()
		if list == nil {
			return nil, ErrValueMustBeList
		}
		return AllIn{
			Key:   proto.RelationKey,
			Value: list,
		}, nil
	case model.BlockContentDataviewFilter_NotAllIn:
		list := proto.Value.GetListValue()
		if list == nil {
			return nil, ErrValueMustBeList
		}
		return Not{AllIn{
			Key:   proto.RelationKey,
			Value: list,
		}}, nil
	default:
		return nil, fmt.Errorf("unexpected filter cond: %v", proto.Condition)
	}
}

type Getter interface {
	Get(key string) *types.Value
}

type Filter interface {
	FilterObject(g Getter) bool
	String() string
}

type AndFilters []Filter

func (a AndFilters) FilterObject(g Getter) bool {
	for _, f := range a {
		if !f.FilterObject(g) {
			return false
		}
	}
	return true
}

func (a AndFilters) String() string {
	var andS []string
	for _, f := range a {
		andS = append(andS, f.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(andS, " AND "))
}

type OrFilters []Filter

func (a OrFilters) FilterObject(g Getter) bool {
	if len(a) == 0 {
		return true
	}
	for _, f := range a {
		if f.FilterObject(g) {
			return true
		}
	}
	return false
}

func (a OrFilters) String() string {
	var orS []string
	for _, f := range a {
		orS = append(orS, f.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(orS, " OR "))
}

type Not struct {
	Filter
}

func (n Not) FilterObject(g Getter) bool {
	if n.Filter == nil {
		return false
	}
	return !n.Filter.FilterObject(g)
}

func (n Not) String() string {
	return fmt.Sprintf("NOT(%s)", n.Filter.String())
}

type Eq struct {
	Key   string
	Cond  model.BlockContentDataviewFilterCondition
	Value *types.Value
}

func (e Eq) FilterObject(g Getter) bool {
	return e.filterObject(g.Get(e.Key))
}

func (e Eq) filterObject(v *types.Value) bool {
	if list := v.GetListValue(); list != nil && e.Value.GetListValue() == nil {
		for _, lv := range list.Values {
			if e.filterObject(lv) {
				return true
			}
		}
		return false
	}
	comp := e.Value.Compare(v)
	switch e.Cond {
	case model.BlockContentDataviewFilter_Equal:
		return comp == 0
	case model.BlockContentDataviewFilter_Greater:
		return comp == -1
	case model.BlockContentDataviewFilter_GreaterOrEqual:
		return comp <= 0
	case model.BlockContentDataviewFilter_Less:
		return comp == 1
	case model.BlockContentDataviewFilter_LessOrEqual:
		return comp >= 0
	}
	return false
}

func (e Eq) String() string {
	var eq string
	switch e.Cond {
	case model.BlockContentDataviewFilter_Equal:
		eq = "="
	case model.BlockContentDataviewFilter_Greater:
		eq = ">"
	case model.BlockContentDataviewFilter_GreaterOrEqual:
		eq = ">="
	case model.BlockContentDataviewFilter_Less:
		eq = "<"
	case model.BlockContentDataviewFilter_LessOrEqual:
		eq = "<="
	}
	return fmt.Sprintf("%s %s '%s'", e.Key, eq, e.Value)
}

type In struct {
	Key   string
	Value *types.ListValue
}

func (i In) FilterObject(g Getter) bool {
	val := g.Get(i.Key)
	for _, v := range i.Value.Values {
		eq := Eq{Value: v, Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i In) String() string {
	return fmt.Sprintf("%v IN(%v)", i.Key, i.Value)
}

type Like struct {
	Key   string
	Value *types.Value
}

func (l Like) FilterObject(g Getter) bool {
	val := g.Get(l.Key)
	if val == nil {
		return false
	}
	valStr := val.GetStringValue()
	if valStr == "" {
		return false
	}
	return strings.Contains(strings.ToLower(valStr), strings.ToLower(l.Value.GetStringValue()))
}

func (l Like) String() string {
	return fmt.Sprintf("%v LIKE '%v'", l.Key, l.Value)
}

type Empty struct {
	Key string
}

func (e Empty) FilterObject(g Getter) bool {
	val := g.Get(e.Key)
	if val == nil {
		return true
	}
	if val.Kind == nil {
		return true
	}
	switch v := val.Kind.(type) {
	case *types.Value_NullValue:
		return true
	case *types.Value_StringValue:
		return v.StringValue == ""
	case *types.Value_ListValue:
		return v.ListValue == nil || len(v.ListValue.Values) == 0
	case *types.Value_StructValue:
		return v.StructValue == nil
	case *types.Value_NumberValue:
		return v.NumberValue == 0
	case *types.Value_BoolValue:
		return !v.BoolValue
	}
	return false
}

func (e Empty) String() string {
	return fmt.Sprintf("%v IS EMPTY", e.Key)
}

type AllIn struct {
	Key   string
	Value *types.ListValue
}

func (l AllIn) FilterObject(g Getter) bool {
	val := g.Get(l.Key)
	if val == nil {
		return false
	}
	list := val.GetListValue()
	if list == nil {
		return false
	}
	exist := func(v *types.Value) bool {
		for _, lv := range list.Values {
			if v.Equal(lv) {
				return true
			}
		}
		return false
	}
	for _, ev := range l.Value.Values {
		if !exist(ev) {
			return false
		}
	}
	return true
}

func (l AllIn) String() string {
	return fmt.Sprintf("%s ALLIN(%v)", l.Key, l.Value)
}
