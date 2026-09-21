package v2model

// widget.go — the sidebar widget surface (APIV2_WIDGETS.md). A widget is one
// row of a space's sidebar: what it points at, how it renders, how many
// entries it shows. The members it shares with the AnyBlock index.json
// widget (target, layout, limit, view_id) carry the format's spellings; the
// row is not an index widget verbatim (it also carries id and scope, and a
// personal widget has no bundle form).

// Widget scopes: which of a space's two sidebar roots a widget lives in.
const (
	// WidgetScopeSpace is the space's shared sidebar (the desktop's Pinned
	// section): every member sees it, the owner and admins write it.
	WidgetScopeSpace = "space"
	// WidgetScopePersonal is this account's own sidebar for the space (the
	// desktop's My Favorites): any member who can write in the space keeps
	// their own.
	WidgetScopePersonal = "personal"
)

// Widget layouts, the format's spellings.
const (
	WidgetLayoutLink        = "link"
	WidgetLayoutTree        = "tree"
	WidgetLayoutList        = "list"
	WidgetLayoutCompactList = "compact_list"
	WidgetLayoutView        = "view"
)

// Widget positions, the two absolute placements a create or an update takes
// beside the relative after/before.
const (
	WidgetPositionFirst = "first"
	WidgetPositionLast  = "last"
)

// WidgetRow is one sidebar widget.
type WidgetRow struct {
	// Id of the widget, what update and delete address.
	Id string `json:"id"`
	// Which sidebar root holds it: space, the shared sidebar the owner and admins write, or personal, this account's own.
	Scope string `json:"scope"`
	// What the widget points at: an object id, or a built-in listing spelled with a leading underscore, such as _favorite or _bin.
	Target string `json:"target"`
	// How it renders: link is one row, tree expands the object's children, list and compact_list show a query's or type's entries, view renders the target's own view.
	Layout string `json:"layout"`
	// How many entries a listing shows. Absent on a link widget, which shows none.
	Limit int `json:"limit,omitempty"`
	// Which of the target's views a view widget shows. Absent means the target's first view.
	ViewId string `json:"view_id,omitempty"`
}

// CreateWidgetRequest is the POST /v2/spaces/{space_id}/widgets body.
type CreateWidgetRequest struct {
	// Id of the object the widget points at. Built-in listings cannot be added; the app no longer offers them either.
	Target string `json:"target"`
	// space or personal. Required: the two roots have different readers and different writers, so the choice is the caller's.
	Scope string `json:"scope"`
	// link, tree, list, compact_list or view. Omitted, the target's kind picks it: tree for a page, view for a query, collection or type, link for everything else. A layout the target cannot render is replaced by the one it can, with a warning.
	Layout *string `json:"layout,omitempty"`
	// How many entries a listing shows, one of 6, 10, 14, 30, 50, or 4, 6, 8, 30, 50 for the list layout. Omitted, the smallest. Any other value is rounded to the smallest, with a warning.
	Limit *int `json:"limit,omitempty"`
	// Which of the target's views a view widget shows, by view id. Only a query, collection or type target has views, and only a space widget keeps one.
	ViewId string `json:"view_id,omitempty"`
	// Id or target of the sidebar widget to place this one after. At most one of after, before and position.
	After *string `json:"after,omitempty"`
	// Id or target of the sidebar widget to place this one before.
	Before *string `json:"before,omitempty"`
	// first or last. Omitted with no after or before, the widget goes last, or before the bin widget when that is last.
	Position *string `json:"position,omitempty"`
}

// UpdateWidgetRequest is the PATCH /v2/spaces/{space_id}/widgets/{widget_id}
// body: omitted members stay unchanged (pointers distinguish absent from
// present-but-empty; at least one member is required), with one exception:
// the layout and the limit are a pair, re-validated together whenever either
// is sent — a layout change re-fits an omitted limit the new list lacks, and
// a limit change replaces an omitted stored layout the target cannot render
// (heart never checked it). The target is the widget's identity and does not
// change: delete the widget and create one for the other target.
type UpdateWidgetRequest struct {
	// link, tree, list, compact_list or view. A layout the target cannot render is replaced by the one it can, with a warning.
	Layout *string `json:"layout,omitempty"`
	// How many entries a listing shows, from the same fixed set a create takes.
	Limit *int `json:"limit,omitempty"`
	// Which of the target's views a view widget shows. Empty string returns to the target's first view.
	ViewId *string `json:"view_id,omitempty"`
	// Id or target of the sidebar widget to move this one after. At most one of after, before and position.
	After *string `json:"after,omitempty"`
	// Id or target of the sidebar widget to move this one before.
	Before *string `json:"before,omitempty"`
	// first or last; last is before the bin widget when that is last.
	Position *string `json:"position,omitempty"`
}

// WidgetPlaced is the receipt of a placement: where the widget went, in
// widget ids, so a dry run shows the move it would make.
type WidgetPlaced struct {
	// Id of the widget this one now follows.
	After string `json:"after,omitempty"`
	// Id of the widget this one now precedes.
	Before string `json:"before,omitempty"`
	// first or last, when the placement was absolute.
	Position string `json:"position,omitempty"`
}

// WidgetResult is what every widget mutation answers with: the widget as it
// now is, or as it would be on a dry run.
type WidgetResult struct {
	WidgetRow
	// True on a dry run, when nothing was written.
	DryRun bool `json:"dry_run,omitempty"`
	// True when this delete removed the widget.
	Removed bool `json:"removed,omitempty"`
	// Where the widget was placed, present when the request asked for a placement.
	Placed *WidgetPlaced `json:"placed,omitempty"`
	// What the server changed about the request on the way, such as a layout or limit the target could not take.
	Warnings []Issue `json:"warnings,omitempty"`
}
