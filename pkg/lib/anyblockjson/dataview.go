package anyblockjson

// dataview.go maps Content.Dataview to the §6.2 JSON form and back: cleaned
// names, lowerCamel enums, defaults omitted, filter trees with implicit
// top-level AND, and select values as option names.

import (
	"fmt"
	"sort"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// dvFormat resolves a property key's format from the dataview's live
// relationLinks first, then bundle/resolver (§6.2).
func (e *exporter) dvFormat(dv *model.BlockContentDataview, key string) (model.RelationFormat, bool) {
	for _, rl := range dv.RelationLinks {
		if rl != nil && rl.Key == key {
			return rl.Format, true
		}
	}
	return e.resolveFormat(key)
}

func (e *exporter) dataviewToJSON(m *omap, dv *model.BlockContentDataview) error {
	m.set("type", "dataview")
	m.setNonEmpty("objectId", e.compactObjectId(dv.TargetObjectId))
	m.setNonEmpty("isCollection", dv.IsCollection)
	m.setNonEmpty("source", stringsToAny(dv.Source))

	var props []any
	for _, rl := range dv.RelationLinks {
		if rl == nil || rl.Key == "" {
			continue
		}
		pm := &omap{}
		pm.set("key", rl.Key)
		pm.setNonEmpty("format", formatNames.name(rl.Format))
		props = append(props, pm)
	}
	m.setNonEmpty("properties", props)

	var views []any
	for _, v := range dv.Views {
		if v == nil {
			continue
		}
		views = append(views, e.viewToJSON(v, dv))
	}
	m.setNonEmpty("views", views)
	// activeView and the deprecated relations field are dropped (§6.2)
	return nil
}

func (e *exporter) viewToJSON(v *model.BlockContentDataviewView, dv *model.BlockContentDataview) *omap {
	vm := &omap{}
	if !e.opts.OmitIds {
		vm.setNonEmpty("id", e.localId(v.Id))
	}
	if v.Type != model.BlockContentDataviewView_Table {
		vm.setNonEmpty("type", viewTypeNames.name(v.Type))
	}
	vm.setNonEmpty("name", v.Name)
	vm.setNonEmpty("groupBy", v.GroupRelationKey)
	vm.setNonEmpty("coverProperty", v.CoverRelationKey)
	vm.setNonEmpty("endProperty", v.EndRelationKey)
	vm.setNonEmpty("hideIcon", v.HideIcon)
	if v.CardSize != model.BlockContentDataviewView_Small {
		vm.setNonEmpty("cardSize", cardSizeNames.name(v.CardSize))
	}
	vm.setNonEmpty("coverFit", v.CoverFit)
	vm.setNonEmpty("coloredGroups", v.GroupBackgroundColors)
	vm.setNonEmpty("pageSize", v.PageLimit)
	vm.setNonEmpty("defaultTemplateId", e.compactObjectId(v.DefaultTemplateId))
	vm.setNonEmpty("defaultTypeId", e.compactObjectId(v.DefaultObjectTypeId))
	vm.setNonEmpty("wrapContent", v.WrapContent)
	if v.ListSize != model.BlockContentDataviewView_Compact {
		vm.setNonEmpty("listSize", listSizeNames.name(v.ListSize))
	}
	vm.setNonEmpty("alternateRows", v.AlternateRows)

	var sorts []any
	for _, s := range v.Sorts {
		// a sort without a property key is junk and would fail the schema
		if s != nil && s.RelationKey != "" {
			sorts = append(sorts, e.sortToJSON(s, dv))
		}
	}
	vm.setNonEmpty("sorts", sorts)

	var filters []any
	for _, f := range v.Filters {
		if f == nil {
			continue
		}
		if fm := e.filterToJSON(f, dv); fm != nil {
			filters = append(filters, fm)
		}
	}
	vm.setNonEmpty("filters", filters)

	var columns []any
	for _, r := range v.Relations {
		if r != nil && r.Key != "" {
			columns = append(columns, e.viewColumnToJSON(r))
		}
	}
	vm.setNonEmpty("columns", columns)

	if !e.opts.OmitIds {
		vm.setNonEmpty("groups", e.viewGroupsToJSON(v.Id, dv))
		vm.setNonEmpty("objectOrders", e.objectOrdersToJSON(v.Id, dv))
	}
	return vm
}

// viewGroupsToJSON emits the kanban group display order: array order, the
// proto's per-group index derived from it (§6.2).
func (e *exporter) viewGroupsToJSON(viewId string, dv *model.BlockContentDataview) []any {
	var out []any
	for _, groupOrder := range dv.GroupOrders {
		if groupOrder == nil || groupOrder.ViewId != viewId {
			continue
		}
		groups := make([]*model.BlockContentDataviewViewGroup, 0, len(groupOrder.ViewGroups))
		for _, g := range groupOrder.ViewGroups {
			if g != nil {
				groups = append(groups, g)
			}
		}
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Index < groups[j].Index })
		for _, g := range groups {
			gm := &omap{}
			gm.setNonEmpty("id", g.GroupId)
			gm.setNonEmpty("hidden", g.Hidden)
			gm.setNonEmpty("backgroundColor", g.BackgroundColor)
			out = append(out, gm)
		}
	}
	return out
}

