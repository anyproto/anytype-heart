package pbtypes

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
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
		} else if !v2.Equal(v1) {
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
	if st == nil || st.Fields == nil {
		return st
	}

	m := make(map[string]*types.Value, len(st.Fields))
	for k, v := range st.Fields {
		if slice.FindPos(filteredKeys, k) > -1 {
			m[k] = v
		}
	}

	return &types.Struct{Fields: m}
}

// StructCutKeys excludes provided keys reusing underlying pb values pointers
func StructCutKeys(st *types.Struct, excludeKeys []string) *types.Struct {
	if st == nil || st.Fields == nil {
		return st
	}

	m := make(map[string]*types.Value, len(st.Fields))
	for k, v := range st.Fields {
		if slice.FindPos(excludeKeys, k) == -1 {
			m[k] = v
		}
	}

	return &types.Struct{Fields: m}
}

// StructDiff returns pb struct which contains:
// - st2 fields that not exist in st1
// - st2 fields that not equal to ones exist in st1
// - nil map value for st1 fields not exist in st2
// In case st1 and st2 are equal returns nil
func StructDiff(st1, st2 *types.Struct) *types.Struct {
	var diff *types.Struct
	if st1 == nil {
		return st2
	}
	if st2 == nil {
		diff = &types.Struct{Fields: map[string]*types.Value{}}
		for k, _ := range st1.Fields {
			diff.Fields[k] = nil
		}
		return diff
	}

	for k, v2 := range st2.Fields {
		v1, ok := st1.Fields[k]
		if !ok || !v1.Equal(v2) {
			if diff == nil {
				diff = &types.Struct{Fields: map[string]*types.Value{}}
			}
			diff.Fields[k] = v2
		}
	}

	for k, _ := range st1.Fields {
		_, ok := st2.Fields[k]
		if !ok {
			if diff == nil {
				diff = &types.Struct{Fields: map[string]*types.Value{}}
			}
			diff.Fields[k] = nil
		}
	}

	return diff
}

func RelationsDiff(rels1, rels2 []*model.Relation) (added []*model.Relation, updated []*model.Relation, removed []string) {
	for i := 0; i < len(rels2); i++ {
		if r := GetRelation(rels1, rels2[i].Key); r == nil {
			added = append(added, rels2[i])
			continue
		} else {
			if !RelationEqual(r, rels2[i]) {
				updated = append(updated, rels2[i])
				continue
			}
		}
	}

	for i := 0; i < len(rels1); i++ {
		if r := GetRelation(rels2, rels1[i].Key); r == nil {
			removed = append(removed, rels1[i].Key)
			continue
		}
	}

	return
}

func RelationsEqual(rels1 []*model.Relation, rels2 []*model.Relation) (equal bool) {
	if len(rels1) != len(rels2) {
		return false
	}

	for i := 0; i < len(rels2); i++ {
		if !RelationEqual(rels1[i], rels2[i]) {
			return false
		}
	}

	return true
}

func RelationEqualOmitDictionary(rel1 *model.Relation, rel2 *model.Relation) (equal bool) {
	if rel1 == nil && rel2 != nil {
		return false
	}
	if rel2 == nil && rel1 != nil {
		return false
	}
	if rel2 == nil && rel1 == nil {
		return true
	}

	if rel1.Key != rel2.Key {
		return false
	}
	if rel1.Format != rel2.Format {
		return false
	}
	if rel1.Name != rel2.Name {
		return false
	}
	if rel1.DefaultValue.Compare(rel2.DefaultValue) != 0 {
		return false
	}
	if rel1.DataSource != rel2.DataSource {
		return false
	}
	if rel1.Hidden != rel2.Hidden {
		return false
	}
	if rel1.ReadOnly != rel2.ReadOnly {
		return false
	}
	if rel1.Multi != rel2.Multi {
		return false
	}
	if rel1.MaxCount != rel2.MaxCount {
		return false
	}
	if !slice.SortedEquals(rel1.ObjectTypes, rel2.ObjectTypes) {
		return false
	}

	return true
}

// RelationCompatible returns if provided relations are compatible in terms of underlying data format
// e.g. it is ok if relation can have a different name and selectDict, while having the same key and format
func RelationCompatible(rel1 *model.Relation, rel2 *model.Relation) (equal bool) {
	if rel1 == nil && rel2 != nil {
		return false
	}
	if rel2 == nil && rel1 != nil {
		return false
	}
	if rel2 == nil && rel1 == nil {
		return true
	}

	if rel1.Key != rel2.Key {
		return false
	}
	if rel1.Format != rel2.Format {
		return false
	}

	// todo: should we compare objectType here?

	return true
}

func RelationEqual(rel1 *model.Relation, rel2 *model.Relation) (equal bool) {
	if !RelationEqualOmitDictionary(rel1, rel2) {
		return false
	}

	return RelationSelectDictEqual(rel1.SelectDict, rel2.SelectDict)
}

func RelationSelectDictEqual(dict1, dict2 []*model.RelationOption) bool {
	if len(dict1) != len(dict2) {
		return false
	}

	for i := 0; i < len(dict1); i++ {
		if !OptionEqualOmitScope(dict1[i], dict2[i]) {
			return false
		}
	}

	return true
}

func OptionEqualOmitScope(opt1, opt2 *model.RelationOption) bool {
	if (opt1 == nil) && (opt2 != nil) {
		return false
	}

	if (opt1 != nil) && (opt2 == nil) {
		return false
	}

	if opt1 == nil && opt2 == nil {
		return true
	}

	if opt1.Id != opt2.Id {
		return false
	}
	if opt1.Text != opt2.Text {
		return false
	}
	if opt1.Color != opt2.Color {
		return false
	}
	return true
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
	if filter1.Value.Compare(filter2.Value) != 0 {
		return false
	}

	return true
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
