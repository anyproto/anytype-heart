package database

import (
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	time_util "github.com/anyproto/anytype-heart/util/time"
)

type dateOnlySort struct {
	arena       *anyenc.Arena
	relationKey string
	reverse     bool
	nulls       model.BlockContentDataviewSortEmptyType
}

func (s dateOnlySort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
		},
	}
}

func (s dateOnlySort) AppendKey(tuple anyenc.Tuple, v *anyenc.Value) anyenc.Tuple {
	defer func() {
		s.arena.Reset()
	}()
	val := v.Get(s.relationKey)
	var (
		empty bool
		ts    int64
	)
	if val != nil && val.Type() == anyenc.TypeNumber {
		tsFloat, _ := val.Float64()
		ts = time_util.CutToDay(time.Unix(int64(tsFloat), 0)).Unix()
	} else {
		empty = true
	}

	if empty {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return tuple.Append(s.arena.NewNull())
		} else {
			return tuple.AppendInverted(s.arena.NewNull())
		}
	}

	if s.reverse {
		return tuple.AppendInverted(s.arena.NewNumberFloat64(float64(ts)))
	} else {
		return tuple.Append(s.arena.NewNumberFloat64(float64(ts)))
	}
}

type emptyPlacementSort struct {
	arena       *anyenc.Arena
	relationKey string
	reverse     bool
	nulls       model.BlockContentDataviewSortEmptyType
	valType     anyenc.Type
}

func (s emptyPlacementSort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
		},
	}
}

func (s emptyPlacementSort) AppendKey(tuple anyenc.Tuple, v *anyenc.Value) anyenc.Tuple {
	defer func() {
		s.arena.Reset()
	}()
	val := v.Get(s.relationKey)

	if s.isEmpty(val) {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return tuple.Append(s.arena.NewNull())
		} else {
			return tuple.AppendInverted(s.arena.NewNull())
		}
	}

	if s.reverse {
		return tuple.AppendInverted(val)
	} else {
		return tuple.Append(val)
	}
}

func (s emptyPlacementSort) isEmpty(val *anyenc.Value) bool {
	if val == nil {
		return true
	}
	switch s.valType {
	case anyenc.TypeNull:
		return true
	case anyenc.TypeString:
		return len(val.GetStringBytes()) == 0
	case anyenc.TypeNumber:
		n, _ := val.Float64()
		return n == 0
	case anyenc.TypeFalse:
		return true
	case anyenc.TypeTrue:
		return false
	case anyenc.TypeArray:
		return len(val.GetArray()) == 0
	case anyenc.TypeObject:
		panic("not implemented")
	}
	return false
}

type textSort struct {
	arena          *anyenc.Arena
	collatorBuffer *collate.Buffer
	collator       *collate.Collator
	relationKey    string
	reverse        bool
	nulls          model.BlockContentDataviewSortEmptyType
}

func (s textSort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
			Path:  []string{s.relationKey},
		},
	}
}

func (s textSort) AppendKey(tuple anyenc.Tuple, v *anyenc.Value) anyenc.Tuple {
	defer func() {
		s.arena.Reset()
		s.collatorBuffer.Reset()
	}()
	val := v.GetStringBytes(s.relationKey)
	if s.relationKey == bundle.RelationKeyName.String() && len(val) == 0 {
		layout := model.ObjectTypeLayout(v.GetFloat64(bundle.RelationKeyResolvedLayout.String()))
		if layout == model.ObjectType_note {
			val = v.GetStringBytes(bundle.RelationKeySnippet.String())
		}
	}

	collated := s.collator.Key(s.collatorBuffer, val)
	if s.reverse {
		if s.nulls == model.BlockContentDataviewSort_Start && len(val) == 0 {
			return tuple.Append(s.arena.NewNull())
		} else {
			return tuple.AppendInverted(s.arena.NewStringBytes(collated))
		}
	} else {
		if s.nulls == model.BlockContentDataviewSort_End && len(val) == 0 {
			return tuple.AppendInverted(s.arena.NewNull())
		} else {
			return tuple.Append(s.arena.NewStringBytes(collated))
		}
	}
}

type tagStatusSort struct {
	arena       *anyenc.Arena
	relationKey string
	reverse     bool
	nulls       model.BlockContentDataviewSortEmptyType
	idToOrderId map[string]string
}

func (s tagStatusSort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
		},
	}
}

func (s tagStatusSort) AppendKey(tuple anyenc.Tuple, v *anyenc.Value) anyenc.Tuple {
	defer func() {
		s.arena.Reset()
	}()

	val := v.Get(s.relationKey)
	var sortKey string
	if val != nil && val.Type() == anyenc.TypeString {
		id, _ := val.StringBytes()
		sortKey = s.idToOrderId[string(id)]
	} else if val != nil && val.Type() == anyenc.TypeArray {
		arr, _ := val.Array()
		for _, it := range arr {
			id, _ := it.StringBytes()
			sortKey += s.idToOrderId[string(id)]
		}
	}

	if sortKey == "" {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return tuple.Append(s.arena.NewNull())
		} else if s.nulls == model.BlockContentDataviewSort_End {
			return tuple.AppendInverted(s.arena.NewNull())
		}
	}

	if s.reverse {
		return tuple.AppendInverted(s.arena.NewString(sortKey))
	} else {
		return tuple.Append(s.arena.NewString(sortKey))
	}
}

type boolSort struct {
	arena       *anyenc.Arena
	relationKey string
	reverse     bool
}

func (b boolSort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
		},
	}
}

func (b boolSort) AppendKey(tuple anyenc.Tuple, v *anyenc.Value) anyenc.Tuple {
	defer func() {
		b.arena.Reset()
	}()
	val := v.Get(b.relationKey)
	if val == nil {
		val = b.arena.NewFalse()
	}
	if b.reverse {
		return tuple.AppendInverted(val)
	} else {
		return tuple.Append(val)
	}
}