func (e *exporter) objectOrdersToJSON(viewId string, dv *model.BlockContentDataview) []any {
	var out []any
	for _, oo := range dv.ObjectOrders {
		if oo == nil || oo.ViewId != viewId {
			continue
		}
		om := &omap{}
		om.setNonEmpty("groupId", oo.GroupId)
		var ids []any
		for _, id := range oo.ObjectIds {
			if id != "" {
				ids = append(ids, e.compactObjectId(id))
			}
		}
		om.setNonEmpty("objectIds", ids)
		out = append(out, om)
	}
	return out
}

func (e *exporter) sortToJSON(s *model.BlockContentDataviewSort, dv *model.BlockContentDataview) *omap {
	sm := &omap{}
	sm.setNonEmpty("property", s.RelationKey)
	if s.Type != model.BlockContentDataviewSort_Asc {
		sm.setNonEmpty("direction", sortDirectionNames.name(s.Type))
	}
	if len(s.CustomOrder) > 0 {
		var order []any
		for _, cv := range s.CustomOrder {
			order = append(order, e.dvValueToJSON(dv, s.RelationKey, cv))
		}
		sm.set("customOrder", order)
	}
	if s.EmptyPlacement != model.BlockContentDataviewSort_NotSpecified {
		sm.setNonEmpty("emptyPlacement", emptyPlacementNames.name(s.EmptyPlacement))
	}
	sm.setNonEmpty("includeTime", s.IncludeTime)
	sm.setNonEmpty("noCollate", s.NoCollate)
	if !e.opts.OmitIds {
		sm.setNonEmpty("id", s.Id)
	}
	// the cached per-node format is dropped; import rehydrates it (§6.2)
	return sm
}

func (e *exporter) filterToJSON(f *model.BlockContentDataviewFilter, dv *model.BlockContentDataview) *omap {
	fm := &omap{}
	if len(f.NestedFilters) > 0 {
		// a proto node with nested filters maps to a group; leaf fields drop
		op := "and"
		if f.Operator == model.BlockContentDataviewFilter_Or {
			op = "or"
		}
		var nested []any
		for _, nf := range f.NestedFilters {
			if nf == nil {
				continue
			}
			if nm := e.filterToJSON(nf, dv); nm != nil {
				nested = append(nested, nm)
			}
		}
		if len(nested) == 0 {
			return nil // a group with no live children is a no-op
		}
		fm.set("operator", op)
		fm.set("filters", nested)
		return fm
	}
	fm.setNonEmpty("property", f.RelationKey)
	if f.Condition != model.BlockContentDataviewFilter_None {
		fm.setNonEmpty("condition", conditionNames.name(f.Condition))
	}
	switch f.Condition {
	case model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists:
		// value is dropped on presence-only conditions (§11)
	default:
		if f.Value != nil {
			fm.setNonEmpty("value", e.dvValueToJSON(dv, f.RelationKey, f.Value))
		}
	}
	if f.QuickOption != model.BlockContentDataviewFilter_ExactDate {
		fm.setNonEmpty("datePreset", datePresetNames.name(f.QuickOption))
	}
	fm.setNonEmpty("includeTime", f.IncludeTime)
	fm.setNonEmpty("nestedProperty", f.RelationProperty)
	if !e.opts.OmitIds {
		fm.setNonEmpty("id", f.Id)
	}
	// a contentless leaf (at most an id) is a no-op node: drop it
	if len(fm.keys) == 0 || (len(fm.keys) == 1 && fm.keys[0] == "id") {
		return nil
	}
	return fm
}

