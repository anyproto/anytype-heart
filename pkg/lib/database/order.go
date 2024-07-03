package database

import (
	"strings"
	"time"

	"github.com/anyproto/any-store/encoding"
	"github.com/anyproto/any-store/key"
	"github.com/anyproto/any-store/query"
	"github.com/gogo/protobuf/types"
	"github.com/valyala/fastjson"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	time_util "github.com/anyproto/anytype-heart/util/time"
)

type Order interface {
	Compare(a, b *types.Struct) int
	Compile() query.Sort
	String() string
}

// ObjectStore interface is used to enrich filters
type ObjectStore interface {
	Query(q Query) (records []Record, err error)
	QueryRaw(filters *Filters, limit int, offset int) ([]Record, error)
}

type SetOrder []Order

func (so SetOrder) Compare(a, b *types.Struct) int {
	for _, o := range so {
		if comp := o.Compare(a, b); comp != 0 {
			return comp
		}
	}
	return 0
}

func (so SetOrder) Compile() query.Sort {
	if len(so) == 0 {
		return nil
	}
	sorts := make(query.Sorts, 0, len(so))
	for _, o := range so {
		sorts = append(sorts, o.Compile())
	}
	return sorts
}

func (so SetOrder) String() (s string) {
	var ss []string
	for _, o := range so {
		ss = append(ss, o.String())
	}
	return strings.Join(ss, ", ")
}

type KeyOrder struct {
	SpaceID        string
	Key            string
	Type           model.BlockContentDataviewSortType
	EmptyPlacement model.BlockContentDataviewSortEmptyType
	RelationFormat model.RelationFormat
	IncludeTime    bool
	Store          ObjectStore
	Options        map[string]string
	comparator     *collate.Collator
}

func (ko *KeyOrder) Compare(a, b *types.Struct) int {
	av := pbtypes.Get(a, ko.Key)
	bv := pbtypes.Get(b, ko.Key)

	av, bv = ko.tryExtractSnippet(a, b, av, bv)
	av, bv = ko.tryExtractDateTime(av, bv)
	av, bv = ko.tryExtractTag(av, bv)

	comp := ko.tryCompareStrings(av, bv)
	if comp == 0 {
		comp = av.Compare(bv)
	}
	comp = ko.tryAdjustEmptyPositions(av, bv, comp)
	if ko.Type == model.BlockContentDataviewSort_Desc {
		comp = -comp
	}
	return comp
}

func (ko *KeyOrder) Compile() query.Sort {
	switch ko.RelationFormat {
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		return ko.textSort()
	case model.RelationFormat_number:
		return ko.basicSort(fastjson.TypeNumber)
	case model.RelationFormat_date:
		if ko.IncludeTime {
			return ko.basicSort(fastjson.TypeNumber)
		} else {
			return ko.dateOnlySort()
		}
	case model.RelationFormat_object, model.RelationFormat_file:
		return ko.basicSort(fastjson.TypeString)
	case model.RelationFormat_url, model.RelationFormat_email, model.RelationFormat_phone, model.RelationFormat_emoji:
		return ko.basicSort(fastjson.TypeString)
	case model.RelationFormat_tag, model.RelationFormat_status:
		// TODO tag-status sort
		return ko.basicSort(fastjson.TypeArray)
	default:
		return ko.basicSort(fastjson.TypeString)
	}
}

func (ko *KeyOrder) basicSort(valType fastjson.Type) query.Sort {
	if ko.EmptyPlacement == model.BlockContentDataviewSort_Start && ko.Type == model.BlockContentDataviewSort_Desc {
		return ko.emptyPlacementSort(valType)
	} else if ko.EmptyPlacement == model.BlockContentDataviewSort_End && ko.Type == model.BlockContentDataviewSort_Asc {
		return ko.emptyPlacementSort(valType)
	} else {
		return &query.SortField{
			Path:    []string{ko.Key},
			Reverse: ko.Type == model.BlockContentDataviewSort_Desc,
			Field:   ko.Key,
		}
	}
}

