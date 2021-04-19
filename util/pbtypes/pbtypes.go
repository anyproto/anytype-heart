package pbtypes

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/gogo/protobuf/types"
	"strings"
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

func StringList(s []string) *types.Value {
	var vals []*types.Value
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
			item := v.GetStringValue()
			if item != "" {
				stringsSlice = append(stringsSlice, item)
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

func HasRelation(rels []*pbrelation.Relation, key string) bool {
	for _, rel := range rels {
		if rel.Key == key {
			return true
		}
	}

	return false
}

func GetObjectType(ots []*pbrelation.ObjectType, url string) *pbrelation.ObjectType {
	for i, ot := range ots {
		if ot.Url == url {
			return ots[i]
		}
	}

	return nil
}

func GetRelation(rels []*pbrelation.Relation, key string) *pbrelation.Relation {
	for i, rel := range rels {
		if rel.Key == key {
			return rels[i]
		}
	}

	return nil
}

func GetOption(opts []*pbrelation.RelationOption, id string) *pbrelation.RelationOption {
	for i, opt := range opts {
		if opt.Id == id {
			return opts[i]
		}
	}

	return nil
}

func HasOption(opts []*pbrelation.RelationOption, id string) bool {
	for _, opt := range opts {
		if opt.Id == id {
			return true
		}
	}

	return false
}

func Get(st *types.Struct, key string) *types.Value {
	if st == nil || st.Fields == nil {
		return nil
	}
	return st.Fields[key]
}

func GetRelationKeys(rels []*pbrelation.Relation) []string {
	var keys []string
	for _, rel := range rels {
		keys = append(keys, rel.Key)
	}

	return keys
}

func GetOptionIds(opts []*pbrelation.RelationOption) []string {
	var keys []string
	for _, opt := range opts {
		keys = append(keys, opt.Id)
	}

	return keys
}

func MergeRelationsDicts(rels1 []*pbrelation.Relation, rels2 []*pbrelation.Relation) []*pbrelation.Relation {
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
func MergeOptionsPreserveScope(opts1 []*pbrelation.RelationOption, opts2 []*pbrelation.RelationOption) []*pbrelation.RelationOption {
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

func RelationFormatCanHaveListValue(format pbrelation.RelationFormat) bool {
	switch format {
	case pbrelation.RelationFormat_tag,
		pbrelation.RelationFormat_file,
		pbrelation.RelationFormat_object:
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
