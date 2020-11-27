package pbtypes

import (
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func RelationsEqual(rels1 []*pbrelation.Relation, rels2 []*pbrelation.Relation) (equal bool) {
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

func RelationEqualOmitDictionary(rel1 *pbrelation.Relation, rel2 *pbrelation.Relation) (equal bool) {
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

	if slice.SortedEquals(rel1.ObjectTypes, rel2.ObjectTypes) {
		return false
	}

	return true
}

// RelationCompatible returns if provided relations are compatible in terms of underlying data format
// e.g. it is ok if relation can have a different name and selectDict, while having the same key and format
func RelationCompatible(rel1 *pbrelation.Relation, rel2 *pbrelation.Relation) (equal bool) {
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

func RelationEqual(rel1 *pbrelation.Relation, rel2 *pbrelation.Relation) (equal bool) {
	if !RelationEqualOmitDictionary(rel1, rel2) {
		return false
	}

	return RelationSelectDictEqual(rel1.SelectDict, rel2.SelectDict)
}

func RelationSelectDictEqual(dict1, dict2 []*pbrelation.RelationSelectOption) bool {
	if len(dict1) != len(dict2) {
		return false
	}

	for i := 0; i < len(dict1); i++ {
		if dict1[i].Id != dict2[i].Id {
			return false
		}
		if dict1[i].Text != dict2[i].Text {
			return false
		}

		if dict1[i].Color != dict2[i].Color {
			return false
		}
	}

	return true
}
