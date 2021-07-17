package filter

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type Order interface {
	Compare(a, b Getter) int
}

type SetOrder []Order

func (so SetOrder) Compare(a, b Getter) int {
	for _, o := range so {
		if comp := o.Compare(a, b); comp != 0 {
			return comp
		}
	}
	return 0
}

type KeyOrder struct {
	Key       string
	Type      model.BlockContentDataviewSortType
	EmptyLast bool // consider empty strings as the last, not first
}

func (ko KeyOrder) Compare(a, b Getter) int {
	av := a.Get(ko.Key)
	bv := b.Get(ko.Key)
	comp := av.Compare(bv)

	_, aString := av.GetKind().(*types.Value_StringValue)
	_, bString := bv.GetKind().(*types.Value_StringValue)
	if ko.EmptyLast && (aString || av == nil) && (bString || bv == nil) {
		if av.GetStringValue() == "" && bv.GetStringValue() != "" {
			comp = 1
		} else if av.GetStringValue() != "" && bv.GetStringValue() == "" {
			comp = -1
		}
	}
	if ko.Type == model.BlockContentDataviewSort_Desc {
		comp = -comp
	}
	return comp
}
