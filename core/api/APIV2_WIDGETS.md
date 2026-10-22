# API v2 — sidebar widgets

Status: PR #3293, 2026-09-21, branch `go-0000-apiv2-widgets` (no ticket yet) · the phase entry is `APIV2.md` §2 Phase 8 · §8 below records where the build departed from §5, §9 is the as-built summary.

This document records what the desktop client lets a user do with sidebar
widgets today, what the heart actually enforces (almost nothing), what the
AnyBlock `index.json` widget grammar looks like, and the surface proposed
for `/v2` so that an agent can add, edit, reorder and remove widgets under
**the same limitations the desktop UI has**. Open decisions are in §7.

Sources: `anytype-ts` `develop` @ `1e78a507f0`; heart `develop` @
`c68386357`; any-block `v0.0.0-20260919082013-9906a7c36b68`.

---

## 1. What a widget is on the wire

One **wrapper block** (`BlockContentWidget{layout, limit, viewId, autoAdded}`)
holding exactly one child **link block** (`BlockContentLink{targetBlockId,
cardStyle, iconSize, description, relations}`). The root's `ChildrenIds`
order is the sidebar order. Every reader takes `ChildrenIds[0]`.

There are **two widget roots per space**, and the desktop renders them as two
different sections:

| root | id | desktop section | who may write | ordering |
|---|---|---|---|---|
| space widgets | `spc.DerivedIDs().Widgets` (derived, one per space, CRDT tree) | **Pinned** ("Pin to Channel") | owner/admin only in shared spaces (`CanManageSpace` minus its one-to-one exemption: the desktop's pin gate is owner-or-admin, which nobody is in a one-to-one space) — `checkSpaceConfigLock` refuses inside `Apply` (`core/block/editor/smartblock/smartblock.go:503-512`), after the state was merged; the UI gates on `canMyParticipantModerate()` | root `ChildrenIds` |
| personal widgets | `domain.NewPersonalWidgetsId(spaceId)` = `_personalWidgets_<spaceId with the dot → _>` (virtual object, backed by a per-account tech-space CRDT store, `core/block/personalfavorites`) | **My Favorites** | any participant who can write (UI gate `canMyParticipantWrite()`; the heart does not check) | `AfterId` chain in the store, projected to `ChildrenIds` |

Both roots accept the **same block RPCs** (`BlockCreateWidget`,
`BlockWidgetSetLayout/Limit/ViewId`, `BlockListDelete`,
`BlockListMoveToExistingObject`) — the personal root's `VirtualWidgetObject`
embeds the same `widget.Widget` and translates block edits into store
deltas. Two personal-root differences: `TargetId` is **immutable** on an
entry (change = delete + create), and the UI never writes `viewId` there.

The `index.json` `widgets` array is the **space** root only. There is no
bundle representation of personal widgets.

## 2. The AnyBlock `index.json` widget (format v2, §2c)

```json
{ "target": "type-fieldnote", "layout": "view", "limit": 6, "view_id": "…",
  "card_style": "card", "icon_size": "medium", "description": "none",
  "properties": ["…"], "auto_added": false }
```

- `target`: a bundle object id, or one of the eight reserved listings
  `_favorite _recent _recent_open _set _collection _all_objects _chat _bin`
  (heart store spellings: `favorite recent recentOpen set collection
  allObjects chat bin`; translation `anyblockjson.WireWidgetTarget` /
  `FormatWidgetTarget`).
- `layout`: `link` (default) `tree` `list` `compact_list` `view`.
- `limit`: 0..100 (the block itself takes the whole int32 range).
- `card_style / icon_size / description / properties`: the link child's
  display members. `description` spells proto `Added` as `manual`.
- `auto_added`, `auto_widget_targets`, `auto_widget_disabled`: machine
  state. **Dead in heart** since `9bd3ce10b` (GO-6247, 2025-09-18): the
  auto-widget code and both bundled relations were deleted. any-block still
  round-trips them.

The codec (`github.com/anyproto/any-block/codec/anyblockjson`) exports
`Widget`, `Index`, the target spelling maps and predicates,
`IndexFromWidgetObject` (fail-closed, writes nothing on any surprise) and
`WidgetsSnapshot`. The four enum vocabularies (`widgetLayoutNames`,
`cardStyleNames`, `iconSizeNames`, `linkDescriptionNames`) are
**unexported**; `viewvocab.go` is the precedent for exporting them.

## 3. What the desktop UI allows today (the limitations to mirror)

The widget UI was rewritten into a section sidebar in Sep 2025. Several
things no longer exist in the client: the "Add widget" object picker,
"Change source", creating `set`/`collection`/`allObjects`/`chat` widgets,
auto-widgets. Their data still sits in users' widget objects.

### 3.1 Creation paths and target gating

Widgets are created only by toggling **Pin to Channel** (space root) or
**Favorite** (personal root) on an object, by the `pin` shortcut, or by
dropping an object into the sidebar. The gate
(`src/ts/component/menu/object.tsx:125-126`):

```ts
allowedPinToChannel = canWrite && !isRelation && !isTemplate && !object.isArchived && canMyParticipantModerate();
allowedFavorite     = canWrite && !isRelation && !isTemplate && !object.isArchived;
```

So the UI excludes **only** relations, templates and archived/deleted
objects. Files, images, bookmarks, dates, participants, chats, notes,
types, sets and collections are all allowed (they just get different
layouts). Toggling means **one widget per target per root** — the UI cannot
create a duplicate; the RPC does not check.

**Predefined listings are not creatable any more.** `favorite`, `recent`,
`recentOpen`, `bin` widgets that already exist are rendered (via
`U.Menu.getSystemWidgets()`), editable (layout/limit) and removable.
`allObject`/`chat` widget blocks are **hidden** (filtered out of the Pinned
section). `set`/`collection` targets were migrated by heart (state
migration 2) into the Set/Collection **type ids** with layout `view` and
`viewId = "all"`.

### 3.2 Layout per target — `U.Menu.getWidgetLayoutOptions` (`src/ts/lib/util/menu.ts:535-577`)

| target | offered layouts (first = fallback) | UI default at creation |
|---|---|---|
| page layouts: Page, Human (profile), Task, Note, Bookmark | `tree`, `link` | `tree` |
| Set, Collection | `view`, `compact_list`, `list`, `link` | `view` |
| Type | `view`, `compact_list`, `list`, `link` | `link` (quirk: the file/system branch of `createWidgetFromObjectIn` wins over the set branch; heart's own migrations used `view` + `viewId:"all"`) |
| File, Image, Audio, Video, Pdf, Participant, Date, Chat, Discussion, Dashboard, Space, SpaceView, Option | `link` only | `link` |
| existing `favorite`, `recent`, `recentOpen` | `compact_list`, `list`, `tree` (no link, no view) | — |
| existing `bin` | `link`, `compact_list`, `list`, `tree` | — |

A stored layout outside the set is **not** an error anywhere — the client
silently renders `options[0]`.

### 3.3 Limit — `U.Menu.getWidgetLimitOptions` (`menu.ts:519-533`)

Fixed pick-lists, no free text: `list` → **4, 6, 8, 30, 50**; every other
layout → **6, 10, 14, 30, 50**. Default = first option (6, or 4 for
`list`). The row is hidden for `link` widgets (a value is still written).
`favorite` ignores the stored limit at render (unbounded).

### 3.4 View

`viewId` is set only through the view tabs inside a dataview widget when
the target has more than one view, and **only on the space root**
(`widget/view/index.tsx:177-184`). Never sent on create. Empty = the
target's first view. No restriction on view type.

### 3.5 Link-child display members

Never set by the UI: `cardStyle=0, iconSize=0, description=0,
relations=[]` on every UI-created widget (`action.ts:926-929`,
`mapper.ts:1087-1095`).

### 3.6 Edit, delete, reorder

- Editable: `layout`, `limit`, `viewId` (space root; personal root: layout
  and limit only). **Not** editable: target (`BlockWidgetSetTargetId` has
  zero call sites), name, `autoAdded`.
- Delete: `BlockListDelete(root, [wrapperId])`; heart unlinks wrapper +
  link together either way.
- Reorder: `BlockListMoveToExistingObject(root, root, siblingWrapperId,
  [wrapperId], Top|Bottom)`; `bin` is not draggable. No max count.

### 3.7 Things that are NOT widget data

Sections (Unread, Type, RecentEdit, Bin, My Favorites "show as"), their
order, visibility and collapse state are **device-local storage**. The
space icon, the home row and the Unread/Type/Recent/Bin rows are synthetic
blocks. `content.section` is client-only.

## 4. What the heart enforces

Essentially nothing beyond shape (`core/block/editor/widget/widget.go`):
the child must be a link with a non-empty `targetBlockId`. No layout
range check, no limit bounds, no target existence, no layout/target
compatibility, no `viewId` check, no duplicate check on the public RPC.
`SetLayout/SetLimit/SetViewId` silently no-op on a non-widget block. An
unresolvable target becomes `addr.MissingObject` and `WidgetObject.Init`
strips the widget silently on the next load.

Traps that shape the implementation:

1. `CreateBlock` returns the **wrapper** id; `SetLayout/SetLimit/SetViewId`,
   move and delete take the wrapper id; `BlockLinkListSetAppearance` takes
   the **link** id.
2. `model.Block_Inner` (5) = append **last**, `Block_InnerFirst` (7) =
   prepend **first**; with `targetId == ""` anything else is coerced to
   `Inner`.
3. Space-root writes from a plain writer in a shared space fail inside
   `Apply` via the ACL lock — pre-check `CanManageSpace` and answer 403.
4. `apicore.ClientCommands` has only `BlockListDelete` of the needed RPCs.
5. `builtinobjects.createWidgets` shows the batching pattern: N
   `CreateBlock` inside one `cache.DoStateCtx` = one `Apply`, one change.

## 5. Proposed surface

Path noun `widgets`, under the space, mirroring the `index.json` widget
shape and its spellings (one grammar, four serializations):

| op id | route | scope gate |
|---|---|---|
| `list_widgets` | `GET /v2/spaces/{space_id}/widgets` | read |
| `create_widget` | `POST /v2/spaces/{space_id}/widgets` | write, C8 idempotent, C9 dry-run |
| `update_widget` | `PATCH /v2/spaces/{space_id}/widgets/{widget_id}` | write, C8, C9 |
| `delete_widget` | `DELETE /v2/spaces/{space_id}/widgets/{widget_id}` | write, C8 |

Row (`WidgetRow`):

```json
{ "id": "<wrapper block id>", "scope": "space" | "personal",
  "target": "<object id>" | "_favorite" | …, "layout": "tree",
  "limit": 6, "view_id": "…" }
```

- `scope` distinguishes the two roots. List returns space widgets first,
  then personal, each in sidebar order; `?scope=` narrows.
- `target` uses the index.json spelling for the reserved listings.
- `limit` is omitted for `link` (meaningless), `view_id` omitted when
  empty.
- Legacy link display members (`card_style`, `icon_size`, `description`,
  `properties`) are **not exposed** in v1; `auto_added` never.

Create body: `{target, scope?, layout?, limit?, view_id?, after?, before?,
position?}`. Update body: `{layout?, limit?, view_id?, after?, before?,
position?}` (pointers; all-nil refused). Reorder is folded into PATCH with
the ops-surface vocabulary (`after` | `before` | `position: first|last`,
at most one). Mutations return the row (201 on create, 200 otherwise;
DELETE answers 200 with the removed row, never 204).

### 5.1 Validation (the UI limitations, made explicit)

| rule | refusal |
|---|---|
| target resolves in the space (store lookup); not archived/deleted; `resolvedLayout` ≠ relation; type ≠ template | 400 `/target` with the reason; unknown id → 404-style issue with the `find` steer |
| reserved targets (`_favorite` …) accepted on read/update/delete of **existing** widgets; creation refused (§7 Q2) | 400 `/target` naming the rule |
| layout ∈ allowed set for the target's layout (table §3.2); default = UI default (Type → `view`, see §7 Q4) | 400 `/layout` listing the allowed values for this target |
| limit ∈ fixed list for the layout (§3.3); default = first option; ignored/refused on `link` (§7 Q4) | 400 `/limit` listing the allowed values |
| `view_id` only for Set/Collection/Type targets and must name one of the target's views (reuse `listViews` in `list_read.go`); refused on `personal` scope | 400 `/view_id` with the view list |
| one widget per target per scope | 400 `/target` naming the existing widget id |
| `scope: space` requires `CanManageSpace`; `personal` requires a writing participant | 403 |
| `target` immutable after creation | update body has no `target` (schema refuses) |

### 5.2 Implementation shape

- **Heart port**: a new narrow `apicore.Widgets` port implemented in
  package `api` (`widgetadapter.go`, the composition root) rather than
  extending `ClientCommands`. It resolves both root ids
  (`spaceService.Get(...).DerivedIDs().Widgets`,
  `domain.NewPersonalWidgetsId`), walks the root's children under
  `cache.DoStateCtx` (the `extractEntriesFromState` shape,
  `virtualwidget.go:203-247`, returning wrapper **and** link ids), and does
  create / set / move / unlink in **one** state pass per request. It
  pre-checks `CanManageSpace` for the space root.
- **Vocabulary**: export the four widget enum name tables from any-block
  (`widgetvocab.go`, mirroring `viewvocab.go`) and re-export through
  `pkg/lib/anyblockjson/bridge.go`, so the API's enum strings cannot drift
  from what the codec reads and writes. Target spelling via
  `WireWidgetTarget`/`FormatWidgetTarget`.
- **v2 layers**: `model/widget.go`, `service/widgets.go` (validation table
  + `v2WidgetRpcError` classifier), `handler/widget.go`
  (`decodeStrictJSONBody`, `respondV2Create`), router rows, `authz.go`
  rows, `doc.go` tag, `ref.go` ops + `RefListWidgets`, `schemas.go`
  `widget` kind, `openapibodies.go` recipes, `markdown/api.md` bullet.
- **Goldens**: `openapi_responses_test.go` (operation count, pair count,
  dry-run/idempotent/body-limited sets), `scripts/fix_openapi_v2.py`
  mirror sets, `grant_gate_test.go` `knownRouteParams` += `widget_id`,
  `v2_router_test.go` idempotency + auth lists, `ref_test.go` helper list;
  `make openapi` (never `go mod tidy`).
- **Wrapper**: no curated tool in v1 (14 → 15 is a recorded design
  constraint); the external `anytype-mcp` bridge gets the four operations
  from the OpenAPI document. Update the `is_favorite` steer in
  `APIV2_TYPED_HINTS_ROUND2.md:425` to point at `create_widget`
  (`scope: personal`).
- **Tests**: service tests on `newV2Fixture` + `StoreFixture` for every
  row of §5.1; handler tests; a real-heart smoke run as in GO-7530.

## 6. Out of scope (recorded)

Sections and their device-local state; the home object (already a space
detail); synthetic rows (Unread/Type/Recent/Bin); auto-widgets; link
display members; bundle-level `auto_widget_*`; a `move` op separate from
PATCH; a curated wrapper tool.

## 7. Decisions (2026-09-21)

| id | question | decision |
|---|---|---|
| Q1 | Scope | **both** roots in v1: `scope: space` (desktop "Pinned", owner/admin) and `scope: personal` (desktop "My Favorites", any writer) |
| Q2 | Reserved listing targets | **refused on create**; existing ones are listed, editable and deletable under their `_` spellings |
| Q3 | Widget address | **widget id or target id** — the path segment resolves as a wrapper block id first, then as a target object id; a target present in both scopes without `?scope=` is refused as ambiguous |
| Q4 | Out-of-set layout / limit | **normalise silently** like the desktop's render fallback: store the first allowed option for that target and report the substitution in `warnings`; `limit` on a `link` widget is dropped with a warning. §5.1 rows for layout and limit read accordingly |
| Q5 | Hidden legacy widgets (`_all_objects`, `_chat`) | listed (default lean, not asked) so an agent can delete stale ones |

## 8. As built (2026-09-21)

Departures from §5, each deliberate:

- `scope` is **required** on create rather than defaulting to `space`: a
  default that refuses every editor of a shared space is not a default.
- Reorder rides PATCH (`after` / `before` / `position`), as proposed; the
  adapter does it as unlink + insert on the root's children in the same
  state pass as the member changes, so an update is one `Apply`.
- A layout change on PATCH keeps the stored limit when the new layout's
  pick-list holds it and re-fits it with a warning otherwise; the layout
  and the limit are always written together, so a concurrent update of the
  other member cannot leave a pair the app would not show.
- Participant and date objects have `_`-prefixed ids in heart, so the
  underscore alone never refuses a target: the store decides (through the
  virtual-aware `QueryByIds`, so a date resolves without a persisted row),
  and only a prefixed id the store does not know, outside the participant
  and date prefixes, is read as a misspelled listing.
- The adapter re-checks the duplicate rule and the write permission under
  the root's lock (two concurrent creates for one target, a role changed
  between check and write), and mints the link id so the wrapper id it
  returns is the one the personal root keeps across store rebuilds.
- A non-empty `view_id` on a personal widget is refused (the desktop never
  writes one there), and on a target without views; an unknown view lists
  the views. An empty `view_id` clears the stored one on either root.
- A PATCH whose plan finds the widget retargeted under the lock (heart's
  own `BlockWidgetSetTargetId` can do that) is a 409: everything was
  validated against the target read before the lock. A DELETE resolved by
  target carries that target into the lock and is refused the same way
  rather than removing a widget under a stale name.
- A stored layout heart wrote with an enum this vocabulary does not know
  (its RPC takes any number) is treated on PATCH like an off-set one: the
  target's first layout, with a warning, never `link` by accident.
- The bin widget keeps its place (the desktop never lets it be dragged) and
  nothing is placed after it (the desktop lands every drop near it before
  it): `after: _bin` is refused, and a default or `last` placement with a
  trailing bin lands before the bin; a requested `last` is receipted as
  `before` the bin (a POST that asked for no placement carries no receipt).
  The rule is resolved on the service's read and enforced again by the
  adapter against the root under the lock; the receipt of a committed write
  is the placement the adapter actually applied, so a bin that became last
  meanwhile is reported, not guessed.
- A stored layout heart wrote with an enum this vocabulary does not know is
  SERVED as the target's render fallback (what the desktop shows), with a
  warning naming the widget, on reads and on mutation answers alike; a
  layout or limit update then stores that fallback.
- A PATCH to `link` with a limit sent does not store the limit and keeps
  the stored one, so a trip through `link` and back comes home to the same
  limit.
- A placement member that is present but empty is refused on PATCH (an
  empty anchor would silently move the widget last); a requested placement
  is receipted as `placed` `{after | before | position}` in widget ids, on
  dry runs too, so a reorder preview shows the move.
- The heart port is the narrow `apicore.Widgets` adapter (§5.2), with the
  space's `CanManageSpace` / `IsReadOnly` verdicts asked up front and the
  editor's `restriction.ErrRestricted` mapped to 403 as the backstop.
- The enum vocabulary is declared in `service/widgets.go` and pinned to the
  codec by test instead of exported from any-block: no module bump needed.
- Legacy hidden listings (`_all_objects`, `_chat`) are listed and
  deletable, never creatable (Q5).
- A PATCH's layout/limit pair is planned UNDER the root's lock: the port's
  `UpdateWidget` takes a plan callback that receives the widget as stored
  at that moment, so a concurrent change of either member is validated
  against, never overwritten from a stale read; the stored layout is
  re-validated against the target whenever either member changes.
- A one-to-one space's shared root is refused (the desktop's pin gate is
  owner-or-admin, and a one-to-one space has neither); its personal root
  works as everywhere.
