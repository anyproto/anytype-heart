package database

import (
	"bytes"
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
	Compare(a, b *domain.Details) int
	AnystoreSort() query.Sort
	UpdateOrderMap(depDetails []*domain.Details) (updated bool)
}

// ObjectStore interface is used to enrich filters
type ObjectStore interface {
	SpaceId() string
	Query(q Query) (records []Record, err error)
	QueryRaw(filters *Filters, limit int, offset int) ([]Record, error)
	QueryIterate(q Query, proc func(details *domain.Details)) (err error)
	GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error)
	ListRelationOptions(relationKey domain.RelationKey) (options []*model.RelationOption, err error)
}

type setOrder []Order

func (so setOrder) Compare(a, b *domain.Details) int {
	for _, o := range so {
		if comp := o.Compare(a, b); comp != 0 {
			return comp
		}
	}
	return 0
}

func (so setOrder) AnystoreSort() query.Sort {
	if len(so) == 0 {
		return nil
	}
	sorts := make(query.Sorts, 0, len(so))
	for _, o := range so {
		sorts = append(sorts, o.AnystoreSort())
	}
	return sorts
}

func (so setOrder) UpdateOrderMap(depDetails []*domain.Details) (updated bool) {
	for _, o := range so {
		updated = o.UpdateOrderMap(depDetails) || updated
	}
	return updated
}

type keyOrder struct {
	key            domain.RelationKey
	sortType       model.BlockContentDataviewSortType
	emptyPlacement model.BlockContentDataviewSortEmptyType
	relationFormat model.RelationFormat
	includeTime    bool

	orderMap        *OrderMap
	orderMapBufferA []byte
	orderMapBufferB []byte
	arena           *anyenc.Arena

	collatorBuffer  *collate.Buffer
	collator        *collate.Collator
	disableCollator bool
}

func NewKeyOrder(store ObjectStore, arena *anyenc.Arena, collatorBuffer *collate.Buffer, sort SortRequest) Order {
	format, err := store.GetRelationFormatByKey(sort.RelationKey)
	if err != nil {
		format = sort.Format
	}

	var orderMap *OrderMap
	if isObjectFormat(format) {
		orderMap = BuildOrderMap(store, sort.RelationKey, format, collatorBuffer)
	}

	return &keyOrder{
		key:             sort.RelationKey,
		sortType:        sort.Type,
		emptyPlacement:  sort.EmptyPlacement,
		relationFormat:  format,
		orderMap:        orderMap,
		orderMapBufferA: make([]byte, 0),
		orderMapBufferB: make([]byte, 0),
		arena:           arena,
		collatorBuffer:  collatorBuffer,
		disableCollator: sort.NoCollate || sort.RelationKey == bundle.RelationKeyOrderId || sort.RelationKey == bundle.RelationKeySpaceOrder,
	}
}

func (ko *keyOrder) ensureCollator() {
	if ko.collator == nil {
		ko.collator = collate.New(language.Und, collate.IgnoreCase)
		ko.collatorBuffer = &collate.Buffer{}
	}
}

func (ko *keyOrder) Compare(a, b *domain.Details) (comp int) {
	av := a.Get(ko.key)
	bv := b.Get(ko.key)

	switch ko.relationFormat {
	case model.RelationFormat_checkbox:
		return ko.compareBool(av, bv)
	case model.RelationFormat_date:
		return ko.compareDates(av, bv)
	case model.RelationFormat_number:
		return ko.compareNumbers(av, bv)
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		av = ko.trySubstituteSnippet(a, av)
		bv = ko.trySubstituteSnippet(b, bv)
		return ko.compareStrings(av, bv)
	case model.RelationFormat_object, model.RelationFormat_file, model.RelationFormat_tag, model.RelationFormat_status:
		return ko.compareObjectValues(av, bv)
	default:
		return ko.compareStrings(av, bv)
	}
}

func (ko *keyOrder) AnystoreSort() query.Sort {
	switch ko.relationFormat {
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		if ko.disableCollator {
			return ko.basicSort(anyenc.TypeString)
		}
		return ko.textSort()
	case model.RelationFormat_number:
		return ko.basicSort(anyenc.TypeNumber)
	case model.RelationFormat_date:
		if ko.includeTime {
			return ko.basicSort(anyenc.TypeNumber)
		} else {
			return ko.dateOnlySort()
		}
	case model.RelationFormat_url, model.RelationFormat_email, model.RelationFormat_phone, model.RelationFormat_emoji:
		return ko.basicSort(anyenc.TypeString)
	case model.RelationFormat_tag, model.RelationFormat_status, model.RelationFormat_object, model.RelationFormat_file:
		return ko.objectSort()
	case model.RelationFormat_checkbox:
		return ko.boolSort()
	default:
		return ko.basicSort(anyenc.TypeString)
	}
}

