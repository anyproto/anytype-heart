package database

import (
	"time"

	"github.com/anyproto/any-store/encoding"
	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	time_util "github.com/anyproto/anytype-heart/util/time"
)

type dateOnlySort struct {
	arena       *fastjson.Arena
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

func (s dateOnlySort) AppendKey(k []byte, v *fastjson.Value) []byte {
	defer func() {
		s.arena.Reset()
	}()
	val := v.Get(s.relationKey)
	var (
		empty bool
		ts    int64
	)
	if val != nil && val.Type() == fastjson.TypeNumber {
		tsFloat, _ := val.Float64()
		ts = time_util.CutToDay(time.Unix(int64(tsFloat), 0)).Unix()
	} else {
		empty = true
	}

	if empty {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return encoding.AppendJSONValue(k, s.arena.NewNull())
		} else {
			return encoding.AppendInvertedJSON(k, s.arena.NewNull())
		}
	}

	if s.reverse {
		return encoding.AppendInvertedJSON(k, s.arena.NewNumberFloat64(float64(ts)))
	} else {
		return encoding.AppendJSONValue(k, s.arena.NewNumberFloat64(float64(ts)))
	}
}

type emptyPlacementSort struct {
	arena       *fastjson.Arena
	relationKey string
	reverse     bool
	nulls       model.BlockContentDataviewSortEmptyType
	valType     fastjson.Type
}

func (s emptyPlacementSort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
		},
	}
}

func (s emptyPlacementSort) AppendKey(k []byte, v *fastjson.Value) []byte {
	defer func() {
		s.arena.Reset()
	}()
	val := v.Get(s.relationKey)

	if s.isEmpty(val) {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return encoding.AppendJSONValue(k, s.arena.NewNull())
		} else {
			return encoding.AppendInvertedJSON(k, s.arena.NewNull())
		}
	}

	if s.reverse {
		return encoding.AppendInvertedJSON(k, val)
	} else {
		return encoding.AppendJSONValue(k, val)
	}
}

func (s emptyPlacementSort) isEmpty(val *fastjson.Value) bool {
	if val == nil {
		return true
	}
	switch s.valType {
	case fastjson.TypeNull:
		return true
	case fastjson.TypeString:
		return len(val.GetStringBytes()) == 0
	case fastjson.TypeNumber:
		n, _ := val.Float64()
		return n == 0
	case fastjson.TypeFalse:
		return true
	case fastjson.TypeTrue:
		return false
	case fastjson.TypeArray:
		return len(val.GetArray()) == 0
	case fastjson.TypeObject:
		panic("not implemented")
	}
	return false
}

type textSort struct {
	arena          *fastjson.Arena
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

func (s textSort) AppendKey(k []byte, v *fastjson.Value) []byte {
	defer func() {
		s.arena.Reset()
		s.collatorBuffer.Reset()
	}()

	val := v.GetStringBytes(s.relationKey)

	if s.relationKey == bundle.RelationKeyName.String() && len(val) == 0 {
		layout := model.ObjectTypeLayout(v.GetFloat64(bundle.RelationKeyLayout.String()))
		if layout == model.ObjectType_note {
			val = v.GetStringBytes(bundle.RelationKeySnippet.String())
		}
	}

	collated := s.collator.Key(s.collatorBuffer, val)
	if s.reverse {
		if s.nulls == model.BlockContentDataviewSort_Start && len(val) == 0 {
			return encoding.AppendJSONValue(k, s.arena.NewNull())
		} else {
			return encoding.AppendInvertedJSON(k, s.arena.NewStringBytes(collated))
		}
	} else {
		if s.nulls == model.BlockContentDataviewSort_End && len(val) == 0 {
			return encoding.AppendInvertedJSON(k, s.arena.NewNull())
		} else {
			return encoding.AppendJSONValue(k, s.arena.NewStringBytes(collated))
		}
	}
}

type tagStatusSort struct {
	arena       *fastjson.Arena
	relationKey string
	reverse     bool
	nulls       model.BlockContentDataviewSortEmptyType
	idToName    map[string]string
}

func (s tagStatusSort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
		},
	}
}

func (s tagStatusSort) AppendKey(k []byte, v *fastjson.Value) []byte {
	defer func() {
		s.arena.Reset()
	}()

	val := v.Get(s.relationKey)
	var sortKey string
	if val != nil && val.Type() == fastjson.TypeString {
		id, _ := val.StringBytes()
		sortKey = s.idToName[string(id)]
	} else if val != nil && val.Type() == fastjson.TypeArray {
		arr, _ := val.Array()
		for _, it := range arr {
			id, _ := it.StringBytes()
			sortKey += s.idToName[string(id)]
		}
	}

	if sortKey == "" {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return encoding.AppendJSONValue(k, s.arena.NewNull())
		} else if s.nulls == model.BlockContentDataviewSort_End {
			return encoding.AppendInvertedJSON(k, s.arena.NewNull())
		}
	}

	if s.reverse {
		return encoding.AppendInvertedJSON(k, s.arena.NewString(sortKey))
	} else {
		return encoding.AppendJSONValue(k, s.arena.NewString(sortKey))
	}
}