- Target lookup goes through the store's virtual-aware `QueryByIds`, so a
  date object, which has no persisted row, resolves like any other.

## 9. As-built summary (2026-09-21)

The sidebar surface: `GET`/`POST /v2/spaces/{space_id}/widgets` and
`PATCH`/`DELETE /v2/spaces/{space_id}/widgets/{widget_id}`. The research,
the desktop's rules and the decisions are in `APIV2_WIDGETS.md`; this
section records only what the build settled.

- **Two roots, one resource.** A space has a shared sidebar (the desktop's
  Pinned section, the derived widget object, owner/admin-only through the
  editor's space-configuration lock) and a per-account one (My Favorites,
  the `_personalWidgets_` virtual object over the tech-space store). Both
  are served as rows with `scope: space | personal`; a list carries both in
  sidebar order, space first. **`scope` is required on a create** — the two
  roots have different readers and different writers, and a default that
  403s for every editor is worse than a required word.
- **The heart port is a new narrow adapter**, `apicore.Widgets`
  (`core/api/widgetadapter.go`), not four more RPCs on `ClientCommands`: it
  reads the wrapper → link pairs off the root's children and does each
  mutation as one locked state pass and one `Apply`, the way
  `builtinobjects` installs a use case's widgets. A create with a
  placement or an update with a move is therefore one editor change, not
  two (the personal root's store still records one entry write per widget
  whose `AfterId` moved). Two invariants the service checks outside the
  lock are re-checked under it — the target is still absent, the caller may
  still write the root — because the editor's own space-configuration
  refusal fires only after the state has been merged into the live
  document.
