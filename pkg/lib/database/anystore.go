package database

import (
	"time"

	"github.com/anyproto/any-store/encoding"
	"github.com/anyproto/any-store/key"
	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	time_util "github.com/anyproto/anytype-heart/util/time"
)

type dateOnlySort struct {
	relationKey string
	reverse     bool
	nulls       model.BlockContentDataviewSortEmptyType
	valType     fastjson.Type
}

func (s dateOnlySort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
		},
	}
}

func (s dateOnlySort) AppendKey(k key.Key, v *fastjson.Value) key.Key {
	arena := &fastjson.Arena{}
	val := v.Get(s.relationKey)
	var (
		empty bool
		ts    int64
	)
	if val.Type() == fastjson.TypeNumber {
		tsFloat, _ := val.Float64()
		ts = time_util.CutToDay(time.Unix(int64(tsFloat), 0)).Unix()
	} else {
		empty = true
	}

	if empty {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return encoding.AppendJSONValue(k, arena.NewNull())
		} else {
			return encoding.AppendInvertedJSON(k, arena.NewNull())
		}
	}

	if s.reverse {
		return encoding.AppendInvertedJSON(k, arena.NewNumberFloat64(float64(ts)))
	} else {
		return encoding.AppendJSONValue(k, arena.NewNumberFloat64(float64(ts)))
	}
}

type emptyPlacementSort struct {
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

func (s emptyPlacementSort) AppendKey(k key.Key, v *fastjson.Value) key.Key {
	arena := &fastjson.Arena{}
	val := v.Get(s.relationKey)

	if s.isEmpty(val) {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return encoding.AppendJSONValue(k, arena.NewNull())
		} else {
			return encoding.AppendInvertedJSON(k, arena.NewNull())
		}
	}

	if s.reverse {
		return encoding.AppendInvertedJSON(k, val)
	} else {
		return encoding.AppendJSONValue(k, val)
	}
}

func (s emptyPlacementSort) isEmpty(val *fastjson.Value) bool {
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
	relationKey string
	reverse     bool
	nulls       model.BlockContentDataviewSortEmptyType
}

func (s textSort) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
			Path:  []string{s.relationKey},
		},
	}
}

func (s textSort) AppendKey(k key.Key, v *fastjson.Value) key.Key {
	// TODO Pass buffer, arena and collator
	coll := collate.New(language.Und, collate.IgnoreCase)

	val := v.GetStringBytes(s.relationKey)

	if s.relationKey == bundle.RelationKeyName.String() && len(val) == 0 {
		layout := model.ObjectTypeLayout(v.GetFloat64(bundle.RelationKeyLayout.String()))
		if layout == model.ObjectType_note {
			val = v.GetStringBytes(bundle.RelationKeySnippet.String())
		}
	}

	arena := &fastjson.Arena{}

	collated := coll.Key(&collate.Buffer{}, val)
	if s.reverse {
		if s.nulls == model.BlockContentDataviewSort_Start && len(val) == 0 {
			return encoding.AppendJSONValue(k, arena.NewNull())
		} else {
			return encoding.AppendInvertedJSON(k, arena.NewStringBytes(collated))
		}
	} else {
		if s.nulls == model.BlockContentDataviewSort_End && len(val) == 0 {
			return encoding.AppendInvertedJSON(k, arena.NewNull())
		} else {
			return encoding.AppendJSONValue(k, arena.NewStringBytes(collated))
		}
	}
}

type tagStatusSort struct {
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

func (s tagStatusSort) AppendKey(k key.Key, v *fastjson.Value) key.Key {
	arena := &fastjson.Arena{}

	val := v.Get(s.relationKey)
	var sortKey string
	if val.Type() == fastjson.TypeString {
		id, _ := val.StringBytes()
		sortKey = s.idToName[string(id)]
	} else if val.Type() == fastjson.TypeArray {
		arr, _ := val.Array()
		for _, it := range arr {
			id, _ := it.StringBytes()
			sortKey += s.idToName[string(id)]
		}
	}

	if sortKey == "" {
		if s.nulls == model.BlockContentDataviewSort_Start {
			return encoding.AppendJSONValue(k, arena.NewNull())
		} else if s.nulls == model.BlockContentDataviewSort_End {
			return encoding.AppendInvertedJSON(k, arena.NewNull())
		}
	}

	if s.reverse {
		return encoding.AppendInvertedJSON(k, arena.NewString(sortKey))
	} else {
		return encoding.AppendJSONValue(k, arena.NewString(sortKey))
	}
}
