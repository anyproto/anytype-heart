package v2service

// widgets.go implements the sidebar widget surface (APIV2_WIDGETS.md):
// GET / POST /v2/spaces/{space_id}/widgets and PATCH / DELETE
// /v2/spaces/{space_id}/widgets/{widget_id}.
//
// The heart validates almost nothing about a widget — any layout, any limit,
// any target, duplicates included — and the desktop's rules live in its menu
// code. This file is where those rules are stated for the API, so a widget
// written here is one the desktop would have let a user make: only real,
// live objects as targets (no relations, no templates, no archived objects,
// no built-in listings), one widget per target per root, a layout the target
// can render, a limit from the desktop's own pick-list, and a view only
// where the target has views. Where the desktop silently renders a fallback
// (an off-list layout or limit), this file stores the fallback and says so
// in a warning, so the stored widget and the rendered one agree.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anyproto/any-block/codec/anyblockjson"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// widgetLayoutNames is the wire vocabulary of the widget layout, the
// format's own spellings (any-block's index.json widget). widgetvocab_test.go
// pins each name against the codec, so the two cannot drift.
var widgetLayoutNames = map[model.BlockContentWidgetLayout]string{
	model.BlockContentWidget_Link:        v2model.WidgetLayoutLink,
	model.BlockContentWidget_Tree:        v2model.WidgetLayoutTree,
	model.BlockContentWidget_List:        v2model.WidgetLayoutList,
	model.BlockContentWidget_CompactList: v2model.WidgetLayoutCompactList,
	model.BlockContentWidget_View:        v2model.WidgetLayoutView,
}

func widgetLayoutName(layout model.BlockContentWidgetLayout) string {
	if name, ok := widgetLayoutNames[layout]; ok {
		return name
	}
	return v2model.WidgetLayoutLink
}

func parseWidgetLayout(name string) (model.BlockContentWidgetLayout, bool) {
	for layout, spelled := range widgetLayoutNames {
		if spelled == name {
			return layout, true
		}
	}
	return 0, false
}

// widgetTargetKind is what the desktop's layout menu keys on: the target's
// object layout, collapsed to the classes that get different layout sets
// (U.Menu.getWidgetLayoutOptions in anytype-ts) or a different default at
// creation (Action.createWidgetFromObjectIn).
type widgetTargetKind int

const (
	// widgetTargetPage: a page-like object (basic, profile, todo, note,
	// bookmark) — tree or link, created as tree.
	widgetTargetPage widgetTargetKind = iota
	// widgetTargetTreeable: every other layout the desktop neither treats
	// as a list nor excludes from tree (tag, the option list, the legacy
	// chat page, notifications …) — tree or link, created as link.
	widgetTargetTreeable
	// widgetTargetList: a query, collection or type — it has views, so
	// view, compact_list, list or link.
	widgetTargetList
	// widgetTargetLinkOnly: files, participants, dates, chats, discussions,
	// properties, options, dashboards, spaces — link, nothing to expand.
	widgetTargetLinkOnly
	// widgetTargetListing: an existing favorite / recent / recent_open
	// listing widget — compact_list, list or tree.
	widgetTargetListing
	// widgetTargetBin: an existing _bin widget — link as well.
	widgetTargetBin
)

// widgetLayoutsFor is the desktop's layout menu per target kind, in menu
// order, and the layout a create takes when the body names none. The first
// entry of the set is what the desktop renders when the stored layout is
// not in it; the default is the desktop's own creation choice, which for a
// treeable non-page object is link even though its menu leads with tree.
func widgetLayoutsFor(kind widgetTargetKind) (allowed []model.BlockContentWidgetLayout, def model.BlockContentWidgetLayout) {
	switch kind {
	case widgetTargetPage:
		return []model.BlockContentWidgetLayout{model.BlockContentWidget_Tree, model.BlockContentWidget_Link}, model.BlockContentWidget_Tree
	case widgetTargetTreeable:
		return []model.BlockContentWidgetLayout{model.BlockContentWidget_Tree, model.BlockContentWidget_Link}, model.BlockContentWidget_Link
	case widgetTargetList:
		return []model.BlockContentWidgetLayout{model.BlockContentWidget_View, model.BlockContentWidget_CompactList, model.BlockContentWidget_List, model.BlockContentWidget_Link}, model.BlockContentWidget_View
	case widgetTargetListing:
		return []model.BlockContentWidgetLayout{model.BlockContentWidget_CompactList, model.BlockContentWidget_List, model.BlockContentWidget_Tree}, model.BlockContentWidget_CompactList
	case widgetTargetBin:
		return []model.BlockContentWidgetLayout{model.BlockContentWidget_Link, model.BlockContentWidget_CompactList, model.BlockContentWidget_List, model.BlockContentWidget_Tree}, model.BlockContentWidget_Link
	}
	return []model.BlockContentWidgetLayout{model.BlockContentWidget_Link}, model.BlockContentWidget_Link
}

// widgetLimitsFor is the desktop's "Number of objects" pick-list per layout
// (U.Menu.getWidgetLimitOptions): the first entry is the default. A link
// widget shows no entries and takes no limit.
func widgetLimitsFor(layout model.BlockContentWidgetLayout) []int {
	if layout == model.BlockContentWidget_List {
		return []int{4, 6, 8, 30, 50}
	}
	return []int{6, 10, 14, 30, 50}
}