- **The desktop's rules, stated once** (`service/widgets.go`): only live
  objects as targets — a relation, a template, an archived or deleted
  object and every built-in listing are refused on create (the app cannot
  make them either; the listings a sidebar already holds are still listed,
  editable and deletable under their `_favorite`-style spellings); one
  widget per target per root; the layout set and the limit pick-list per
  target kind copied from `U.Menu.getWidgetLayoutOptions` /
  `getWidgetLimitOptions` (a tag or option-list target keeps `tree` in its
  set the way the desktop's menu does, created as `link`; participants and
  dates are link-only and are real objects despite their `_`-prefixed
  ids); `view_id` only on a query/collection/type
  target, resolved against its views by id or unique suffix, and only on
  the space root (the desktop never writes one on a personal widget).
- **Normalise, don't refuse** (decision Q4): an off-set layout or an
  off-list limit is stored as the app's own render fallback and reported
  in `warnings`, so the stored widget and the rendered one agree. A layout
  change re-fits the stored limit to the new layout's list without a
  when the body sent none: a stored value the new list also holds stays,
  any other is re-fitted with a warning. A type target defaults to `view`
  (the desktop's own quirk creates `link` there; its menu leads with
  `view`). "Owner and admins" for the space root is the editor's
  `CanManageSpace` minus its one-to-one exemption: a one-to-one space has
  no owner or admin and the desktop never offers the pin action there, so
  the API refuses its space root too (the personal root stays available).
