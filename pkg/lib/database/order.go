package database

import (
	"strings"

	"github.com/gogo/protobuf/types"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	time_util "github.com/anyproto/anytype-heart/util/time"
)

type Order interface {
	Compare(a, b Getter) int
	String() string
}

// ObjectStore interface is used to enrich filters
type ObjectStore interface {
	Query(q Query) (records []Record, total int, err error)
	QueryRaw(filters *Filters, limit int, offset int) ([]Record, error)
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

func (ko *KeyOrder) Compare(a, b Getter) int {
	av := a.Get(ko.Key)
	bv := b.Get(ko.Key)

	av, bv = ko.handleNoteLayout(a, b, av, bv)
	av, bv = ko.handleDateWithTime(av, bv)
	av, bv = ko.handleTag(av, bv)

	comp := ko.tryCompareStrings(av, bv)
	if comp == 0 {
		comp = av.Compare(bv)
	}
	if ko.Type == model.BlockContentDataviewSort_Desc {
		comp = -comp
	}
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
		if ko.Type == model.BlockContentDataviewSort_Desc && ko.EmptyPlacement == model.BlockContentDataviewSort_End {
			comp = -comp
		}
		if ko.Type == model.BlockContentDataviewSort_Asc && ko.EmptyPlacement == model.BlockContentDataviewSort_Start {
			comp = -comp
		}
	}
	if aString && bString && comp == 0 {
		ko.ensureComparator()
		comp = ko.comparator.CompareString(av.GetStringValue(), bv.GetStringValue())
	}
	return comp
}

func (ko *KeyOrder) isSpecialSortOfEmptyValuesNeed(av *types.Value, bv *types.Value, aString bool, bString bool) bool {
	return (ko.EmptyPlacement != model.BlockContentDataviewSort_NotSpecified) &&
		(aString || av == nil) && (bString || bv == nil)
}

func (ko *KeyOrder) handleTag(av *types.Value, bv *types.Value) (*types.Value, *types.Value) {
	if ko.RelationFormat == model.RelationFormat_tag || ko.RelationFormat == model.RelationFormat_status {
		av = ko.GetOptionValue(av)
		bv = ko.GetOptionValue(bv)
	}
	return av, bv
}

func (ko *KeyOrder) handleDateWithTime(av *types.Value, bv *types.Value) (*types.Value, *types.Value) {
	if ko.RelationFormat == model.RelationFormat_date && !ko.IncludeTime {
		av = time_util.CutValueToDay(av)
		bv = time_util.CutValueToDay(bv)
	}
	return av, bv
}

func (ko *KeyOrder) handleNoteLayout(a Getter, b Getter, av *types.Value, bv *types.Value) (*types.Value, *types.Value) {
	av = ko.trySubstituteSnippet(a, av)
	bv = ko.trySubstituteSnippet(b, bv)
	return av, bv
}

func (ko *KeyOrder) trySubstituteSnippet(getter Getter, value *types.Value) *types.Value {
	if ko.Key == bundle.RelationKeyName.String() && getLayout(getter) == model.ObjectType_note {
		value = getter.Get(bundle.RelationKeyName.String())
		if value == nil {
			value = getter.Get(bundle.RelationKeySnippet.String())
		}
	}
	return value
}

func getLayout(getter Getter) model.ObjectTypeLayout {
	rawLayout := getter.Get(bundle.RelationKeyLayout.String()).GetNumberValue()
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

func NewCustomOrder(key string, needOrder []*types.Value, keyOrd KeyOrder) CustomOrder {
	m := make(map[string]int, 0)
	for id, v := range needOrder {
		m[v.String()] = id
	}

	return CustomOrder{
		Key:          key,
		NeedOrderMap: m,
		KeyOrd:       keyOrd,
	}
}

type CustomOrder struct {
	Key          string
	NeedOrderMap map[string]int
	KeyOrd       KeyOrder
}

func (co CustomOrder) Compare(a, b Getter) int {
	aID, okA := co.NeedOrderMap[a.Get(co.Key).String()]
	bID, okB := co.NeedOrderMap[b.Get(co.Key).String()]

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