// widgetKindOfLayout maps a target's object layout to its widget kind, the
// desktop's classification: set layouts are lists; file, system,
// participant, date, chat and discussion layouts lose tree; the page
// layouts are created as tree; whatever is left keeps tree in its menu but
// is created as link.
func widgetKindOfLayout(layout model.ObjectTypeLayout) widgetTargetKind {
	switch layout {
	case model.ObjectType_basic, model.ObjectType_profile, model.ObjectType_todo, model.ObjectType_note, model.ObjectType_bookmark:
		return widgetTargetPage
	case model.ObjectType_set, model.ObjectType_collection, model.ObjectType_objectType:
		return widgetTargetList
	case model.ObjectType_file, model.ObjectType_image, model.ObjectType_audio, model.ObjectType_video, model.ObjectType_pdf,
		model.ObjectType_relation, model.ObjectType_relationOption, model.ObjectType_dashboard, model.ObjectType_space, model.ObjectType_spaceView,
		model.ObjectType_participant, model.ObjectType_date, model.ObjectType_chatDerived, model.ObjectType_discussion:
		return widgetTargetLinkOnly
	}
	return widgetTargetTreeable
}

// widgetKindOfListing maps a stored built-in listing id to its widget kind.
func widgetKindOfListing(stored string) widgetTargetKind {
	switch stored {
	case widget.DefaultWidgetFavorite, widget.DefaultWidgetRecentlyEdited, widget.DefaultWidgetRecentlyOpened:
		return widgetTargetListing
	case widget.DefaultWidgetBin:
		return widgetTargetBin
	}
	// allObjects and chat are stored data the desktop no longer renders;
	// set and collection were migrated to type targets long ago
	return widgetTargetLinkOnly
}

// widgetTarget is what a create or update knows about the object a widget
// points at.
type widgetTarget struct {
	stored string // the id as the link block holds it
	kind   widgetTargetKind
}

// parseWidgetScope reads a scope member; "" is not a scope.
func parseWidgetScope(scope, path string) (apicore.WidgetScope, error) {
	switch scope {
	case v2model.WidgetScopeSpace:
		return apicore.WidgetScopeSpace, nil
	case v2model.WidgetScopePersonal:
		return apicore.WidgetScopePersonal, nil
	case "":
		return "", v2model.ValidationFailed("scope is required",
			v2model.Issue{Path: path, Message: "space is the sidebar every member sees, written by the owner and admins; personal is this account's own sidebar in the space, written by any member who can write"})
	}
	return "", v2model.ValidationFailed(fmt.Sprintf("unknown scope %q", scope),
		v2model.Issue{Path: path, Message: "scope is space or personal"})
}

// widgetScopesOf lists the roots a scope filter selects: one, or both in
// served order when the filter is empty.
func widgetScopesOf(filter string) ([]apicore.WidgetScope, error) {
	if filter == "" {
		return []apicore.WidgetScope{apicore.WidgetScopeSpace, apicore.WidgetScopePersonal}, nil
	}
	scope, err := parseWidgetScope(filter, "scope")
	if err != nil {
		return nil, err
	}
	return []apicore.WidgetScope{scope}, nil
}

// widgetRow serves one entry: the stored listing id in the format's
// underscore spelling, no limit on a link widget. The stored layout must be
// one this vocabulary knows; servedWidgetRow is the entry point that
// handles one it does not.
func widgetRow(entry apicore.WidgetEntry) v2model.WidgetRow {
	row := v2model.WidgetRow{
		Id:     entry.Id,
		Scope:  string(entry.Scope),
		Target: anyblockjson.FormatWidgetTarget(entry.Target),
		Layout: widgetLayoutName(entry.Layout),
		ViewId: entry.ViewId,
	}
	if row.Layout != v2model.WidgetLayoutLink {
		row.Limit = int(entry.Limit)
	}
	return row
}

// servedWidgetRow is widgetRow for an entry as heart stores it: a layout enum
// this vocabulary does not know (heart's RPC takes any number) is served as
// the target's render fallback — what the desktop shows — with a warning that
// names the widget, so a reader is never told link for a widget that shows
// a view (C11: unrepresentable content warns, it does not disappear).
func (s *Service) servedWidgetRow(spaceId string, entry apicore.WidgetEntry, warnings *[]v2model.Issue) v2model.WidgetRow {
	if _, known := widgetLayoutNames[entry.Layout]; !known {
		allowed, _ := widgetLayoutsFor(s.existingWidgetTarget(spaceId, entry).kind)
		stored := entry.Layout
		entry.Layout = allowed[0]
		*warnings = append(*warnings, v2model.Issue{Path: "widget_id", Message: fmt.Sprintf("widget %s stores layout %d, which this API does not name; shown as %s, the same fallback the app renders. A layout or limit update stores that fallback", entry.Id, stored, widgetLayoutName(allowed[0]))})
	}
	return widgetRow(entry)
}

// ensureWidgets is the fail-closed guard for a nil port.
func (s *Service) ensureWidgets() error {
	if s.widgets == nil {
		return v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError, "sidebar widgets are not available on this server")
	}
	return nil
}

