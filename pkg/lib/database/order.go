package database

import (
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	timeutil "github.com/anyproto/anytype-heart/util/time"
)

type Order interface {
	Compare(a, b *domain.Details, orderIdsMap map[domain.RelationKey]map[string]string) int
	AnystoreSort() query.Sort
}

// ObjectStore interface is used to enrich filters
type ObjectStore interface {
	SpaceId() string
	Query(q Query) (records []Record, err error)
	QueryRaw(filters *Filters, limit int, offset int) ([]Record, error)
	GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error)
	ListRelationOptions(relationKey domain.RelationKey) (options []*model.RelationOption, err error)
}

type SetOrder []Order

func (so SetOrder) Compare(a, b *domain.Details, orderIdsMap map[domain.RelationKey]map[string]string) int {
	for _, o := range so {
		if comp := o.Compare(a, b, orderIdsMap); comp != 0 {
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
	Key                domain.RelationKey
	Type               model.BlockContentDataviewSortType
	EmptyPlacement     model.BlockContentDataviewSortEmptyType
	relationFormat     model.RelationFormat
	IncludeTime        bool
	objectStore        ObjectStore
	optionsIdToOrderId map[string]string
	arena              *anyenc.Arena
	collatorBuffer     *collate.Buffer
	collator           *collate.Collator
	disableCollator    bool
}

func (ko *KeyOrder) ensureCollator() {
	if ko.collator == nil {
		ko.collator = collate.New(language.Und, collate.IgnoreCase)
		ko.collatorBuffer = &collate.Buffer{}
	}
}

func (ko *KeyOrder) Compare(a, b *domain.Details, orderIdsMap map[domain.RelationKey]map[string]string) int {
	av := a.Get(ko.Key)
	bv := b.Get(ko.Key)

	av, bv = ko.tryExtractSnippet(a, b, av, bv)
	av, bv = ko.tryExtractDateTime(av, bv)

	var orderIds map[string]string
	if orderIdsMap != nil {
		orderIds = orderIdsMap[ko.Key]
	}
	av, bv = ko.tryExtractTag(av, bv, orderIds)
	av, bv = ko.tryExtractBool(av, bv)

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
		if ko.disableCollator {
			return ko.basicSort(anyenc.TypeString)
		}
		return ko.textSort()
	case model.RelationFormat_number:
		return ko.basicSort(anyenc.TypeNumber)
	case model.RelationFormat_date:
		if ko.IncludeTime {
			return ko.basicSort(anyenc.TypeNumber)
		} else {
			return ko.dateOnlySort()
		}
	case model.RelationFormat_object, model.RelationFormat_file:
		return ko.basicSort(anyenc.TypeString)
	case model.RelationFormat_url, model.RelationFormat_email, model.RelationFormat_phone, model.RelationFormat_emoji:
		return ko.basicSort(anyenc.TypeString)
	case model.RelationFormat_tag, model.RelationFormat_status:
		return ko.tagStatusSort()
	case model.RelationFormat_checkbox:
		return ko.boolSort()
	default:
		return ko.basicSort(anyenc.TypeString)
	}
}

func (ko *KeyOrder) basicSort(valType anyenc.Type) query.Sort {
	if ko.EmptyPlacement == model.BlockContentDataviewSort_Start && ko.Type == model.BlockContentDataviewSort_Desc {
		return ko.emptyPlacementSort(valType)
	} else if ko.EmptyPlacement == model.BlockContentDataviewSort_End && ko.Type == model.BlockContentDataviewSort_Asc {
		return ko.emptyPlacementSort(valType)
	} else {
		return &query.SortField{
			Path:    []string{string(ko.Key)},
			Reverse: ko.Type == model.BlockContentDataviewSort_Desc,
			Field:   string(ko.Key),
		}
	}
}

func (ko *KeyOrder) tagStatusSort() query.Sort {
	if ko.optionsIdToOrderId == nil {
		ko.optionsIdToOrderId = make(map[string]string)
	}
	if len(ko.optionsIdToOrderId) == 0 && ko.objectStore != nil {
		ko.optionsIdToOrderId = optionsToMap(ko.Key, ko.objectStore)
	}
	return tagStatusSort{
		arena:       ko.arena,
		relationKey: string(ko.Key),
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:       ko.EmptyPlacement,
		idToOrderId: ko.optionsIdToOrderId,
	}
}

func (ko *KeyOrder) emptyPlacementSort(valType anyenc.Type) query.Sort {
	return emptyPlacementSort{
		arena:       ko.arena,
		relationKey: string(ko.Key),
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:       ko.EmptyPlacement,
		valType:     valType,
	}
}

func (ko *KeyOrder) dateOnlySort() query.Sort {
	return dateOnlySort{
		arena:       ko.arena,
		relationKey: string(ko.Key),
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
		relationKey:    string(ko.Key),
		reverse:        ko.Type == model.BlockContentDataviewSort_Desc,
		nulls:          ko.EmptyPlacement,
	}
}

func (ko *KeyOrder) boolSort() query.Sort {
	return boolSort{
		arena:       ko.arena,
		relationKey: ko.Key.String(),
		reverse:     ko.Type == model.BlockContentDataviewSort_Desc,
	}
}

func (ko *KeyOrder) tryAdjustEmptyPositions(av domain.Value, bv domain.Value, comp int) int {
	if ko.EmptyPlacement == model.BlockContentDataviewSort_NotSpecified {
		return comp
	}
	aNull := !av.Ok()
	bNull := !bv.Ok()
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

func (ko *KeyOrder) tryCompareStrings(av domain.Value, bv domain.Value) int {
	comp := 0
	aStringVal, aString := av.TryString()
	bStringVal, bString := bv.TryString()
	if ko.isSpecialSortOfEmptyValuesNeed(av, bv, aString, bString) {
		if aStringVal == "" && bStringVal != "" {
			comp = 1
		} else if aStringVal != "" && bStringVal == "" {
			comp = -1
		}
	}
	if aString && bString && comp == 0 && !ko.disableCollator {
		ko.ensureCollator()
		comp = ko.collator.CompareString(aStringVal, bStringVal)
	}
	if aStringVal == "" || bStringVal == "" {
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

func (ko *KeyOrder) isSpecialSortOfEmptyValuesNeed(av domain.Value, bv domain.Value, aString bool, bString bool) bool {
	return (ko.EmptyPlacement != model.BlockContentDataviewSort_NotSpecified) &&
		(aString || !av.Ok()) && (bString || !bv.Ok())
}

func (ko *KeyOrder) tryExtractBool(av domain.Value, bv domain.Value) (domain.Value, domain.Value) {
	if ko.relationFormat == model.RelationFormat_checkbox {
		if !av.Ok() {
			av = domain.Bool(false)
		}
		if !bv.Ok() {
			bv = domain.Bool(false)
		}
	}
	return av, bv
}

func (ko *KeyOrder) tryExtractTag(av domain.Value, bv domain.Value, orderIdsMap map[string]string) (domain.Value, domain.Value) {
	if ko.relationFormat == model.RelationFormat_tag || ko.relationFormat == model.RelationFormat_status {
		av = ko.getOptionValue(av, orderIdsMap)
		bv = ko.getOptionValue(bv, orderIdsMap)
	}
	return av, bv
}

func (ko *KeyOrder) tryExtractDateTime(av domain.Value, bv domain.Value) (domain.Value, domain.Value) {
	if ko.relationFormat == model.RelationFormat_date && !ko.IncludeTime {
		if v, ok := av.TryFloat64(); ok {
			av = domain.Int64(timeutil.CutToDay(time.Unix(int64(v), 0)).Unix())
		}
		if v, ok := bv.TryFloat64(); ok {
			bv = domain.Int64(timeutil.CutToDay(time.Unix(int64(v), 0)).Unix())
		}
	}
	return av, bv
}

func (ko *KeyOrder) tryExtractSnippet(a *domain.Details, b *domain.Details, av domain.Value, bv domain.Value) (domain.Value, domain.Value) {
	av = ko.trySubstituteSnippet(a, av)
	bv = ko.trySubstituteSnippet(b, bv)
	return av, bv
}

func (ko *KeyOrder) trySubstituteSnippet(getter *domain.Details, value domain.Value) domain.Value {
	if ko.Key == bundle.RelationKeyName && getLayout(getter) == model.ObjectType_note {
		_, ok := getter.TryString(bundle.RelationKeyName)
		if !ok {
			return getter.Get(bundle.RelationKeySnippet)
		}
	}
	return value
}

func getLayout(getter *domain.Details) model.ObjectTypeLayout {
	rawLayout := getter.GetInt64(bundle.RelationKeyResolvedLayout)
	return model.ObjectTypeLayout(int32(rawLayout))
}

func (ko *KeyOrder) getOptionValue(value domain.Value, orderIdsMap map[string]string) domain.Value {
	if orderIdsMap == nil {
		orderIdsMap = optionsToMap(ko.Key, ko.objectStore)
	}
	ko.optionsIdToOrderId = orderIdsMap

	res := ""
	for _, objectId := range value.StringList() {
		res += orderIdsMap[objectId]
	}
	return domain.String(res)
}

func newCustomOrder(arena *anyenc.Arena, key domain.RelationKey, idsIndices map[string]int, keyOrd *KeyOrder) customOrder {
	return customOrder{
		arena:        arena,
		Key:          key,
		NeedOrderMap: idsIndices,
		KeyOrd:       keyOrd,
	}
}

type customOrder struct {
	arena        *anyenc.Arena
	Key          domain.RelationKey
	NeedOrderMap map[string]int
	KeyOrd       *KeyOrder

	buf []byte
}

func (co customOrder) AppendKey(k anyenc.Tuple, v *anyenc.Value) anyenc.Tuple {
	defer func() {
		co.arena.Reset()
		co.buf = co.buf[:0]
	}()

	var rawValue string
	if val := v.Get(string(co.Key)); val != nil {
		rawValue = string(val.MarshalTo(co.buf))
	}
	idx, ok := co.NeedOrderMap[rawValue]
	if !ok {
		anystoreSort := co.KeyOrd.AnystoreSort()
		// Push to the end
		k = co.arena.NewNumberInt(len(co.NeedOrderMap)).MarshalTo(k)
		// and add sorting
		return anystoreSort.AppendKey(k, v)
	}
	return co.arena.NewNumberInt(idx).MarshalTo(k)
}

func (co customOrder) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
			Path:  []string{string(co.Key)},
		},
	}
}

func (co customOrder) AnystoreSort() query.Sort {
	return co
}

func (co customOrder) getStringVal(val domain.Value) string {
	defer func() {
		co.arena.Reset()
		co.buf = co.buf[:0]
	}()

	jsonVal := val.ToAnyEnc(co.arena)
	if jsonVal == nil {
		return ""
	}
	return string(jsonVal.MarshalTo(co.buf))
}

func (co customOrder) Compare(a, b *domain.Details, orderIdsMap map[domain.RelationKey]map[string]string) int {

	aID, okA := co.NeedOrderMap[co.getStringVal(a.Get(co.Key))]
	bID, okB := co.NeedOrderMap[co.getStringVal(b.Get(co.Key))]

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

	return co.KeyOrd.Compare(a, b, orderIdsMap)
}
