package pbtypes

import (
	"sort"

	"github.com/planetscale/vtprotobuf/types/known/structpb"
	types "google.golang.org/protobuf/types/known/structpb"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func StructEqualIgnore(det1 *types.Struct, det2 *types.Struct, excludeKeys []string) (equal bool) {
	var m1, m2 map[string]*types.Value
	if det1 == nil || det1.Fields == nil {
		m1 = make(map[string]*types.Value)
	} else {
		m1 = det1.Fields
	}
	if det2 == nil || det2.Fields == nil {
		m2 = make(map[string]*types.Value)
	} else {
		m2 = det2.Fields
	}

	for key, v1 := range m1 {
		if slice.FindPos(excludeKeys, key) >= 0 {
			continue
		}
		if v2, exists := m2[key]; !exists {
			return false
		} else if (*structpb.Value)(v1).EqualVT((*structpb.Value)(v2)) {
			return false
		}
	}

	for key, _ := range m2 {
		if slice.FindPos(excludeKeys, key) >= 0 {
			continue
		}
		if _, exists := m1[key]; !exists {
			return false
		}
	}

	return true
}

// StructFilterKeys returns provided keys reusing underlying pb values pointers
func StructFilterKeys(st *types.Struct, filteredKeys []string) *types.Struct {
	return StructFilterKeysFunc(st, func(key string, _ *types.Value) bool {
		return slice.FindPos(filteredKeys, key) > -1
	})
}

func StructFilterKeysFunc(st *types.Struct, filter func(key string, val *types.Value) bool) *types.Struct {
	if st == nil || st.Fields == nil {
		return st
	}

	m := make(map[string]*types.Value, len(st.Fields))
	for k, v := range st.Fields {
		if filter(k, v) {
			m[k] = v
		}
	}
	return &types.Struct{Fields: m}
}

// StructCutKeys excludes provided keys reusing underlying pb values pointers
func StructCutKeys(st *types.Struct, excludeKeys []string) *types.Struct {
	return StructFilterKeysFunc(st, func(key string, _ *types.Value) bool {
		return slice.FindPos(excludeKeys, key) == -1
	})
}

func StructMerge(st1, st2 *types.Struct, copyVals bool) *types.Struct {
	var res *types.Struct
	if st1 == nil || st1.Fields == nil {
		return CopyStruct(st2, copyVals)
	}

	if st2 == nil || st2.Fields == nil {
		return CopyStruct(st1, copyVals)
	}

	res = CopyStruct(st1, copyVals)
	for k, v := range st2.Fields {
		if copyVals {
			res.Fields[k] = CopyVal(v)
		} else {
			res.Fields[k] = v
		}
	}

	return res
}

func DataviewSortsEqualSorted(sorts1, sorts2 []*model.BlockContentDataviewSort) bool {
	if len(sorts1) != len(sorts2) {
		return false
	}
	for i := range sorts1 {
		if !DataviewSortEqual(sorts1[i], sorts2[i]) {
			return false
		}
	}

	return true
}

func DataviewFiltersEqualSorted(filters1, filters2 []*model.BlockContentDataviewFilter) bool {
	if len(filters1) != len(filters2) {
		return false
	}
	for i := range filters1 {
		if !DataviewFilterEqual(filters1[i], filters2[i]) {
			return false
		}
	}

	return true
}

func DataviewViewsEqualSorted(views1, views2 []*model.BlockContentDataviewView) bool {
	if len(views1) != len(views2) {
		return false
	}
	for i := range views1 {
		if !DataviewViewEqual(views1[i], views2[i]) {
			return false
		}
	}

	return true
}

func DataviewSortEqual(sort1, sort2 *model.BlockContentDataviewSort) bool {
	if sort1 == nil && sort2 != nil {
		return false
	}
	if sort1 != nil && sort2 == nil {
		return false
	}
	if sort1 == nil && sort2 == nil {
		return true
	}
	if sort1.RelationKey != sort2.RelationKey {
		return false
	}
	if sort1.Type != sort2.Type {
		return false
	}
	return true
}

func DataviewFilterEqual(filter1, filter2 *model.BlockContentDataviewFilter) bool {
	if filter1 == nil && filter2 != nil {
		return false
	}
	if filter1 != nil && filter2 == nil {
		return false
	}
	if filter1 == nil && filter2 == nil {
		return true
	}
	if filter1.RelationKey != filter2.RelationKey {
		return false
	}
	if filter1.Condition != filter2.Condition {
		return false
	}
	if filter1.Operator != filter2.Operator {
		return false
	}
	if filter1.RelationProperty != filter2.RelationProperty {
		return false
	}
	return (*structpb.Value)(filter1.Value).EqualVT((*structpb.Value)(filter2.Value))
}

func DataviewViewEqual(view1, view2 *model.BlockContentDataviewView) bool {
	if view1 == nil && view2 != nil {
		return false
	}
	if view1 != nil && view2 == nil {
		return false
	}
	if view1 == nil && view2 == nil {
		return true
	}
	if view1.Id != view2.Id {
		return false
	}
	if view1.Name != view2.Name {
		return false
	}
	if view1.Type != view2.Type {
		return false
	}
	if !DataviewFiltersEqualSorted(view1.Filters, view2.Filters) {
		return false
	}
	if !DataviewSortsEqualSorted(view1.Sorts, view2.Sorts) {
		return false
	}
	if len(view1.Relations) != len(view2.Relations) {
		return false
		// todo add relations check
	}

	return true
}

func SortedRange(s *types.Struct, f func(k string, v *types.Value)) {
	if s == nil || s.Fields == nil {
		return
	}
	var keys = make([]string, 0, len(s.Fields))
	for k := range s.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f(k, s.Fields[k])
	}
}