// ListWidgets implements GET /v2/spaces/{space_id}/widgets: both roots in
// sidebar order, the space root first, narrowed by scope.
func (s *Service) ListWidgets(ctx context.Context, spaceId, scopeFilter string, offset, limit int) ([]v2model.WidgetRow, int, bool, []v2model.Issue, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, nil, err
	}
	if err := s.ensureWidgets(); err != nil {
		return nil, 0, false, nil, err
	}
	scopes, err := widgetScopesOf(scopeFilter)
	if err != nil {
		return nil, 0, false, nil, err
	}
	var rows []v2model.WidgetRow
	var warnings []v2model.Issue
	for _, scope := range scopes {
		entries, err := s.widgets.ListWidgets(ctx, spaceId, scope)
		if err != nil {
			return nil, 0, false, nil, widgetPortError("list widgets", err)
		}
		for _, entry := range entries {
			rows = append(rows, s.servedWidgetRow(spaceId, entry, &warnings))
		}
	}
	page, hasMore := pagination.Paginate(rows, offset, limit)
	return page, len(rows), hasMore, warnings, nil
}

// widgetPortError classifies an adapter failure: the editor's space
// configuration lock (a member who is not the owner or an admin touching the
// space root) is a 403, an unknown widget id a 404, the rest a 500.
func widgetPortError(op string, err error) error {
	switch {
	case errors.Is(err, apicore.ErrWidgetNotFound):
		return v2model.NotFound(fmt.Sprintf("%s: %s", op, err.Error()))
	case errors.Is(err, apicore.ErrBinPlacement):
		return v2model.ValidationFailed(fmt.Sprintf("%s: %s", op, err.Error()),
			v2model.Issue{Path: "/after", Message: "the app lands every drop near the bin before it; send before with the same widget"})
	case errors.Is(err, apicore.ErrWidgetRetargeted):
		return v2model.NewError(http.StatusConflict, v2model.CodeEtagMismatch,
			fmt.Sprintf("%s: the widget's target changed while the request was in flight; re-read the sidebar and retry", op))
	case errors.Is(err, apicore.ErrWidgetExists):
		// the service's own duplicate check ran outside the root's lock; a
		// concurrent create for the same target reaches this one
		return v2model.ValidationFailed(fmt.Sprintf("%s: %s", op, err.Error()),
			v2model.Issue{Path: "/target", Message: "one widget per target per scope, the way the app toggles them; update the existing widget instead"})
	case errors.Is(err, restriction.ErrRestricted):
		return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
			fmt.Sprintf("%s: the space sidebar is space configuration, changed only by the space owner or an admin; scope personal is this account's own", op))
	}
	return fmt.Errorf("%s: %w", op, err)
}

// ensureWidgetWritable is the per-scope permission gate, asked before any
// validation so a member who cannot write the root learns that first.
func (s *Service) ensureWidgetWritable(ctx context.Context, spaceId string, scope apicore.WidgetScope) error {
	ok, err := s.widgets.CanEditWidgets(ctx, spaceId, scope)
	if err != nil {
		return fmt.Errorf("check widget permission: %w", err)
	}
	if ok {
		return nil
	}
	if scope == apicore.WidgetScopeSpace {
		return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
			"the space sidebar is changed only by the space owner or an admin; scope personal is this account's own sidebar and takes the same body")
	}
	return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
		"this account cannot write in the space, so it keeps no sidebar of its own there")
}

// resolveWidgetTarget answers what a create may point a widget at.
func (s *Service) resolveWidgetTarget(spaceId, target string) (widgetTarget, error) {
	if target == "" {
		return widgetTarget{}, v2model.ValidationFailed("target is required",
			v2model.Issue{Path: "/target", Message: "the id of the object the widget points at"}.
				Hintf("find one with %s", v2model.RefSearchSpace(spaceId)))
	}
	if anyblockjson.IsReservedWidgetTarget(target) || widget.IsPredefinedWidgetTargetId(target) {
		return widgetTarget{}, v2model.ValidationFailed(
			fmt.Sprintf("built-in listing %q cannot be added as a widget", target),
			v2model.Issue{Path: "/target", Message: "the app no longer offers the built-in listings as widgets; the ones a sidebar already holds can still be updated and deleted"}.
				Hintf("see what the sidebar holds with %s", v2model.RefListWidgets(spaceId)))
	}
	if strings.HasPrefix(target, domain.PersonalWidgetsPrefix) {
		return widgetTarget{}, v2model.ValidationFailed(
			fmt.Sprintf("%q is a sidebar, not an object", target),
			v2model.Issue{Path: "/target", Message: "the personal sidebar cannot be a widget target"})
	}
	// heart's own ids for participants and dates start with an underscore
	// too, so the prefix alone says nothing: the store decides, and only a
	// prefixed id the store does not know reads as a misspelled listing.
	// The lookup is the virtual-aware one: a date has no persisted row, its
	// details are synthesized from the id
	details, found := s.widgetTargetDetails(spaceId, target)
	if !found {
		if anyblockjson.IsPlatformId(target) && !strings.HasPrefix(target, domain.ParticipantPrefix) && !strings.HasPrefix(target, addr.DatePrefix) {
			return widgetTarget{}, v2model.ValidationFailed(
				fmt.Sprintf("unknown built-in listing %q", target),
				v2model.Issue{Path: "/target", Message: "no object with this id; the built-in listings are " + strings.Join(anyblockjson.ReservedWidgetTargets(), ", ") + ", and none of them can be added"})
		}
		return widgetTarget{}, v2model.ValidationFailed(
			fmt.Sprintf("no object %q in the space", target),
			v2model.Issue{Path: "/target", Message: "the target is an object id in this space"}.
				Hintf("find one with %s", v2model.RefSearchSpace(spaceId)))
	}
	switch {
	case details.GetBool(bundle.RelationKeyIsDeleted):
		return widgetTarget{}, v2model.ValidationFailed(
			fmt.Sprintf("object %q is deleted", target),
			v2model.Issue{Path: "/target", Message: "a deleted object cannot be a widget target"})
	case details.GetBool(bundle.RelationKeyIsArchived):
		return widgetTarget{}, v2model.ValidationFailed(
			fmt.Sprintf("object %q is in the bin", target),
			v2model.Issue{Path: "/target", Message: "an archived object cannot be a widget target; restore it first"})
	}
	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
	if layout == model.ObjectType_relation {
		return widgetTarget{}, v2model.ValidationFailed(
			fmt.Sprintf("object %q is a property", target),
			v2model.Issue{Path: "/target", Message: "properties cannot be widget targets; types, queries, collections and objects can"})
	}
	if typeDetails, err := s.store.SpaceIndex(spaceId).GetDetails(details.GetString(bundle.RelationKeyType)); err == nil &&
		typeDetails.GetString(bundle.RelationKeyUniqueKey) == bundle.TypeKeyTemplate.URL() {
		return widgetTarget{}, v2model.ValidationFailed(
			fmt.Sprintf("object %q is a template", target),
			v2model.Issue{Path: "/target", Message: "templates cannot be widget targets"})
	}
	return widgetTarget{stored: target, kind: widgetKindOfLayout(layout)}, nil
}