func (ko *KeyOrder) emptyPlacementSort(valType fastjson.Type) query.Sort {
	return emptyPlacementSort{
		relationKey: ko.Key,
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:       ko.EmptyPlacement,
		valType:     valType,
	}
}

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

func (ko *KeyOrder) dateOnlySort() query.Sort {
	return dateOnlySort{
		relationKey: ko.Key,
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:       ko.EmptyPlacement,
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
	// TODO note layout check

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

func (ko *KeyOrder) textSort() query.Sort {
	return textSort{
		relationKey: ko.Key,
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:       ko.EmptyPlacement,
	}
}

// func (ko *KeyOrder) sort() {
// 	if ko.EmptyPlacement == model.BlockContentDataviewSort_NotSpecified {
// 		if ko.Key == bundle.RelationKeyName.String() && getLayout(getter) == model.ObjectType_note {
// 			// custom: order by (name OR snippet)
// 		} else if ko.RelationFormat == model.RelationFormat_date && !ko.IncludeTime {
// 			// custom: cut out time
// 		} else if ko.RelationFormat == model.RelationFormat_tag || ko.RelationFormat == model.RelationFormat_status {
// 			// custom: order by tag and status Name
// 		} else {
// 			// compare with collate collate.New(language.Und, collate.IgnoreCase)
// 		}
// 	} else if ko.EmptyPlacement == model.BlockContentDataviewSort_Start {
// 		// basic comparison with collate, but only if ORDER BY always put nulls first
// 	} else {
// 		// custom
// 	}
// }

func (ko *KeyOrder) tryAdjustEmptyPositions(av *types.Value, bv *types.Value, comp int) int {
	if ko.EmptyPlacement == model.BlockContentDataviewSort_NotSpecified {
		return comp
	}
	_, aNull := av.GetKind().(*types.Value_NullValue)
	_, bNull := bv.GetKind().(*types.Value_NullValue)
	if av == nil {
		aNull = true
	}
	if bv == nil {
		bNull = true
	}
	if aNull && bNull {
		comp = 0
	} else if aNull {
		comp = 1
	} else if bNull {
		comp = -1
	} else {
		return comp
	}

	comp = ko.tryFlipComp(comp)
	return comp
}

func (ko *KeyOrder) tryCompareStrings(av *types.Value, bv *types.Value) int {
	comp := 0
	_, aString := av.GetKind().(*types.Value_StringValue)
	_, bString := bv.GetKind().(*types.Value_StringValue)
	if ko.isSpecialSortOfEmptyValuesNeed(av, bv, aString, bString) {
		if av.GetStringValue() == "" && bv.GetStringValue() != "" {
			comp = 1
		} else if av.GetStringValue() != "" && bv.GetStringValue() == "" {
			comp = -1
		}
	}
	if aString && bString && comp == 0 {
		ko.ensureComparator()
		comp = ko.comparator.CompareString(av.GetStringValue(), bv.GetStringValue())
	}
	if av.GetStringValue() == "" || bv.GetStringValue() == "" {
		comp = ko.tryFlipComp(comp)
	}
	return comp
}

func (ko *KeyOrder) tryFlipComp(comp int) int {
	if ko.Type == model.BlockContentDataviewSort_Desc && ko.EmptyPlacement == model.BlockContentDataviewSort_End ||
		ko.Type == model.BlockContentDataviewSort_Asc && ko.EmptyPlacement == model.BlockContentDataviewSort_Start {
		comp = -comp
	}
	return comp
}

func (ko *KeyOrder) isSpecialSortOfEmptyValuesNeed(av *types.Value, bv *types.Value, aString bool, bString bool) bool {
	return (ko.EmptyPlacement != model.BlockContentDataviewSort_NotSpecified) &&
		(aString || av == nil) && (bString || bv == nil)
}

func (ko *KeyOrder) tryExtractTag(av *types.Value, bv *types.Value) (*types.Value, *types.Value) {
	if ko.RelationFormat == model.RelationFormat_tag || ko.RelationFormat == model.RelationFormat_status {
		av = ko.GetOptionValue(av)
		bv = ko.GetOptionValue(bv)
	}
	return av, bv
}

func (ko *KeyOrder) tryExtractDateTime(av *types.Value, bv *types.Value) (*types.Value, *types.Value) {
	if ko.RelationFormat == model.RelationFormat_date && !ko.IncludeTime {
		av = time_util.CutValueToDay(av)
		bv = time_util.CutValueToDay(bv)
	}
	return av, bv
}

func (ko *KeyOrder) tryExtractSnippet(a *types.Struct, b *types.Struct, av *types.Value, bv *types.Value) (*types.Value, *types.Value) {
	av = ko.trySubstituteSnippet(a, av)
	bv = ko.trySubstituteSnippet(b, bv)
	return av, bv
}

func (ko *KeyOrder) trySubstituteSnippet(getter *types.Struct, value *types.Value) *types.Value {
	if ko.Key == bundle.RelationKeyName.String() && getLayout(getter) == model.ObjectType_note {
		value = pbtypes.Get(getter, bundle.RelationKeyName.String())
		if value == nil {
			value = pbtypes.Get(getter, bundle.RelationKeySnippet.String())
		}
	}
	return value
}

func getLayout(getter *types.Struct) model.ObjectTypeLayout {
	rawLayout := pbtypes.Get(getter, bundle.RelationKeyLayout.String()).GetNumberValue()
	return model.ObjectTypeLayout(int32(rawLayout))
}

func (ko *KeyOrder) GetOptionValue(value *types.Value) *types.Value {
	if ko.Options == nil {
		ko.Options = make(map[string]string)
	}

	if len(ko.Options) == 0 && ko.Store != nil {
		ko.Options = optionsToMap(ko.SpaceID, ko.Key, ko.Store)
	}

	res := ""
	for _, optID := range pbtypes.GetStringListValue(value) {
		res += ko.Options[optID]
	}

	return pbtypes.String(res)
}

func (ko *KeyOrder) String() (s string) {
	s = ko.Key
	if ko.Type == model.BlockContentDataviewSort_Desc {
		s += " DESC"
	}
	return
}

func (ko *KeyOrder) ensureComparator() {
	if ko.comparator == nil {
		ko.comparator = collate.New(language.Und, collate.IgnoreCase)
	}
}

func NewCustomOrder(key string, idsIndices map[string]int, keyOrd KeyOrder) CustomOrder {
	return CustomOrder{
		Key:          key,
		NeedOrderMap: idsIndices,
		KeyOrd:       keyOrd,
	}
}

type CustomOrder struct {
	Key          string
	NeedOrderMap map[string]int
	KeyOrd       KeyOrder
}

func (co CustomOrder) AppendKey(k key.Key, v *fastjson.Value) key.Key {
	arena := &fastjson.Arena{}
	val := v.GetStringBytes(co.Key)
	idx, ok := co.NeedOrderMap[string(val)]
	if !ok {
		compiled := co.KeyOrd.Compile()
		// Push to the end
		k = encoding.AppendJSONValue(k, arena.NewNumberInt(len(co.NeedOrderMap)))
		// and add sorting
		return compiled.AppendKey(k, v)
	}
	return encoding.AppendJSONValue(k, arena.NewNumberInt(idx))
}

func (co CustomOrder) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
			Path:  []string{co.Key},
		},
	}
}

func (co CustomOrder) Compile() query.Sort {
	return co
}

func (co CustomOrder) Compare(a, b *types.Struct) int {
	aID, okA := co.NeedOrderMap[pbtypes.Get(a, co.Key).GetStringValue()]
	bID, okB := co.NeedOrderMap[pbtypes.Get(b, co.Key).GetStringValue()]

	if okA && okB {
		if aID == bID {
			return 0
		}

		if aID < bID {
			return -1
		}
		return 1
	}

	if okA {
		return -1
	}
	if okB {
		return 1
	}

	return co.KeyOrd.Compare(a, b)
}

func (co CustomOrder) String() (s string) {
	ss := make([]string, len(co.NeedOrderMap))
	for key, id := range co.NeedOrderMap {
		ss[id] = key
	}
	return strings.Join(ss, ", ")
}