- **Receipts and refusals on the way**: a requested placement comes back
  as `placed` `{after | before | position}` in widget ids, on dry runs too;
  a present-but-empty placement member or layout on PATCH is refused rather
  than read as a default; a widget retargeted under the lock by heart's own
  RPC between the read and the plan is a 409, on PATCH and on DELETE alike;
  the bin widget is never moved and nothing goes after it, as on the
  desktop.
- **Addressing** (decision Q3): the path segment is a widget id first, then
  a target in either spelling; a target present in both roots without
  `?scope=` is `ambiguous_input` naming both ids, with two resend refs.
- **Vocabulary pinned to the codec**: `widgetLayoutNames` is round-tripped
  through `anyblockjson.WidgetsSnapshot` in a test, and target spellings go
  through `FormatWidgetTarget`/`WireWidgetTarget`, so the members a row
  shares with an `index.json` widget (`target`, `layout`, `limit`,
  `view_id`) carry the same spellings. A row is not an index widget
  verbatim: it also carries `id` and `scope`, which the index schema
  refuses, and personal widgets have no bundle form at all.
- **Not built, recorded**: the link child's display members
  (`card_style`, `icon_size`, `description`, `properties`) — the desktop
  never sets them; `auto_added` and the bundle's `auto_widget_*` — dead in
  heart since GO-6247; a curated wrapper tool (the external bridge gets the
  four operations from the document); the `is_favorite` steer in the
  retired typed-hints notes now has its answer in `create_widget`.