// existingWidgetTarget describes the target of a widget that already exists,
// for the layout and limit rules an update applies. It refuses nothing: a
// target that no longer resolves is treated as link-only, and the sidebar
// drops the widget on its own the next time it loads.
func (s *Service) existingWidgetTarget(spaceId string, entry apicore.WidgetEntry) widgetTarget {
	if widget.IsPredefinedWidgetTargetId(entry.Target) {
		return widgetTarget{stored: entry.Target, kind: widgetKindOfListing(entry.Target)}
	}
	details, found := s.widgetTargetDetails(spaceId, entry.Target)
	if !found {
		return widgetTarget{stored: entry.Target, kind: widgetTargetLinkOnly}
	}
	return widgetTarget{stored: entry.Target, kind: widgetKindOfLayout(model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout)))}
}

// widgetTargetDetails reads a target's details through the store's
// virtual-aware lookup (QueryByIds synthesizes a date's details from its
// id; a persisted row is read as usual). false when the space holds no
// such object.
func (s *Service) widgetTargetDetails(spaceId, id string) (*domain.Details, bool) {
	records, err := s.store.SpaceIndex(spaceId).QueryByIds([]string{id})
	if err != nil || len(records) == 0 || records[0].Details == nil || records[0].Details.GetString(bundle.RelationKeyId) != id {
		return nil, false
	}
	return records[0].Details, true
}

// normalizeWidgetLayout is the desktop's render fallback, stored: a layout
// the target cannot take becomes the first the target can, with a warning
// that names both. No layout at all takes the desktop's creation default.
func normalizeWidgetLayout(target widgetTarget, requested string, warnings *[]v2model.Issue) (model.BlockContentWidgetLayout, error) {
	allowed, def := widgetLayoutsFor(target.kind)
	if requested == "" {
		return def, nil
	}
	layout, ok := parseWidgetLayout(requested)
	if !ok {
		return 0, v2model.ValidationFailed(fmt.Sprintf("unknown layout %q", requested),
			v2model.Issue{Path: "/layout", Message: "layout is one of link, tree, list, compact_list, view"})
	}
	names := make([]string, len(allowed))
	for i, l := range allowed {
		names[i] = widgetLayoutName(l)
		if l == layout {
			return layout, nil
		}
	}
	*warnings = append(*warnings, v2model.Issue{Path: "/layout",
		Message: fmt.Sprintf("this target renders as %s, not %s; stored %s, the same fallback the app shows. It takes %s", names[0], requested, names[0], strings.Join(names, ", "))})
	return allowed[0], nil
}

// normalizeWidgetLimit is the limit's twin: a link widget takes none (a sent
// one is dropped with a warning), any other layout takes one of its
// pick-list and rounds the rest to the smallest. The comparison is done in
// int, before any narrowing: an int32 cast of an oversized value would land
// on a list entry by accident.
func normalizeWidgetLimit(layout model.BlockContentWidgetLayout, requested *int, warnings *[]v2model.Issue) int32 {
	options := widgetLimitsFor(layout)
	if layout == model.BlockContentWidget_Link {
		if requested != nil {
			*warnings = append(*warnings, v2model.Issue{Path: "/limit", Message: "a link widget shows no entries, so it takes no limit; the value was not stored"})
		}
		return int32(options[0])
	}
	if requested == nil {
		return int32(options[0])
	}
	names := make([]string, len(options))
	for i, option := range options {
		names[i] = fmt.Sprint(option)
		if *requested == option {
			return int32(option)
		}
	}
	*warnings = append(*warnings, v2model.Issue{Path: "/limit",
		Message: fmt.Sprintf("the %s layout shows %s entries, not %d; stored %d, the same fallback the app shows", widgetLayoutName(layout), strings.Join(names, ", "), *requested, options[0])})
	return int32(options[0])
}