// dvValueToJSON converts a filter value or custom-order entry: option names
// for select properties (§3), compact ids for object-valued ones, verbatim
// otherwise.
func (e *exporter) dvValueToJSON(dv *model.BlockContentDataview, key string, v *types.Value) any {
	format, ok := e.dvFormat(dv, key)
	if ok {
		switch format {
		case model.RelationFormat_status, model.RelationFormat_tag:
			return e.mapValueStrings(v, func(id string) string { return e.optionName(key, id) })
		case model.RelationFormat_object, model.RelationFormat_file:
			return e.mapValueStrings(v, e.compactObjectId)
		}
	}
	return protoValueToJSON(v)
}

// mapValueStrings applies fn to a string value or each element of a string
// list, keeping the single/list shape.
func (e *exporter) mapValueStrings(v *types.Value, fn func(string) string) any {
	if s, ok := v.GetKind().(*types.Value_StringValue); ok {
		return fn(s.StringValue)
	}
	if l, ok := v.GetKind().(*types.Value_ListValue); ok {
		out := make([]any, 0, len(l.ListValue.Values))
		for _, el := range l.ListValue.Values {
			if s := el.GetStringValue(); s != "" {
				out = append(out, fn(s))
			} else {
				out = append(out, protoValueToJSON(el))
			}
		}
		return out
	}
	return protoValueToJSON(v)
}

//
// ---- import ----
//

type jsonDvProperty struct {
	Key    string `json:"key"`
	Format string `json:"format"`
}

type jsonView struct {
	Id                string            `json:"id"`
	Type              string            `json:"type"`
	Name              string            `json:"name"`
	GroupBy           string            `json:"groupBy"`
	CoverProperty     string            `json:"coverProperty"`
	EndProperty       string            `json:"endProperty"`
	HideIcon          bool              `json:"hideIcon"`
	CardSize          string            `json:"cardSize"`
	CoverFit          bool              `json:"coverFit"`
	ColoredGroups     bool              `json:"coloredGroups"`
	PageSize          int32             `json:"pageSize"`
	DefaultTemplateId string            `json:"defaultTemplateId"`
	DefaultTypeId     string            `json:"defaultTypeId"`
	WrapContent       bool              `json:"wrapContent"`
	ListSize          string            `json:"listSize"`
	AlternateRows     bool              `json:"alternateRows"`
	Sorts             []jsonSort        `json:"sorts"`
	Filters           []jsonFilter      `json:"filters"`
	Columns           []jsonViewColumn  `json:"columns"`
	Groups            []jsonViewGroup   `json:"groups"`
	ObjectOrders      []jsonObjectOrder `json:"objectOrders"`
}

type jsonSort struct {
	Property       string `json:"property"`
	Direction      string `json:"direction"`
	CustomOrder    []any  `json:"customOrder"`
	EmptyPlacement string `json:"emptyPlacement"`
	IncludeTime    bool   `json:"includeTime"`
	NoCollate      bool   `json:"noCollate"`
	Id             string `json:"id"`
}

type jsonFilter struct {
	Operator string       `json:"operator"`
	Filters  []jsonFilter `json:"filters"`

	Property       string `json:"property"`
	Condition      string `json:"condition"`
	Value          any    `json:"value"`
	DatePreset     string `json:"datePreset"`
	IncludeTime    bool   `json:"includeTime"`
	NestedProperty string `json:"nestedProperty"`
	Id             string `json:"id"`
}

type jsonViewColumn struct {
	Property    string  `json:"property"`
	Hidden      bool    `json:"hidden"`
	Width       float64 `json:"width"`
	Aggregation string  `json:"aggregation"`
	Align       string  `json:"align"`
}

