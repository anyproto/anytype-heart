package filter

import (
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	time_util "github.com/anyproto/anytype-heart/util/time"
)

var log = logging.Logger("anytype-order")

type Order interface {
	Compare(a, b Getter) int
	String() string
}

type OptionsGetter interface {
	GetAggregatedOptions(relationKey string) (options []*model.RelationOption, err error)
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
	Key            string
	Type           model.BlockContentDataviewSortType
	EmptyLast      bool // consider empty strings as the last, not first
	RelationFormat model.RelationFormat
	IncludeTime    bool
	Store          OptionsGetter
	Options        map[string]string
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
	if ko.EmptyLast && (aString || av == nil) && (bString || bv == nil) {
		if av.GetStringValue() == "" && bv.GetStringValue() != "" {
			comp = 1
		} else if av.GetStringValue() != "" && bv.GetStringValue() == "" {
			comp = -1
		}
	}
	if aString && bString && comp == 0 {
		comp = strings.Compare(strings.ToLower(av.GetStringValue()), strings.ToLower(bv.GetStringValue()))
	}
	return comp
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
		ko.Options = optionsToMap(ko.Key, ko.Store)
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