func (ko *keyOrder) UpdateOrderMap(depDetails []*domain.Details) (updated bool) {
	return ko.orderMap.Update(depDetails)
}

func (ko *keyOrder) basicSort(valType anyenc.Type) query.Sort {
	if ko.emptyPlacement == model.BlockContentDataviewSort_Start && ko.sortType == model.BlockContentDataviewSort_Desc {
		return ko.emptyPlacementSort(valType)
	} else if ko.emptyPlacement == model.BlockContentDataviewSort_End && ko.sortType == model.BlockContentDataviewSort_Asc {
		return ko.emptyPlacementSort(valType)
	} else {
		return &query.SortField{
			Path:    []string{string(ko.key)},
			Reverse: ko.sortType == model.BlockContentDataviewSort_Desc,
			Field:   string(ko.key),
		}
	}
}

func (ko *keyOrder) objectSort() query.Sort {
	return objectSort{
		arena:       ko.arena,
		relationKey: string(ko.key),
		reverse:     ko.sortType == model.BlockContentDataviewSort_Desc,
		nulls:       ko.emptyPlacement,
		orders:      ko.orderMap,
		keyBuffer:   make([]byte, 0),
	}
}

func (ko *keyOrder) emptyPlacementSort(valType anyenc.Type) query.Sort {
	return emptyPlacementSort{
		arena:       ko.arena,
		relationKey: string(ko.key),
		reverse:     ko.sortType == model.BlockContentDataviewSort_Desc,
		nulls:       ko.emptyPlacement,
		valType:     valType,
	}
}

func (ko *keyOrder) dateOnlySort() query.Sort {
	return dateOnlySort{
		arena:       ko.arena,
		relationKey: string(ko.key),
		reverse:     ko.sortType == model.BlockContentDataviewSort_Desc,
		nulls:       ko.emptyPlacement,
	}
}

func (ko *keyOrder) textSort() query.Sort {
	ko.ensureCollator()
	return textSort{
		arena:          ko.arena,
		collator:       ko.collator,
		collatorBuffer: ko.collatorBuffer,
		relationKey:    string(ko.key),
		reverse:        ko.sortType == model.BlockContentDataviewSort_Desc,
		nulls:          ko.emptyPlacement,
	}
}

func (ko *keyOrder) boolSort() query.Sort {
	return boolSort{
		arena:       ko.arena,
		relationKey: ko.key.String(),
		reverse:     ko.sortType == model.BlockContentDataviewSort_Desc,
	}
}

func (ko *keyOrder) compareStrings(av domain.Value, bv domain.Value) int {
	aStr, okA := av.TryString()
	bStr, okB := bv.TryString()

	aEmpty := !okA || aStr == ""
	bEmpty := !okB || bStr == ""

	comp, ok := ko.tryCompareEmptyValues(aEmpty, bEmpty)
	if ok {
		return comp
	}

	if ko.disableCollator {
		comp = av.Compare(bv)
	} else {
		ko.ensureCollator()
		comp = ko.collator.CompareString(aStr, bStr)
	}

	if ko.sortType == model.BlockContentDataviewSort_Desc {
		comp = -comp
	}
	return comp
}

func (ko *keyOrder) compareBool(av domain.Value, bv domain.Value) int {
	if !av.Ok() {
		av = domain.Bool(false)
	}
	if !bv.Ok() {
		bv = domain.Bool(false)
	}
	comp := av.Compare(bv)
	if ko.sortType == model.BlockContentDataviewSort_Desc {
		comp = -comp
	}
	return comp
}

func (ko *keyOrder) compareObjectValues(av domain.Value, bv domain.Value) int {
	aList, okA := av.TryWrapToStringList()
	bList, okB := bv.TryWrapToStringList()

	aEmpty := !okA || len(aList) == 0
	bEmpty := !okB || len(bList) == 0

	comp, ok := ko.tryCompareEmptyValues(aEmpty, bEmpty)
	if ok {
		return comp
	}

	ko.orderMapBufferA = ko.orderMap.BuildOrder(ko.orderMapBufferA, aList...)
	ko.orderMapBufferB = ko.orderMap.BuildOrder(ko.orderMapBufferB, bList...)
	comp = bytes.Compare(ko.orderMapBufferA, ko.orderMapBufferB)

	// if we cannot order by orderIds or names, let's try order by number of objects in detail value
	if comp == 0 {
		if len(aList) < len(bList) {
			comp = -1
		} else if len(aList) > len(bList) {
			comp = 1
		}
	}

	if ko.sortType == model.BlockContentDataviewSort_Desc {
		comp = -comp
	}
	return comp
}

