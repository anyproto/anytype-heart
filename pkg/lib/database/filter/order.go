package filter

import (
	"github.com/gogo/protobuf/types"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type Order interface {
	Compare(a, b Getter) int
	String() string
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

func (so SetOrder) String() (s string) {
	var ss []string
	for _, o := range so {
		ss = append(ss, o.String())
	}
	return strings.Join(ss, ", ")
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

func (ko KeyOrder) String() (s string) {
	s = ko.Key
	if ko.Type == model.BlockContentDataviewSort_Desc {
		s += " DESC"
	}
	return
}

type CustomOrder struct {
	Key string
	NeedOrder []*types.Value
}

func (co CustomOrder) Compare(a, b Getter) int {
	av := a.Get(co.Key)
	bv := b.Get(co.Key)
	for _, v := range co.NeedOrder {
		if v.Equal(av) {
			return -1
		}
		if v.Equal(bv) {
			return 1
		}
	}

	return 0
}

func (co CustomOrder) String() (s string) {
	var ss []string
	for _, v := range co.NeedOrder {
		ss = append(ss, v.String())
	}
	return strings.Join(ss, ", ")
}
