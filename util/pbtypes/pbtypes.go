package pbtypes

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func Int64(v int64) *types.Value {
	return &types.Value{
		Kind: &types.Value_NumberValue{NumberValue: float64(v)},
	}
}

func Float64(v float64) *types.Value {
	return &types.Value{
		Kind: &types.Value_NumberValue{NumberValue: v},
	}
}

func Null() *types.Value {
	return &types.Value{
		Kind: &types.Value_NullValue{NullValue: types.NullValue_NULL_VALUE},
	}
}

func String(v string) *types.Value {
	return &types.Value{
		Kind: &types.Value_StringValue{StringValue: v},
	}
}

func Struct(v *types.Struct) *types.Value {
	return &types.Value{
		Kind: &types.Value_StructValue{StructValue: v},
	}
}

func StringList(s []string) *types.Value {
	var vals = make([]*types.Value, 0, len(s))
	for _, str := range s {
		vals = append(vals, String(str))
	}

	return &types.Value{
		Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}},
	}
}

func IntList(ints ...int) *types.Value {
	var vals = make([]*types.Value, 0, len(ints))
	for _, i := range ints {
		vals = append(vals, Int64(int64(i)))
	}

	return &types.Value{
		Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}},
	}
}

func NilToNullWrapper(v *types.Value) *types.Value {
	if v == nil || v.Kind == nil {
		return Null()
	}
	return v
}

func Bool(v bool) *types.Value {
	return &types.Value{
		Kind: &types.Value_BoolValue{BoolValue: v},
	}
}

func GetFloat64(s *types.Struct, name string) float64 {
	if s == nil || s.Fields == nil {
		return 0
	}
	if v, ok := s.Fields[name]; ok {
		return v.GetNumberValue()
	}
	return 0
}

func GetInt64(s *types.Struct, name string) int64 {
	if s == nil || s.Fields == nil {
		return 0
	}
	if v, ok := s.Fields[name]; ok {
		return int64(v.GetNumberValue())
	}
	return 0
}

func GetString(s *types.Struct, name string) string {
	if s == nil || s.Fields == nil {
		return ""
	}
	if v, ok := s.Fields[name]; ok {
		return v.GetStringValue()
	}
	return ""
}

func GetStruct(s *types.Struct, name string) *types.Struct {
	if s == nil || s.Fields == nil {
		return nil
	}
	if v, ok := s.Fields[name]; ok {
		return v.GetStructValue()
	}
	return nil
}

func GetBool(s *types.Struct, name string) bool {
	if s == nil || s.Fields == nil {
		return false
	}
	if v, ok := s.Fields[name]; ok {
		return v.GetBoolValue()
	}
	return false
}

func IsExpectedBoolValue(val *types.Value, expectedValue bool) bool {
	if val == nil {
		return false
	}
	if v, ok := val.Kind.(*types.Value_BoolValue); ok && v.BoolValue == expectedValue {
		return true
	}
	return false
}

func Exists(s *types.Struct, name string) bool {
	if s == nil || s.Fields == nil {
		return false
	}
	_, ok := s.Fields[name]
	return ok
}

func GetStringList(s *types.Struct, name string) []string {
	if s == nil || s.Fields == nil {
		return nil
	}

	if v, ok := s.Fields[name]; !ok {
		return nil
	} else {
		return GetStringListValue(v)

	}
}

// UpdateStringList updates a string list field using modifier function and returns updated value
func UpdateStringList(s *types.Struct, name string, modifier func([]string) []string) []string {
	if s == nil {
		return nil
	}
	list := GetStringList(s, name)
	list = modifier(list)
	if s.Fields == nil {
		s.Fields = map[string]*types.Value{}
	}
	s.Fields[name] = StringList(list)
	return list
}

func GetIntList(s *types.Struct, name string) []int {
	if s == nil || s.Fields == nil {
		return nil
	}

	if v, ok := s.Fields[name]; !ok {
		return nil
	} else {
		return GetIntListValue(v)

	}
}

func GetIntListValue(v *types.Value) []int {
	if v == nil {
		return nil
	}
	var res []int
	if list, ok := v.Kind.(*types.Value_ListValue); ok {
		if list.ListValue == nil {
			return nil
		}
		for _, v := range list.ListValue.Values {
			if _, ok = v.GetKind().(*types.Value_NumberValue); ok {
				res = append(res, int(v.GetNumberValue()))
			}
		}
	} else if val, ok := v.Kind.(*types.Value_NumberValue); ok {
		return []int{int(val.NumberValue)}
	}

	return res
}

