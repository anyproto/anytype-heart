package pbtypes

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
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

// GetStringListValue returns string slice from StringValue and List of StringValue
func GetStringListValue(v *types.Value) []string {
	if v == nil {
		return nil
	}
	var stringsSlice []string
	if list, ok := v.Kind.(*types.Value_ListValue); ok {
		if list.ListValue == nil {
			return nil
		}
		for _, v := range list.ListValue.Values {
			if _, ok = v.GetKind().(*types.Value_StringValue); ok {
				stringsSlice = append(stringsSlice, v.GetStringValue())
			}
		}
	} else if val, ok := v.Kind.(*types.Value_StringValue); ok {
		return []string{val.StringValue}
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

func MergeRelations(rels1 []*model.Relation, rels2 []*model.Relation) []*model.Relation {
	if rels1 == nil {
		return rels2
	}
	if rels2 == nil {
		return rels1
	}

	rels := make([]*model.Relation, 0, len(rels2)+len(rels1))
	for _, rel := range rels2 {
		rels = append(rels, rel)
	}

	for _, rel := range rels1 {
		if HasRelation(rels, rel.Key) {
			continue
		}
		rels = append(rels, rel)
	}

	return rels
}

func GetObjectType(ots []*model.ObjectType, url string) *model.ObjectType {
	for i, ot := range ots {
		if ot.Url == url {
			return ots[i]
		}
	}

	return nil
}

func GetRelation(rels []*model.Relation, key string) *model.Relation {
	for i, rel := range rels {
		if rel.Key == key {
			return rels[i]
		}
	}

	return nil
}

func GetOption(opts []*model.RelationOption, id string) *model.RelationOption {
	for i, opt := range opts {
		if opt.Id == id {
			return opts[i]
		}
	}

	return nil
}

func HasOption(opts []*model.RelationOption, id string) bool {
	for _, opt := range opts {
		if opt.Id == id {
			return true
		}
	}

	return false
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

func GetRelationKeys(rels []*model.Relation) []string {
	var keys []string
	for _, rel := range rels {
		keys = append(keys, rel.Key)
	}

	return keys
}

func GetOptionIds(opts []*model.RelationOption) []string {
	var keys []string
	for _, opt := range opts {
		keys = append(keys, opt.Id)
	}

	return keys
}

func MergeRelationsDicts(rels1 []*model.Relation, rels2 []*model.Relation) []*model.Relation {
	rels := CopyRelations(rels1)
	for _, rel2 := range rels2 {
		var found bool

		for i, rel := range rels {
			if rel.Key == rel2.Key {
				rel2Copy := CopyRelation(rel2)
				rels[i].SelectDict = rel2Copy.SelectDict
				rels[i].Name = rel2Copy.Name
				found = true
				break
			}
		}

		if !found {
			rels = append(rels, CopyRelation(rel2))
		}
	}
	return rels
}

// MergeOptionsPreserveScope adds and updates options from opts2 into opts1 based on the ID
// in case opts2 doesn't have id that opts1 have it doesn't remove the existing one
// in case opts2 has the key that opts1 already have it updates everything except scope
func MergeOptionsPreserveScope(opts1 []*model.RelationOption, opts2 []*model.RelationOption) []*model.RelationOption {
	opts := CopyOptions(opts1)
	for _, opt2 := range opts2 {
		var found bool
		for i, opt := range opts {
			if opt.Id == opt2.Id {
				opts[i].Text = opt2.Text
				opts[i].Color = opt2.Color
				found = true
				break
			}
		}
		if !found {
			opt2Copy := *opt2
			opts = append(opts, &opt2Copy)
		}
	}
	return opts
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

func StructIsEmpty(s *types.Struct) bool {
	return s == nil || len(s.Fields) == 0
}

func GetMapOfKeysAndValuesFromStruct(collection *types.Struct) map[string]*types.Value {
	keyMap := map[string]*types.Value{}
	if collection == nil {
		return keyMap
	}
	keyStack := []string{""}
	collStack := []*types.Struct{collection}

	for len(collStack) != 0 {
		coll := collStack[len(collStack)-1]
		lastKey := keyStack[len(keyStack)-1]
		keyStack = keyStack[:len(keyStack)-1]
		collStack = collStack[:len(collStack)-1]
		for k, v := range coll.Fields {
			subColl, ok := v.Kind.(*types.Value_StructValue)
			updatedKey := lastKey
			if updatedKey != "" {
				updatedKey += "/"
			}
			updatedKey += k
			if !ok {
				keyMap[updatedKey] = v
				continue
			}
			collStack = append(collStack, subColl.StructValue)
			keyStack = append(keyStack, updatedKey)
		}
	}
	return keyMap
}

func CompareKeyMaps(before map[string]*types.Value, after map[string]*types.Value) (keysSet []string, keysRemoved []string) {
	for k, afterValue := range after {
		beforeValue, exists := before[k]
		if exists && afterValue.Equal(beforeValue) {
			continue
		}
		keysSet = append(keysSet, k)
	}

	for k := range before {
		if _, exists := after[k]; exists {
			continue
		}
		keysRemoved = append(keysRemoved, k)
	}
	return
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

func RelationFormatCanHaveListValue(format model.RelationFormat) bool {
	switch format {
	case model.RelationFormat_tag,
		model.RelationFormat_file,
		model.RelationFormat_object:
		return true
	default:
		return false
	}
}

func RelationIdToKey(id string) (string, error) {
	if strings.HasPrefix(id, addr.CustomRelationURLPrefix) {
		return strings.TrimPrefix(id, addr.CustomRelationURLPrefix), nil
	}

	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return strings.TrimPrefix(id, addr.BundledRelationURLPrefix), nil
	}
	return "", fmt.Errorf("incorrect id format")
}

func Delete(st *types.Struct, key string) (ok bool) {
	if st != nil && st.Fields != nil {
		if _, ok := st.Fields[key]; ok {
			delete(st.Fields, key)
			return true
		}
	}
	return false
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

func Sprint(p proto.Message) string {
	m := jsonpb.Marshaler{Indent: " "}
	result, _ := m.MarshalToString(p)
	return string(result)
}