// normalizeStoredLayout re-validates a STORED layout the body did not touch:
// a known layout the target can take stays; anything else — a layout off the
// target's set, or an enum this vocabulary does not know (heart's RPC takes
// any number) — becomes the set's first, the app's render fallback, with a
// warning.
func normalizeStoredLayout(target widgetTarget, stored model.BlockContentWidgetLayout, warnings *[]v2model.Issue) model.BlockContentWidgetLayout {
	allowed, _ := widgetLayoutsFor(target.kind)
	if _, known := widgetLayoutNames[stored]; known {
		for _, l := range allowed {
			if l == stored {
				return stored
			}
		}
	}
	names := make([]string, len(allowed))
	for i, l := range allowed {
		names[i] = widgetLayoutName(l)
	}
	spelled := fmt.Sprintf("layout %d", stored)
	if name, known := widgetLayoutNames[stored]; known {
		spelled = name
	}
	*warnings = append(*warnings, v2model.Issue{Path: "/layout",
		Message: fmt.Sprintf("the stored %s is not one this target renders; stored %s, the same fallback the app shows. It takes %s", spelled, names[0], strings.Join(names, ", "))})
	return allowed[0]
}

// refitWidgetLimit re-fits a STORED limit to a new layout when the body sent
// none: a value the new layout's list also holds stays (the desktop's
// layout action changes nothing else), any other becomes the list's
// smallest, with a warning because the caller did not ask for the change.
func refitWidgetLimit(layout model.BlockContentWidgetLayout, stored int32, warnings *[]v2model.Issue) int32 {
	if layout == model.BlockContentWidget_Link {
		return stored
	}
	options := widgetLimitsFor(layout)
	for _, option := range options {
		if int(stored) == option {
			return stored
		}
	}
	*warnings = append(*warnings, v2model.Issue{Path: "/limit",
		Message: fmt.Sprintf("the stored limit %d is not one the %s layout shows; stored %d, the layout's smallest", stored, widgetLayoutName(layout), options[0])})
	return int32(options[0])
}

// resolveWidgetViewId checks a view_id against the target's own views and
// returns the stored view id. Only a list-kind target has views, and only
// the space root keeps a view: the desktop never writes one on a personal
// widget.
func (s *Service) resolveWidgetViewId(ctx context.Context, spaceId string, scope apicore.WidgetScope, target widgetTarget, viewRef string) (string, error) {
	if viewRef == "" {
		return "", nil
	}
	if target.kind != widgetTargetList {
		return "", v2model.ValidationFailed("this target has no views",
			v2model.Issue{Path: "/view_id", Message: "only a query, collection or type target has views to pick from; omit view_id"})
	}
	if scope == apicore.WidgetScopePersonal {
		return "", v2model.ValidationFailed("a personal widget keeps no view",
			v2model.Issue{Path: "/view_id", Message: "the app shows a personal widget with the target's first view; omit view_id, or make it a space widget"})
	}
	read, err := s.reader.ReadObject(ctx, spaceId, target.stored)
	if err != nil {
		return "", fmt.Errorf("read widget target %s: %w", target.stored, err)
	}
	var dv *model.BlockContentDataview
	for _, block := range read.Snapshot.GetBlocks() {
		if content := block.GetDataview(); content != nil && (dv == nil || block.Id == dataviewBlockId) {
			dv = content
		}
	}
	if dv == nil || len(dv.Views) == 0 {
		return "", v2model.ValidationFailed(fmt.Sprintf("object %q has no views", target.stored),
			v2model.Issue{Path: "/view_id", Message: "omit view_id"})
	}
	ids := make([]string, len(dv.Views))
	for i, view := range dv.Views {
		ids[i] = view.Id
	}
	idx, matches := matchBlockRef(ids, viewRef)
	switch {
	case matches == 1:
		return dv.Views[idx].Id, nil
	case matches > 1:
		return "", v2model.AmbiguousInput(fmt.Sprintf("view %q matches more than one view of %q", viewRef, target.stored),
			v2model.Issue{Path: "/view_id", Message: "the reference is a suffix of several view ids: " + strings.Join(ids, ", ")})
	}
	return "", v2model.ValidationFailed(fmt.Sprintf("object %q has no view %q", target.stored, viewRef),
		v2model.Issue{Path: "/view_id", Message: "its views are " + strings.Join(ids, ", ")})
}

// widgetPlaced is the receipt of a resolved placement.
func widgetPlaced(placement apicore.WidgetPlacement) *v2model.WidgetPlaced {
	switch {
	case placement.AfterId != "":
		return &v2model.WidgetPlaced{After: placement.AfterId}
	case placement.BeforeId != "":
		return &v2model.WidgetPlaced{Before: placement.BeforeId}
	case placement.First:
		return &v2model.WidgetPlaced{Position: v2model.WidgetPositionFirst}
	}
	return &v2model.WidgetPlaced{Position: v2model.WidgetPositionLast}
}