// GetStringListValue returns string slice from StringValue and List of StringValue
func GetStringListValue(v *types.Value) []string {
	if v == nil {
		return nil
	}
	if list, ok := v.Kind.(*types.Value_ListValue); ok {
		return ListValueToStrings(list.ListValue)
	} else if val, ok := v.Kind.(*types.Value_StringValue); ok && val.StringValue != "" {
		return []string{val.StringValue}
	}
	return nil
}

func ListValueToStrings(list *types.ListValue) []string {
	if list == nil {
		return nil
	}
	stringsSlice := make([]string, 0, len(list.Values))
	for _, v := range list.Values {
		if _, ok := v.GetKind().(*types.Value_StringValue); ok {
			stringsSlice = append(stringsSlice, v.GetStringValue())
		}
	}
	return stringsSlice
}

func HasField(st *types.Struct, key string) bool {
	if st == nil || st.Fields == nil {
		return false
	}

	_, exists := st.Fields[key]

	return exists
}

func HasRelation(rels []*model.Relation, key string) bool {
	for _, rel := range rels {
		if rel.Key == key {
			return true
		}
	}

	return false
}

func HasRelationLink(rels []*model.RelationLink, key string) bool {
	for _, rel := range rels {
		if rel.Key == key {
			return true
		}
	}

	return false
}

func GetRelation(rels []*model.Relation, key string) *model.Relation {
	for i, rel := range rels {
		if rel.Key == key {
			return rels[i]
		}
	}

	return nil
}

func Get(st *types.Struct, keys ...string) *types.Value {
	for i, key := range keys {
		if st == nil || st.Fields == nil {
			return nil
		}
		if i == len(keys)-1 {
			return st.Fields[key]
		} else {
			st = GetStruct(st, key)
		}
	}
	return nil
}

func GetRelationListKeys(rels []*model.RelationLink) []string {
	var keys []string
	for _, rel := range rels {
		keys = append(keys, rel.Key)
	}

	return keys
}

// StructToMap converts a types.Struct to a map from strings to Go types.
// StructToMap panics if s is invalid.
func StructToMap(s *types.Struct) map[string]interface{} {
	if s == nil {
		return nil
	}
	m := map[string]interface{}{}
	for k, v := range s.Fields {
		m[k] = ValueToInterface(v)
	}
	return m
}

func ValueToInterface(v *types.Value) interface{} {
	switch k := v.Kind.(type) {
	case *types.Value_NullValue:
		return nil
	case *types.Value_NumberValue:
		return k.NumberValue
	case *types.Value_StringValue:
		return k.StringValue
	case *types.Value_BoolValue:
		return k.BoolValue
	case *types.Value_StructValue:
		return StructToMap(k.StructValue)
	case *types.Value_ListValue:
		s := make([]interface{}, len(k.ListValue.Values))
		for i, e := range k.ListValue.Values {
			s[i] = ValueToInterface(e)
		}
		return s
	default:
		panic("protostruct: unknown kind")
	}
}

// deprecated
func BundledRelationIdToKey(id string) (string, error) {
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return strings.TrimPrefix(id, addr.BundledRelationURLPrefix), nil
	}

	return "", fmt.Errorf("incorrect id format")
}

type Getter interface {
	Get(key string) *types.Value
}

type structGetter struct {
	st *types.Struct
}

func ValueGetter(s *types.Struct) Getter {
	return &structGetter{s}
}

func (sg *structGetter) Get(key string) *types.Value {
	if sg == nil {
		return nil
	}
	if sg.st.Fields == nil {
		return nil
	}
	return sg.st.Fields[key]
}

func Map(s *types.Struct, keys ...string) *types.Struct {
	if len(keys) == 0 {
		return s
	}
	if s == nil {
		return nil
	}
	ns := new(types.Struct)
	if s.Fields == nil {
		return ns
	}
	ns.Fields = make(map[string]*types.Value)
	for _, key := range keys {
		if value, ok := s.Fields[key]; ok {
			ns.Fields[key] = value
		}
	}
	return ns
}

func StructIterate(st *types.Struct, f func(path []string, v *types.Value)) {
	var iterate func(s *types.Struct, f func(path []string, v *types.Value), path []string)
	iterate = func(s *types.Struct, f func(path []string, v *types.Value), path []string) {
		if s == nil || s.Fields == nil {
			return
		}
		for k, v := range s.Fields {
			p := append(path, k)
			f(p, v)
			iterate(GetStruct(s, k), f, p)
		}
	}
	iterate(st, f, nil)
}

