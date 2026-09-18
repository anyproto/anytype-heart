# Type ops — design

Status: implemented. `core/api/v2/service/typeops.go` is the channel,
`core/block/editor/template/collection.go` the view prune, and
`core/api/v2/service/typeops_test.go` the tests. The four open questions at the
bottom are answered in place.

`PATCH /v2/spaces/{space_id}/types/{type}` takes a whole-type body. There is
now an op channel beside it, so an incremental edit — the common case — is no
longer expressible only as a full-document rewrite.

## Why

A whole-document PATCH cannot carry intent. `property_definitions: [X]` is
ambiguous between *make X the only field* and *add X*, and the endpoint has to
pick one. It picks replace; callers read it as add.

That is not a hypothetical. Three agents ran the same ten-task benchmark
independently, in separate spaces, against the same build. To add one field to
a type that already had four, all three sent:

```json
{"body": {"property_definitions": [
  {"name": "Harvest Season", "format": "select", "options": [...]}
]}}
```

A one-element array, not an empty one. All three lost the other four fields.
All three then made a second call re-sending the complete list. Two of them ran
on a gutted type for a dozen calls before noticing.

One called `GET /types/{key}` specifically to check, and was reassured — because
a type document carries **two** property lists that drift apart:

| list | on replace |
|---|---|
| `type_settings.property_definitions` | replaced — drops what you omit |
| `blocks[].dataview.properties` + view columns | append-only — gains new, never prunes removed |

So immediately after the destructive write, the dataview half still listed all
four original fields while the definitions half held one. Verification that is
*correct* still passed.

An op names its intent, which is what lets the implementation do the consequent
bookkeeping — update the definitions **and** the views, in the direction the op
implies. That is the structural argument for ops here, not ergonomics.

Every other edit on this API is already an op with a targeting vocabulary
(`PATCH /objects` takes `set_properties`, `insert_blocks`, `update_view`, …).
Types are the one resource still doing whole-document PATCH, and the one
resource with this failure mode.

## Shape

The existing ops envelope, on the type resource:

```json
PATCH /v2/spaces/{space_id}/types/{type}
{"ops": [
  {"op": "add_property", "property": "Harvest Season", "format": "select",
   "section": "featured", "options": [{"name": "Early summer"}]},
  {"op": "move_property", "property": "harvest_season", "after": "location"},
  {"op": "remove_property", "property": "sun_needs"}
]}
```

Same rules as the object op channel: 1..512 ops, applied in order as one atomic
edit, `?dry_run=true` previews exactly what the real run does, and each op is
addressed by the same `ops[i].field` pointer in errors.

### `add_property`

| field | |
|---|---|
| `property` | required — api key or display name, resolved by the existing chain |
| `format` | required when the name does not already exist; ignored when it does |
| `section` | `featured` / `hidden`, default unsectioned |
| `options` | select and multi_select only; needs `?create_missing_options=true` |
| `after` / `before` / `position` | optional placement, same vocabulary as the block ops |

Idempotent: adding a property the type already lists moves it if placement is
given, and is otherwise a no-op that reports nothing.

Also appends to the dataview's properties and to every view's columns — the
behaviour that already exists today for the add direction.

### `remove_property`

Takes `property`. Detaches it from the type's recommended lists **and prunes it
from the views** — the half that was missing, and the reason a removed field
kept showing as a column.

Removing a property that the type does not list is an error, not a silent
no-op: a caller who misspells the key should hear about it.

This never deletes the property object itself. The property stays in the space
and keeps any values already stored on objects; only the type's declaration of
it goes away. `DELETE /properties/{key}` remains the way to remove the property
itself.

### `move_property`

Takes `property` plus exactly one of `after` / `before` / `position`. Reorders
the definitions list; `position: "first"` is the "make this the primary field"
verb, mirroring `move_view`.

## What the whole-type body keeps

The flat body stays, unchanged, for create and for genuine whole-shape updates.
It works well — all three benchmark agents used it correctly on the first try,
including `default_view` on create. The op channel is for incremental edits, not
a replacement.