func (ko *keyOrder) compareDates(av domain.Value, bv domain.Value) int {
	if !ko.includeTime {
		if v, ok := av.TryInt64(); ok {
			av = domain.Int64(timeutil.CutToDay(time.Unix(v, 0)).Unix())
		}
		if v, ok := bv.TryInt64(); ok {
			bv = domain.Int64(timeutil.CutToDay(time.Unix(v, 0)).Unix())
		}
	}
	return ko.compareNumbers(av, bv)
}

func (ko *keyOrder) compareNumbers(av domain.Value, bv domain.Value) int {
	_, okA := av.TryInt64()
	_, okB := bv.TryInt64()

	comp, ok := ko.tryCompareEmptyValues(!okA, !okB)
	if ok {
		return comp
	}

	comp = av.Compare(bv)
	if ko.sortType == model.BlockContentDataviewSort_Desc {
		comp = -comp
	}
	return comp
}

func (ko *keyOrder) tryCompareEmptyValues(aIsEmpty, bIsEmpty bool) (int, bool) {
	if aIsEmpty && bIsEmpty {
		return 0, true
	}

	if ko.emptyPlacement == model.BlockContentDataviewSort_NotSpecified {
		return 0, false
	}

	if aIsEmpty {
		if ko.emptyPlacement == model.BlockContentDataviewSort_Start {
			return -1, true // A=null < B
		} else {
			return 1, true //  B < A=null
		}
	}

	if bIsEmpty {
		if ko.emptyPlacement == model.BlockContentDataviewSort_Start {
			return 1, true //  B=null < A
		} else {
			return -1, true // A < B=null
		}
	}

	return 0, false
}

func (ko *keyOrder) trySubstituteSnippet(details *domain.Details, value domain.Value) domain.Value {
	rawLayout := details.GetInt64(bundle.RelationKeyResolvedLayout)
	if ko.key == bundle.RelationKeyName && model.ObjectTypeLayout(rawLayout) == model.ObjectType_note { // nolint:gosec
		if _, ok := details.TryString(bundle.RelationKeyName); !ok {
			return details.Get(bundle.RelationKeySnippet)
		}
	}
	return value
}

func isObjectFormat(format model.RelationFormat) bool {
	return format == model.RelationFormat_object || format == model.RelationFormat_file ||
		format == model.RelationFormat_tag || format == model.RelationFormat_status
}

func newCustomOrder(arena *anyenc.Arena, key domain.RelationKey, idsIndices map[string]int, keyOrd *keyOrder) customOrder {
	return customOrder{
		arena:        arena,
		key:          key,
		needOrderMap: idsIndices,
		keyOrd:       keyOrd,
	}
}

type customOrder struct {
	arena        *anyenc.Arena
	key          domain.RelationKey
	needOrderMap map[string]int
	keyOrd       *keyOrder

	buf []byte
}

func (co customOrder) AppendKey(k anyenc.Tuple, v *anyenc.Value) anyenc.Tuple {
	defer func() {
		co.arena.Reset()
		co.buf = co.buf[:0]
	}()

	var rawValue string
	if val := v.Get(string(co.key)); val != nil {
		rawValue = string(val.MarshalTo(co.buf))
	}
	idx, ok := co.needOrderMap[rawValue]
	if !ok {
		anystoreSort := co.keyOrd.AnystoreSort()
		// Push to the end
		k = co.arena.NewNumberInt(len(co.needOrderMap)).MarshalTo(k)
		// and add sorting
		return anystoreSort.AppendKey(k, v)
	}
	return co.arena.NewNumberInt(idx).MarshalTo(k)
}

func (co customOrder) Fields() []query.SortField {
	return []query.SortField{
		{
			Field: "",
			Path:  []string{string(co.key)},
		},
	}
}

func (co customOrder) AnystoreSort() query.Sort {
	return co
}

func (co customOrder) UpdateOrderMap(depDetails []*domain.Details) bool {
	return co.keyOrd.UpdateOrderMap(depDetails)
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

func (co customOrder) Compare(a, b *domain.Details) int {

	aID, okA := co.needOrderMap[co.getStringVal(a.Get(co.key))]
	bID, okB := co.needOrderMap[co.getStringVal(b.Get(co.key))]

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

	return co.keyOrd.Compare(a, b)
}