func StructEqualKeys(st1, st2 *types.Struct) bool {
	if (st1 == nil) != (st2 == nil) {
		return false
	}
	if (st1.Fields == nil) != (st2.Fields == nil) {
		return false
	}
	if len(st1.Fields) != len(st2.Fields) {
		return false
	}
	for k := range st1.Fields {
		if _, ok := st2.Fields[k]; !ok {
			return false
		}
	}
	return true
}

func Sprint(p proto.Message) string {
	m := jsonpb.Marshaler{Indent: " "}
	result, _ := m.MarshalToString(p)
	return result
}

func StructCompareIgnoreKeys(st1 *types.Struct, st2 *types.Struct, ignoreKeys []string) bool {
	if (st1 == nil) != (st2 == nil) {
		return false
	}
	if (st1.Fields == nil) != (st2.Fields == nil) {
		return false
	}
	if len(st1.Fields) != len(st2.Fields) {
		return false
	}
	for k, v := range st1.Fields {
		if slice.FindPos(ignoreKeys, k) > -1 {
			continue
		}
		if v2, ok := st2.Fields[k]; !ok || !v.Equal(v2) {
			return false
		}
	}
	return true
}

// ValueListWrapper wraps single value into the list. If value is already a list, it is returned as is.
// Null and struct values are not supported
func ValueListWrapper(value *types.Value) (*types.ListValue, error) {
	switch v := value.Kind.(type) {
	case *types.Value_ListValue:
		return v.ListValue, nil
	case *types.Value_StringValue, *types.Value_NumberValue, *types.Value_BoolValue:
		return &types.ListValue{Values: []*types.Value{value}}, nil
	}
	return nil, fmt.Errorf("not supported type")
}

// NormalizeStruct replace some cases with nil values with NullValue instead
// - nil values in the Struct.Fields (supported by protobuf, not by all our clients)
// - calls NormalizeValue for each field
// the Struct argument is modified in place
func NormalizeStruct(t *types.Struct) (wasNormalized bool) {
	if t == nil {
		return false
	}

	for k, v := range t.Fields {
		if v == nil {
			t.Fields[k] = &types.Value{Kind: &types.Value_NullValue{NullValue: types.NullValue_NULL_VALUE}}
			wasNormalized = true
			continue
		}
		if NormalizeValue(v) {
			wasNormalized = true
		}
	}
	return
}

// NormalizeValue replace some cases with nil values with NullValue instead
// - nil values in the Value.Kind (not supported by protobuf, because it is required field)
// - nil values in the ListValue.Values (supported by protobuf, not by all our clients)
// the Struct argument is modified in place
func NormalizeValue(t *types.Value) (wasNormalized bool) {
	if t == nil {
		return false
	}
	switch v := t.Kind.(type) {
	case *types.Value_StructValue:
		return NormalizeStruct(v.StructValue)
	case *types.Value_ListValue:
		for i, lv := range v.ListValue.Values {
			if lv == nil {
				v.ListValue.Values[i] = &types.Value{Kind: &types.Value_NullValue{NullValue: types.NullValue_NULL_VALUE}}
				wasNormalized = true
				continue
			}
			if NormalizeValue(lv) {
				wasNormalized = true
			}
		}
	case nil:
		// nil value is not valid in most of the others pb implementations. Replace it with NullValue
		t.Kind = &types.Value_NullValue{NullValue: types.NullValue_NULL_VALUE}
	}

	return
}

func ValidateStruct(t *types.Struct) error {
	if t == nil {
		return nil
	}
	for _, v := range t.Fields {
		if v == nil {
			return fmt.Errorf("map value is nil")
		}
		if err := ValidateValue(v); err != nil {
			return err
		}
	}
	return nil
}

func ValidateValue(t *types.Value) error {
	if t == nil {
		return nil
	}
	switch v := t.Kind.(type) {
	case *types.Value_StructValue:
		return ValidateStruct(v.StructValue)
	case *types.Value_ListValue:
		for _, v := range v.ListValue.Values {
			if v == nil {
				return fmt.Errorf("list value is nil")
			}
			if err := ValidateValue(v); err != nil {
				return err
			}
		}
	case nil:
		return fmt.Errorf("value Kind is nil")
	default:
		return nil
	}
	return nil
}

func IsStructEmpty(s *types.Struct) bool {
	if s == nil {
		return true
	} else if s.GetFields() == nil {
		return true
	} else if len(s.GetFields()) == 0 {
		return true
	}
	return false
}

// deprecated
func RelationIdToKey(id string) (string, error) {
	if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
		return strings.TrimPrefix(id, addr.RelationKeyToIdPrefix), nil
	}
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return strings.TrimPrefix(id, addr.BundledRelationURLPrefix), nil
	}

	return "", fmt.Errorf("incorrect id format")
}