type jsonViewGroup struct {
	Id              string `json:"id"`
	Hidden          bool   `json:"hidden"`
	BackgroundColor string `json:"backgroundColor"`
}

type jsonObjectOrder struct {
	GroupId   string   `json:"groupId"`
	ObjectIds []string `json:"objectIds"`
}

func (imp *importer) dataviewFromJSON(jb *jsonBlock) (*model.BlockContentDataview, error) {
	dv := &model.BlockContentDataview{
		TargetObjectId: imp.resolveId(jb.ObjectId),
		IsCollection:   jb.IsCollection,
		Source:         jb.Source,
	}
	var props []jsonDvProperty
	if len(jb.Properties) > 0 {
		if err := jsonUnmarshal(jb.Properties, &props); err != nil {
			return nil, fmt.Errorf("dataview properties: %w", err)
		}
	}
	for _, p := range props {
		dv.RelationLinks = append(dv.RelationLinks, &model.RelationLink{
			Key:    p.Key,
			Format: formatNames.value(p.Format),
		})
	}
	for _, jv := range jb.Views {
		viewId := jv.Id
		if viewId == "" {
			viewId = imp.genId()
		}
		view := &model.BlockContentDataviewView{
			Id:                    viewId,
			Type:                  viewTypeNames.value(jv.Type),
			Name:                  jv.Name,
			GroupRelationKey:      jv.GroupBy,
			CoverRelationKey:      jv.CoverProperty,
			EndRelationKey:        jv.EndProperty,
			HideIcon:              jv.HideIcon,
			CardSize:              cardSizeNames.value(jv.CardSize),
			CoverFit:              jv.CoverFit,
			GroupBackgroundColors: jv.ColoredGroups,
			PageLimit:             jv.PageSize,
			DefaultTemplateId:     imp.resolveId(jv.DefaultTemplateId),
			DefaultObjectTypeId:   imp.resolveId(jv.DefaultTypeId),
			WrapContent:           jv.WrapContent,
			ListSize:              listSizeNames.value(jv.ListSize),
			AlternateRows:         jv.AlternateRows,
		}
		for _, js := range jv.Sorts {
			view.Sorts = append(view.Sorts, imp.sortFromJSON(js, dv))
		}
		for _, jf := range jv.Filters {
			view.Filters = append(view.Filters, imp.filterFromJSON(jf, dv))
		}
		for _, jc := range jv.Columns {
			view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{
				Key:       jc.Property,
				IsVisible: !jc.Hidden,
				Width:     int32(jc.Width),
				Formula:   aggregationNames.value(jc.Aggregation),
				Align:     alignNames.value(jc.Align),
			})
		}
		if len(jv.Groups) > 0 {
			groupOrder := &model.BlockContentDataviewGroupOrder{ViewId: viewId}
			for i, jg := range jv.Groups {
				groupOrder.ViewGroups = append(groupOrder.ViewGroups, &model.BlockContentDataviewViewGroup{
					GroupId:         jg.Id,
					Index:           int32(i), // derived from array order (§6.2)
					Hidden:          jg.Hidden,
					BackgroundColor: jg.BackgroundColor,
				})
			}
			dv.GroupOrders = append(dv.GroupOrders, groupOrder)
		}
		for _, jo := range jv.ObjectOrders {
			oo := &model.BlockContentDataviewObjectOrder{ViewId: viewId, GroupId: jo.GroupId}
			for _, id := range jo.ObjectIds {
				oo.ObjectIds = append(oo.ObjectIds, imp.resolveId(id))
			}
			dv.ObjectOrders = append(dv.ObjectOrders, oo)
		}
		dv.Views = append(dv.Views, view)
	}
	return dv, nil
}

// impDvFormat rehydrates the cached per-node format from the dataview's
// properties list and bundle; unresolvable keys get format 0 (§6.2).
func (imp *importer) impDvFormat(dv *model.BlockContentDataview, key string) model.RelationFormat {
	for _, rl := range dv.RelationLinks {
		if rl != nil && rl.Key == key {
			return rl.Format
		}
	}
	if f, ok := imp.resolveFormat(key); ok {
		return f
	}
	return 0
}

