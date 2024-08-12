package database

import (
	"github.com/anyproto/any-store/encoding"
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
	AnystoreSort() query.Sort
}

// ObjectStore interface is used to enrich filters
type ObjectStore interface {
	Query(q Query) (records []Record, err error)
	QueryRaw(filters *Filters, limit int, offset int) ([]Record, error)
	GetRelationFormatByKey(key string) (model.RelationFormat, error)
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

func (so SetOrder) AnystoreSort() query.Sort {
	if len(so) == 0 {
		return nil
	}
	sorts := make(query.Sorts, 0, len(so))
	for _, o := range so {
		sorts = append(sorts, o.AnystoreSort())
	}
	return sorts
}

type KeyOrder struct {
	SpaceID        string
	Key            string
	Type           model.BlockContentDataviewSortType
	EmptyPlacement model.BlockContentDataviewSortEmptyType
	relationFormat model.RelationFormat
	IncludeTime    bool
	Store          ObjectStore
	Options        map[string]string
	arena          *fastjson.Arena
	collatorBuffer *collate.Buffer
	collator       *collate.Collator
}

func (ko *KeyOrder) ensureCollator() {
	if ko.collator == nil {
		ko.collator = collate.New(language.Und, collate.IgnoreCase)
		ko.collatorBuffer = &collate.Buffer{}
	}
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

func (ko *KeyOrder) AnystoreSort() query.Sort {
	switch ko.relationFormat {
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
		return ko.tagStatusSort()
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

func (ko *KeyOrder) tagStatusSort() query.Sort {
	if ko.Options == nil {
		ko.Options = make(map[string]string)
	}
	if len(ko.Options) == 0 && ko.Store != nil {
		ko.Options = optionsToMap(ko.SpaceID, ko.Key, ko.Store)
	}
	return tagStatusSort{
		arena:       ko.arena,
		relationKey: ko.Key,
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:       ko.EmptyPlacement,
		idToName:    ko.Options,
	}
}

func (ko *KeyOrder) emptyPlacementSort(valType fastjson.Type) query.Sort {
	return emptyPlacementSort{
		arena:       ko.arena,
		relationKey: ko.Key,
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:       ko.EmptyPlacement,
		valType:     valType,
	}
}

func (ko *KeyOrder) dateOnlySort() query.Sort {
	return dateOnlySort{
		arena:       ko.arena,
		relationKey: ko.Key,
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:       ko.EmptyPlacement,
	}
}

func (ko *KeyOrder) textSort() query.Sort {
	ko.ensureCollator()
	return textSort{
		arena:          ko.arena,
		collator:       ko.collator,
		collatorBuffer: ko.collatorBuffer,
		relationKey:    ko.Key,
		reverse:        ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:          ko.EmptyPlacement,
	}
}

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
		ko.ensureCollator()
		comp = ko.collator.CompareString(av.GetStringValue(), bv.GetStringValue())
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
	if ko.relationFormat == model.RelationFormat_tag || ko.relationFormat == model.RelationFormat_status {
		av = ko.GetOptionValue(av)
		bv = ko.GetOptionValue(bv)
	}
	return av, bv
}

func (ko *KeyOrder) tryExtractDateTime(av *types.Value, bv *types.Value) (*types.Value, *types.Value) {
	if ko.relationFormat == model.RelationFormat_date && !ko.IncludeTime {
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

func newCustomOrder(arena *fastjson.Arena, key string, idsIndices map[string]int, keyOrd *KeyOrder) customOrder {
	return customOrder{
		arena:        arena,
		Key:          key,
		NeedOrderMap: idsIndices,
		KeyOrd:       keyOrd,
	}
}

type customOrder struct {
	arena        *fastjson.Arena
	Key          string
	NeedOrderMap map[string]int
	KeyOrd       *KeyOrder

	buf []byte
}

func (co customOrder) AppendKey(k []byte, v *fastjson.Value) []byte {
	defer func() {
		co.arena.Reset()
		co.buf = co.buf[:0]
	}()

	var rawValue string
	if val := v.Get(co.Key); val != nil {
		rawValue = string(val.MarshalTo(co.buf))
	}
	idx, ok := co.NeedOrderMap[rawValue]
	if !ok {
		anystoreSort := co.KeyOrd.AnystoreSort()
		// Push to the end
		k = encoding.AppendJSONValue(k, co.arena.NewNumberInt(len(co.NeedOrderMap)))
		// and add sorting
		return anystoreSort.AppendKey(k, v)
	}
	return encoding.AppendJSONValue(k, co.arena.NewNumberInt(idx))
}

func (co customOrder) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
			Path:  []string{co.Key},
		},
	}
}

func (co customOrder) AnystoreSort() query.Sort {
	return co
}

func (co customOrder) getStringVal(val *types.Value) string {
	defer func() {
		co.arena.Reset()
		co.buf = co.buf[:0]
	}()

	jsonVal := pbtypes.ProtoValueToJson(co.arena, val)
	if jsonVal == nil {
		return ""
	}
	return string(jsonVal.MarshalTo(co.buf))
}

func (co customOrder) Compare(a, b *types.Struct) int {

	aID, okA := co.NeedOrderMap[co.getStringVal(pbtypes.Get(a, co.Key))]
	bID, okB := co.NeedOrderMap[co.getStringVal(pbtypes.Get(b, co.Key))]

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