// resolveWidgetPlacement turns after / before / position into a placement,
// after and before naming a widget of the same root by id or target.
func resolveWidgetPlacement(entries []apicore.WidgetEntry, after, before, position string, self string) (apicore.WidgetPlacement, error) {
	given := 0
	for _, v := range []string{after, before, position} {
		if v != "" {
			given++
		}
	}
	if given > 1 {
		return apicore.WidgetPlacement{}, v2model.AmbiguousInput("send at most one of after, before and position",
			v2model.Issue{Message: "after and before name a sibling widget; position is first or last"})
	}
	if self != "" {
		for _, entry := range entries {
			if entry.Id == self && entry.Target == widget.DefaultWidgetBin {
				return apicore.WidgetPlacement{}, v2model.ValidationFailed("the bin widget keeps its place",
					v2model.Issue{Message: "the app never lets the bin be dragged; omit after, before and position"})
			}
		}
	}
	switch {
	case after != "":
		id, err := siblingWidgetId(entries, after, "/after", self)
		if err == nil {
			for _, entry := range entries {
				if entry.Id == id && entry.Target == widget.DefaultWidgetBin {
					return apicore.WidgetPlacement{}, v2model.ValidationFailed("nothing goes after the bin widget",
						v2model.Issue{Path: "/after", Message: "the app lands every drop near the bin before it; send before with the same widget"})
				}
			}
		}
		return apicore.WidgetPlacement{AfterId: id}, err
	case before != "":
		id, err := siblingWidgetId(entries, before, "/before", self)
		return apicore.WidgetPlacement{BeforeId: id}, err
	case position == v2model.WidgetPositionFirst:
		return apicore.WidgetPlacement{First: true}, nil
	case position == v2model.WidgetPositionLast, position == "":
		// last means last among the widgets the app lets a drop land after:
		// a trailing bin stays last, the way the desktop forces every drop
		// near it to land before it
		if n := len(entries); n > 0 && entries[n-1].Target == widget.DefaultWidgetBin && entries[n-1].Id != self {
			return apicore.WidgetPlacement{BeforeId: entries[n-1].Id}, nil
		}
		return apicore.WidgetPlacement{}, nil
	}
	return apicore.WidgetPlacement{}, v2model.ValidationFailed(fmt.Sprintf("unknown position %q", position),
		v2model.Issue{Path: "/position", Message: "position is first or last; after and before place relative to a sibling"})
}

// siblingWidgetId resolves an after/before reference within one root.
func siblingWidgetId(entries []apicore.WidgetEntry, ref, path, self string) (string, error) {
	matches := matchWidgetRef(entries, ref)
	switch {
	case len(matches) == 1 && matches[0].Id == self:
		return "", v2model.ValidationFailed("a widget cannot be placed relative to itself",
			v2model.Issue{Path: path, Message: "name another widget of the same scope, or send position first or last"})
	case len(matches) == 1:
		return matches[0].Id, nil
	case len(matches) > 1:
		return "", v2model.AmbiguousInput(fmt.Sprintf("%q names more than one widget", ref),
			v2model.Issue{Path: path, Message: "use the widget id: " + widgetIdList(matches)})
	}
	return "", v2model.ValidationFailed(fmt.Sprintf("no widget %q in this scope", ref),
		v2model.Issue{Path: path, Message: "after and before name a widget of the same scope by its id or its target"})
}

// matchWidgetRef finds the widgets a reference names: by widget id, else by
// target in either spelling.
func matchWidgetRef(entries []apicore.WidgetEntry, ref string) []apicore.WidgetEntry {
	for _, entry := range entries {
		if entry.Id == ref {
			return []apicore.WidgetEntry{entry}
		}
	}
	stored := anyblockjson.WireWidgetTarget(ref)
	var matches []apicore.WidgetEntry
	for _, entry := range entries {
		if entry.Target == stored || entry.Target == ref {
			matches = append(matches, entry)
		}
	}
	return matches
}

func widgetIdList(entries []apicore.WidgetEntry) string {
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = fmt.Sprintf("%s (%s)", entry.Id, entry.Scope)
	}
	return strings.Join(ids, ", ")
}

// resolveWidget finds the widget a path segment names, across the roots a
// scope filter selects: by widget id first, then by target. A target
// present in both roots needs the scope said.
func (s *Service) resolveWidget(ctx context.Context, spaceId, ref, scopeFilter string) (apicore.WidgetEntry, []apicore.WidgetEntry, error) {
	scopes, err := widgetScopesOf(scopeFilter)
	if err != nil {
		return apicore.WidgetEntry{}, nil, err
	}
	var matches []apicore.WidgetEntry
	byScope := map[apicore.WidgetScope][]apicore.WidgetEntry{}
	for _, scope := range scopes {
		entries, err := s.widgets.ListWidgets(ctx, spaceId, scope)
		if err != nil {
			return apicore.WidgetEntry{}, nil, widgetPortError("resolve widget", err)
		}
		byScope[scope] = entries
		matches = append(matches, matchWidgetRef(entries, ref)...)
	}
	switch {
	case len(matches) == 1:
		return matches[0], byScope[matches[0].Scope], nil
	case len(matches) > 1:
		// the scope parameter disambiguates only across roots; two widgets
		// for one target in ONE root (heart's own create path never checked)
		// are told apart by widget id alone
		issue := v2model.Issue{Path: "widget_id", Message: "use the widget id: " + widgetIdList(matches)}
		if matches[0].Scope != matches[len(matches)-1].Scope {
			issue.Message = "say which with the scope parameter, or use the widget id: " + widgetIdList(matches)
			issue = issue.Hintf("resend with %s or %s", v2model.Resend("scope", v2model.WidgetScopeSpace), v2model.Resend("scope", v2model.WidgetScopePersonal))
			return apicore.WidgetEntry{}, nil, v2model.AmbiguousInput(fmt.Sprintf("%q names a widget in both scopes", ref), issue)
		}
		return apicore.WidgetEntry{}, nil, v2model.AmbiguousInput(fmt.Sprintf("%q names more than one widget in the %s sidebar", ref, matches[0].Scope),
			issue.Hintf("list them with %s", v2model.RefListWidgets(spaceId)))
	}
	return apicore.WidgetEntry{}, nil, v2model.NotFound(fmt.Sprintf("no widget %q", ref),
		v2model.Issue{Path: "widget_id", Message: "a widget is addressed by its id or by its target"}.
			Hintf("list the sidebar with %s", v2model.RefListWidgets(spaceId)))
}