func (imp *importer) sortFromJSON(js jsonSort, dv *model.BlockContentDataview) *model.BlockContentDataviewSort {
	s := &model.BlockContentDataviewSort{
		RelationKey:    js.Property,
		Type:           sortDirectionNames.value(js.Direction),
		Format:         imp.impDvFormat(dv, js.Property),
		IncludeTime:    js.IncludeTime,
		Id:             js.Id,
		EmptyPlacement: emptyPlacementNames.value(js.EmptyPlacement),
		NoCollate:      js.NoCollate,
	}
	for _, entry := range js.CustomOrder {
		s.CustomOrder = append(s.CustomOrder, imp.dvValueFromJSON(dv, js.Property, entry))
	}
	return s
}

func (imp *importer) filterFromJSON(jf jsonFilter, dv *model.BlockContentDataview) *model.BlockContentDataviewFilter {
	if jf.Operator != "" {
		f := &model.BlockContentDataviewFilter{
			Operator: model.BlockContentDataviewFilter_And,
		}
		if jf.Operator == "or" {
			f.Operator = model.BlockContentDataviewFilter_Or
		}
		for _, nf := range jf.Filters {
			f.NestedFilters = append(f.NestedFilters, imp.filterFromJSON(nf, dv))
		}
		return f
	}
	f := &model.BlockContentDataviewFilter{
		Id:               jf.Id,
		RelationKey:      jf.Property,
		RelationProperty: jf.NestedProperty,
		Condition:        conditionNames.value(jf.Condition),
		QuickOption:      datePresetNames.value(jf.DatePreset),
		Format:           imp.impDvFormat(dv, jf.Property),
		IncludeTime:      jf.IncludeTime,
	}
	if jf.Value != nil {
		f.Value = imp.dvValueFromJSON(dv, jf.Property, jf.Value)
	}
	return f
}

// dvValueFromJSON reverses dvValueToJSON: option names back to ids where a
// resolver knows them, ref labels back to full object ids, verbatim
// otherwise (§3, §9a).
func (imp *importer) dvValueFromJSON(dv *model.BlockContentDataview, key string, v any) *types.Value {
	format := imp.impDvFormat(dv, key)
	switch format {
	case model.RelationFormat_status, model.RelationFormat_tag:
		return mapJSONStrings(v, func(name string) string { return imp.optionId(key, name) })
	case model.RelationFormat_object, model.RelationFormat_file:
		return mapJSONStrings(v, imp.resolveId)
	}
	return jsonToProtoValue(v)
}

func mapJSONStrings(v any, fn func(string) string) *types.Value {
	switch x := v.(type) {
	case string:
		return &types.Value{Kind: &types.Value_StringValue{StringValue: fn(x)}}
	case []any:
		vals := make([]*types.Value, 0, len(x))
		for _, el := range x {
			if s, ok := el.(string); ok {
				vals = append(vals, &types.Value{Kind: &types.Value_StringValue{StringValue: fn(s)}})
			} else {
				vals = append(vals, jsonToProtoValue(el))
			}
		}
		return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}}}
	}
	return jsonToProtoValue(v)
}

func (imp *importer) optionId(key, name string) string {
	if imp.opts.ResolveOptions != nil {
		if id, ok := imp.opts.ResolveOptions.OptionId(domain.RelationKey(key), name); ok {
			return id
		}
	}
	// unresolved names pass through; creating options is the wiring's job (§3)
	return name
}

func (e *exporter) viewColumnToJSON(r *model.BlockContentDataviewRelation) *omap {
	cm := &omap{}
	cm.set("property", r.Key)
	// hidden is the inverse of proto isVisible; omitted means visible (§6.2)
	cm.setNonEmpty("hidden", !r.IsVisible)
	cm.setNonEmpty("width", r.Width)
	if r.Formula != model.BlockContentDataviewRelation_None {
		cm.setNonEmpty("aggregation", aggregationNames.name(r.Formula))
	}
	if r.Align != model.Block_AlignLeft {
		cm.setNonEmpty("align", alignNames.name(r.Align))
	}
	// deprecated per-column date/time fields are dropped (§6.2)
	return cm
}