`property_definitions` in the body keeps its replace semantics. Replace is
genuinely needed for reordering and bulk removal, and flipping the meaning of an
existing member would silently change behaviour for anyone already relying on
it. What changed instead (implemented, see below) is that a replace now says
what it took away.

## Already implemented, independent of this proposal

A type update that detaches properties now reports them, in both channels:

```json
{"id": "...", "key": "plant",
 "removed": {"properties": [{"key": "location", "name": "Location", "format": "text"},
                            {"key": "sun_needs", "name": "Sun Needs", "format": "select"}]},
 "warnings": [{"path": "/type_settings/property_definitions",
               "message": "property_definitions REPLACES the type's field list: 2 no longer listed (location, sun_needs)",
               "hint": "send the complete list to keep a field, or omit property_definitions entirely to leave the list untouched"}]}
```

`CreateResult.Removed` is `Created`'s twin — a removal is never inferable from
the request alone, since the caller sent what they wanted rather than what they
dropped. A dry run reports it identically.

The served schema node for `property_definitions` also states the replace rule;
it previously had no description at all, while the correct sentence existed only
in the swagger annotation, where a caller never looks.

## Answers to the open questions

1. **The op channel lives on `PATCH /types/{type}`.** Not on `PATCH
   /objects/{id}`. The type resource speaks api keys; the object channel
   speaks object ids and already covers the type's blocks and views. Two
   endpoints therefore accept an `ops` envelope with disjoint op sets, which
   the code keeps honest in one place: `parseOpsEnvelope` decodes both, the
   two op-name lists stay separate, and the schema route serves the union.
2. **`add_property` mints an unknown name**, exactly as the body does, and
   reports it in `created.properties` through the same resolver. `format` is
   required when nothing answers to the name, because that is the case where
   the op creates something: guessing a format would make the difference
   between a text field and a select field a silent default.
3. **`remove_property` prunes the property from every view's columns**, not
   only the default one, symmetric with the existing add-direction sync. A
   view that uses the property as its `group_by`, in a sort or in a filter is
   left entirely as it is and named in a warning — dropping the column out
   from under the arrangement would leave a board grouped by something nobody
   can see. Both halves are reported.
4. **No ops for `layout`, `plural_name`, `default_view`, `default_template`
   or `icon`.** Those are single-valued and stay on the body. Ops for the
   list, body for everything else.

## What the design did not settle

The doc treats the type's field list as one list. It is **four**:
`recommendedFeaturedRelations`, `recommendedRelations`,
`recommendedFileRelations`, `recommendedHiddenRelations`, which is what
`section` names. Only order *within* a section is stored, so:

- `after` and `before` name an anchor in the same section. Across sections the
  write would change nothing at all, so it is refused with both sections named
  rather than silently doing nothing.
- `position: "first"` makes the property the leading field **of its section**.
- `section` is `"featured"`, `"hidden"`, or `null` for the plain field list.
  Stating it on a property the type already lists moves it between the lists,
  which is the only verb that does.
- `file` is not authorable, and stating a section on a property that sits
  there is **refused**. Moving one out would be one-way — no op puts it back —
  and a request this API cannot undo is worse than a refusal.

`move_property` therefore reorders; `add_property` re-sections.

## Related

- `core/api/v2/service/typeops.go` — the op channel
- `core/api/v2/service/schema_write.go` — `UpdateType`, which routes to it
- `core/api/v2/service/typeoptions.go` — `detachedProperties` (the removal
  report, shared with the body path) and the declared-option guards
- `core/api/v2/service/typeshortcut.go` — the flat body
- `core/api/v2/service/schemas_ops.go` — the three served op schemas
- `core/block/editor/template/collection.go` — `ReconcileTypeDataviewColumns`
  and its removal direction, `PruneTypeDataviewColumns`
- `core/api/v2/model/model.go` — `CreateResult.Removed`