// CreateWidget implements POST /v2/spaces/{space_id}/widgets.
func (s *Service) CreateWidget(ctx context.Context, spaceId string, req v2model.CreateWidgetRequest, dryRun bool) (*v2model.WidgetResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	if err := s.ensureWidgets(); err != nil {
		return nil, err
	}
	scope, err := parseWidgetScope(req.Scope, "/scope")
	if err != nil {
		return nil, err
	}
	if err := s.ensureWidgetWritable(ctx, spaceId, scope); err != nil {
		return nil, err
	}
	target, err := s.resolveWidgetTarget(spaceId, req.Target)
	if err != nil {
		return nil, err
	}
	entries, err := s.widgets.ListWidgets(ctx, spaceId, scope)
	if err != nil {
		return nil, widgetPortError("create widget", err)
	}
	for _, entry := range entries {
		if entry.Target == target.stored {
			return nil, v2model.ValidationFailed(fmt.Sprintf("the %s sidebar already holds a widget for %q", scope, req.Target),
				v2model.Issue{Path: "/target", Message: "one widget per target per scope, the way the app toggles them; update the existing widget " + entry.Id + " instead"})
		}
	}
	// a member that is present but empty is refused, the way the served
	// schema refuses it, rather than read as omitted
	for path, value := range map[string]*string{"/layout": req.Layout, "/after": req.After, "/before": req.Before, "/position": req.Position} {
		if value != nil && *value == "" {
			return nil, v2model.ValidationFailed("an empty "+strings.TrimPrefix(path, "/")+" says nothing",
				v2model.Issue{Path: path, Message: "omit the member for the default; layout is one of link, tree, list, compact_list, view; after and before name a sibling widget; position is first or last"})
		}
	}
	var warnings []v2model.Issue
	layout, err := normalizeWidgetLayout(target, deref(req.Layout), &warnings)
	if err != nil {
		return nil, err
	}
	limit := normalizeWidgetLimit(layout, req.Limit, &warnings)
	viewId, err := s.resolveWidgetViewId(ctx, spaceId, scope, target, req.ViewId)
	if err != nil {
		return nil, err
	}
	placement, err := resolveWidgetPlacement(entries, deref(req.After), deref(req.Before), deref(req.Position), "")
	if err != nil {
		return nil, err
	}
	create := apicore.WidgetCreate{Target: target.stored, Layout: layout, Limit: limit, ViewId: viewId, Placement: placement}
	asked := req.After != nil || req.Before != nil || req.Position != nil
	if dryRun {
		row := widgetRow(apicore.WidgetEntry{Scope: scope, Target: create.Target, Layout: create.Layout, Limit: create.Limit, ViewId: create.ViewId})
		result := &v2model.WidgetResult{WidgetRow: row, DryRun: true, Warnings: warnings}
		if asked {
			result.Placed = widgetPlaced(placement)
		}
		return result, nil
	}
	entry, err := s.widgets.CreateWidget(ctx, spaceId, scope, create)
	if err != nil {
		return nil, widgetPortError("create widget", err)
	}
	result := &v2model.WidgetResult{WidgetRow: widgetRow(entry), Warnings: warnings}
	if asked {
		// the receipt is the placement as APPLIED under the lock, which can
		// differ from the one resolved on the read (a bin that became last
		// meanwhile turns last into before it)
		result.Placed = widgetPlaced(placement)
		if entry.Placed != nil {
			result.Placed = widgetPlaced(*entry.Placed)
		}
	}
	return result, nil
}

// UpdateWidget implements PATCH /v2/spaces/{space_id}/widgets/{widget_id}.
func (s *Service) UpdateWidget(ctx context.Context, spaceId, ref, scopeFilter string, req v2model.UpdateWidgetRequest, dryRun bool) (*v2model.WidgetResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	if err := s.ensureWidgets(); err != nil {
		return nil, err
	}
	if req.Layout == nil && req.Limit == nil && req.ViewId == nil && req.After == nil && req.Before == nil && req.Position == nil {
		return nil, v2model.ValidationFailed("update needs at least one of layout, limit, view_id, after, before, position",
			v2model.Issue{Message: "omitted members stay unchanged; the target is the widget's identity and does not change"})
	}
	entry, siblings, err := s.resolveWidget(ctx, spaceId, ref, scopeFilter)
	if err != nil {
		return nil, err
	}
	if err := s.ensureWidgetWritable(ctx, spaceId, entry.Scope); err != nil {
		return nil, err
	}
	target := s.existingWidgetTarget(spaceId, entry)
	// the members that need no lock: a view id resolves against the target,
	// a placement against the siblings — neither depends on the widget's
	// own stored pair
	var viewId *string
	if req.ViewId != nil {
		resolved, err := s.resolveWidgetViewId(ctx, spaceId, entry.Scope, target, *req.ViewId)
		if err != nil {
			return nil, err
		}
		viewId = &resolved
	}
	var placement *apicore.WidgetPlacement
	// a placement member that is present but empty is refused rather than
	// read as "last": on a PATCH an empty anchor would silently move the
	// widget to the end
	for path, value := range map[string]*string{"/after": req.After, "/before": req.Before, "/position": req.Position} {
		if value != nil && *value == "" {
			return nil, v2model.ValidationFailed("an empty "+strings.TrimPrefix(path, "/")+" places nothing",
				v2model.Issue{Path: path, Message: "omit the member to keep the position; after and before name a sibling widget, position is first or last"})
		}
	}
	if req.Layout != nil && *req.Layout == "" {
		return nil, v2model.ValidationFailed("an empty layout changes nothing",
			v2model.Issue{Path: "/layout", Message: "omit the member to keep the layout; it is one of link, tree, list, compact_list, view"})
	}
	if req.After != nil || req.Before != nil || req.Position != nil {
		resolved, err := resolveWidgetPlacement(siblings, deref(req.After), deref(req.Before), deref(req.Position), entry.Id)
		if err != nil {
			return nil, err
		}
		placement = &resolved
	}
	// the layout and the limit are a PAIR, planned against the widget as
	// it is UNDER THE LOCK (the adapter hands the plan the current entry),
	// so a concurrent change of either member is seen, not overwritten:
	// a body that sends one member still writes both, the stored layout is
	// re-validated against the target whenever either changes (heart
	// itself never checked it), and a layout change with no limit in the
	// body keeps the stored limit when the new list holds it. The plan's
	// warnings are the ones of the run that committed.
	var warnings []v2model.Issue
	plan := func(current apicore.WidgetEntry) (apicore.WidgetUpdate, error) {
		warnings = warnings[:0]
		if current.Target != entry.Target {
			// heart's own RPC can retarget a wrapper; everything above was
			// validated against the target read before the lock
			return apicore.WidgetUpdate{}, v2model.NewError(http.StatusConflict, v2model.CodeEtagMismatch,
				fmt.Sprintf("the widget's target changed while the request was in flight (now %q); re-read the sidebar and retry", anyblockjson.FormatWidgetTarget(current.Target)))
		}
		update := apicore.WidgetUpdate{ViewId: viewId, Placement: placement}
		if req.Layout == nil && req.Limit == nil {
			return update, nil
		}
		var layout model.BlockContentWidgetLayout
		var err error
		if req.Layout != nil {
			layout, err = normalizeWidgetLayout(target, *req.Layout, &warnings)
			if err != nil {
				return apicore.WidgetUpdate{}, err
			}
		} else {
			layout = normalizeStoredLayout(target, current.Layout, &warnings)
		}
		var limit int32
		switch {
		case layout == model.BlockContentWidget_Link:
			// a link widget shows no entries: a sent limit is not stored —
			// and the stored one is KEPT, so a trip through link and back
			// comes home to the same limit
			if req.Limit != nil {
				warnings = append(warnings, v2model.Issue{Path: "/limit", Message: "a link widget shows no entries, so it takes no limit; the value was not stored and the stored one stays"})
			}
			limit = current.Limit
		case req.Limit != nil:
			limit = normalizeWidgetLimit(layout, req.Limit, &warnings)
		default:
			limit = refitWidgetLimit(layout, current.Limit, &warnings)
		}
		update.Layout = &layout
		update.Limit = &limit
		return update, nil
	}
	if dryRun {
		// the preview plans against the read above: the best answer with no
		// lock taken, and the one the commit re-plans under it
		update, err := plan(entry)
		if err != nil {
			return nil, err
		}
		next := entry
		if update.Layout != nil {
			next.Layout = *update.Layout
		}
		if update.Limit != nil {
			next.Limit = *update.Limit
		}
		if update.ViewId != nil {
			next.ViewId = *update.ViewId
		}
		result := &v2model.WidgetResult{WidgetRow: s.servedWidgetRow(spaceId, next, &warnings), DryRun: true}
		result.Warnings = warnings
		if placement != nil {
			result.Placed = widgetPlaced(*placement)
		}
		return result, nil
	}
	updated, err := s.widgets.UpdateWidget(ctx, spaceId, entry.Scope, entry.Id, plan)
	if err != nil {
		var v2e *v2model.Error
		if errors.As(err, &v2e) {
			return nil, err
		}
		return nil, widgetPortError("update widget", err)
	}
	result := &v2model.WidgetResult{WidgetRow: s.servedWidgetRow(spaceId, updated, &warnings), Warnings: warnings}
	result.Warnings = warnings
	if placement != nil {
		result.Placed = widgetPlaced(*placement)
		if updated.Placed != nil {
			result.Placed = widgetPlaced(*updated.Placed)
		}
	}
	return result, nil
}

// DeleteWidget implements DELETE /v2/spaces/{space_id}/widgets/{widget_id}.
func (s *Service) DeleteWidget(ctx context.Context, spaceId, ref, scopeFilter string, dryRun bool) (*v2model.WidgetResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	if err := s.ensureWidgets(); err != nil {
		return nil, err
	}
	entry, _, err := s.resolveWidget(ctx, spaceId, ref, scopeFilter)
	if err != nil {
		return nil, err
	}
	if err := s.ensureWidgetWritable(ctx, spaceId, entry.Scope); err != nil {
		return nil, err
	}
	var warnings []v2model.Issue
	if dryRun {
		row := s.servedWidgetRow(spaceId, entry, &warnings)
		return &v2model.WidgetResult{WidgetRow: row, DryRun: true, Warnings: warnings}, nil
	}
	removed, err := s.widgets.DeleteWidget(ctx, spaceId, entry.Scope, entry.Id, entry.Target)
	if err != nil {
		return nil, widgetPortError("delete widget", err)
	}
	row := s.servedWidgetRow(spaceId, removed, &warnings)
	return &v2model.WidgetResult{WidgetRow: row, Removed: true, Warnings: warnings}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
