# Round 6 — the remaining six scenarios

Status: **measured, not fixed.** S1, S3, S5, S7, S9, S10 against build
`78a737bbb`. 12 sonnet actors, 6 opus auditors, zero timeouts and zero 5xx in
any scenario — nothing here is a contention artifact.

**With this round every one of the 45 MCP tools and all 17 ops has been
exercised at least once.**

**[repro]** = reproduced against the live server myself, command given.
**[audit]** = an auditor's finding from the transcripts, not independently re-run.

---

# Priority 0

## R6-1 — a view accepts a date filter that search refuses, then matches nothing **[repro]**

The same structured date filter, two surfaces, opposite behaviour:

```
POST /v2/spaces/{sp}/search     {"filters":[{"property":"close_date","condition":"greater_or_equal","value":"2026-07-01"}, …]}
  -> 400  'property "…" is a date — the structured form takes unix seconds (1782864000), not "2026-07-01"'

POST /v2/spaces/{sp}/queries    {"views":[{"filters":[ …the identical two filters… ]}]}
  -> 201, stored verbatim, and the view returns total:0 of 2 deals that should match
```

`search` validates date filter values and explains the fix precisely. The view
path accepts the same value, stores it, and silently yields an empty view.

This is S7's headline in the wild: an actor built "this quarter's deals,
biggest first", read the definition back correctly, saw zero rows, and ended
the run believing the task was done. **The correct error text already exists —
it is simply not applied on the view path.** Apply `search`'s date-value
validation to `insert_view`/`update_view` filters and this whole class closes.

## R6-2 — `default_template` is stored and never applied **[repro]**

```
POST   /v2/spaces/{sp}/templates            -> 201, template id, 2 blocks
PATCH  /v2/spaces/{sp}/types/memo {default_template:<id>}  -> 200, etag moves
GET    /v2/spaces/{sp}/types/memo           -> default_template stored: YES
POST   /v2/spaces/{sp}/objects {type:"memo"} -> new object has 0 blocks
```

Accepted, persisted, correctly read back, no effect. There is **no `template`
parameter on `create_object`** either (its schema has only `body`, `dry_run`,
`create_missing_options`, `space_id`), and neither the object nor the type
schema says what `default_template` does at create time.

So **creating an object from a template is not achievable through this API**,
while the API presents every appearance of supporting it. One actor spent five
calls discovering this; the other never tested it and reported success.

This is the "silently wrong" pattern in its subtlest form: a 200, a moved etag,
a correct read-back, and no effect.

---

# Priority 1

## R6-3 — `request_key` exists on the wrong operations, and is undiscoverable **[repro]**

| surface | result |
|---|---|
| served OpenAPI | **0 occurrences** of `request_key` |
| MCP tool schemas | **9 tools** — `add-chat-message`, `create-chat`, `delete-chat-message`, `edit-chat-message`, `read-chat`, `toggle-chat-reaction`, `create-space`, `update-space`, `validate` |
| `create-object`, `patch-object`, `create-type`, `create-property`, `create-collection`, `create-query` | **none** |

Two problems. **Idempotency is offered where duplicates barely matter (chat
reactions, space renames) and withheld from the bulk-write paths where they
do.** S10 task 7 asks an actor to re-run a twelve-record load by mistake; there
is no retry key on `create_object` to prevent it.

And a REST caller cannot discover the feature at all: it is absent from the
served document, so it exists only for MCP callers.

*(Correction: my own brief for this scenario claimed `create-object` declares
`request_key`. It does not — the auditor caught the error.)*

## R6-4 — cross-space links work and the docs say they do not **[repro]**

**Corrected 2026-09-19. The original finding — "no cross-space object reference
of any kind" — was wrong**, and the evidence was sitting in one of the actors'
own objects. What is true is more interesting.

A cross-space deep link **works**. Lab B's *Nitrogen Fixation Rate Trial* holds:

```
Related field observations (Field Notes B): [Soil Nitrogen Levels — East Field]
(anytype://object?objectId=bafyreiaxzmto2f…&spaceId=bafyreiecu…tyc7ze.1nzm3nx8rq2hb)
```

The served documentation describes only half of it. From `schemas/object`,
the sole mention of the scheme anywhere on the surface:

> `` `[text](anytype://object?objectId=<id>)` links to an object in this space ``

**`spaceId` is documented nowhere** — 0 occurrences across 8 schema kinds and 6
op schemas — and the sentence explicitly scopes the link to *this space*. A
caller who reads the documentation carefully concludes cross-space linking is
impossible. That is precisely what the other actor concluded after being
refused by three typed-relation routes, and it is what the audit reported.

Actor B extrapolated `&spaceId=` past the documentation and was right.

So the defect is not a missing capability, it is **documentation that denies a
capability the product has**. Three things to fix:

1. Document `spaceId`, and drop "in this space".
2. Say which space-id spelling it takes. The actor used the **full**
   `<cid>.<replicationKey>` form; the API serves the compact form (`tyc7ze`)
   nearly everywhere else, and whether compact is accepted here is untested.
3. Note what a typed relation cannot do, since three relation-shaped routes
   refuse cross-space targets while the deep link allows them. A caller needs
   to know the link is the mechanism.

**Methodology note.** Neither the actor nor the auditor was at fault; the
auditor reported what the transcript's failures showed, and nothing in the
run contradicted it except one object's stored text. I relayed it without
checking the stored state. The lesson for future rounds: **when an audit
reports a capability absent, read the end state, not only the calls** — a
success that happened by an unexpected route leaves no failing call behind.

## R6-5 — a type's dataview views have no read path for their rows **[audit]**

`get-query-objects` requires a `query_id`. A view created on a **type's** own
dataview therefore has no operation that returns its rows. One S7 actor put its
views on the type (entirely legal, and the other actor's choice of a query
object was equally legal) and **could not verify any of them** — not because it
failed to try, but because no such read exists.

Two callers, two legal designs, and only one of them is observable.

## R6-6 — table structural edits are not expressible **[audit 2/2]**

`set_cell` works well (see below), but inserting a row mid-table and moving a
column are not operations. Both actors achieved both tasks by resending the
**entire table** through `replace_subtree`. That works, and it is the pattern
`property_definitions` replacement was moved away from for good reasons: it is
a whole-object write for a one-cell change, and any slip loses the table.

## R6-7 — an error names a property by hex where the caller sent a slug **[repro]**

```
sent:    {"property":"close_date", …}
error:   'property "6aadcde161fab2f86565765c" is a date — the structured form takes unix seconds …'
```

The F11 / R3-f family, still live on this branch. The message is otherwise
excellent — it states the expected form and gives a worked value. It just names
the property by an id the caller never used and cannot look up.

---

# Priority 2

## R6-8 — `dry_run` receipts describe nothing **[audit 4/4]**

`{"type":"deal","dry_run":true}` is the whole response. For an `insert_view`
dry run it does not even say that a view would be created. Four actors used
dry runs as their standard practice and none of them learned anything from one.
Compare `diff_stats` on a real write, which is the best affordance in this API.
A dry run should return the `diff_stats` it would produce.

## R6-9 — global search under-reports immediately after a write **[audit]**

An S9 actor's `search-global` 19 seconds after creating an object returned 1 of
2 matching rows; a later identical call returned both. Index freshness, not
scope — `search-space` against the same data was correct. Worth documenting a
consistency expectation, since agents write-then-search constantly.

## R6-10 — one unhelpful 400 forked two actors onto incompatible schemas **[audit]**

`create_type {"name":"Recipe"}` → 400. One actor recovered by creating a
throwaway object to install the **bundled** `recipe` type, inheriting four
fields nobody asked for and a name it never noticed was wrong ("Time", not
"Prep time"). The other supplied `api_key:"kitchen_recipe"` and got a clean
custom type. Same request, two incompatible data models, traceable to one
error message that did not say which road it was pushing the caller down.

## R6-11 — a type-level `add_property` does not reach the type's templates **[audit]**

Adding `rating` to the type put it in `property_definitions`, the dataview and
the default view's columns — all verified. The **template object** for that
type does not receive it. Nothing in the `update_type` response mentions
templates.

## R6-12 — views still omit the property they are organised by **[audit, confirms R5-9]**

A kanban grouped by `stage` has columns `owner, value, close_date` — no
`stage`. Confirmed again on a query's views, having first been seen on a
collection's. R5-8 (sort `direction` not echoed) also reproduced here.

---

# What works — verified, do not re-litigate

- **Tables and `set_cell` are clean.** S5 had **zero errors, zero 4xx, zero
  no-op writes**, every etag distinct and every `diff_stats` matching the next
  read. `set_cell` addresses cells by header name (`row:"Catering"`,
  `col:"Estimate"`), which is friendlier than its id-based example suggests.
- **`move_block`, `delete_block`, `replace_subtree` all work**, with
  `diff_stats` proving the "change only this" constraint each time. One actor's
  `replace_subtree` round-tripped 103 words verbatim.
- **The file round trip works.** Both actors passed `upload-file`'s returned id
  straight into a `files` property on the first try — no translation step.
  `download-file` returns path, filename, media_type and size.
- **`search-global` works across spaces and carries `space_id`** in compact
  form on every row, resolvable against `list-spaces`. Space-scoped search
  returns a strict subset with the same ids, and the compact id lifted from a
  global result is accepted verbatim — no spelling conversion. *This was
  expected to be a trap and is not.*
- **`add_property` reaches the board**, verified again: `property_definitions`,
  the dataview and the default view's columns, all three, with existing
  instances byte-identical before and after.

---

# Ranked work order

1. **R6-1** — apply search's date-filter validation to view filters. A silent
   empty view is the worst failure mode here and the fix text already exists.
2. **R6-2** — make `default_template` work on `create_object`, or refuse to
   store it and say templates must be instantiated another way.
3. **R6-3** — put `request_key` on the write paths that need it, and get it
   into the served document.
4. **R6-5** — a read path for a type dataview's rows, or a warning when a view
   is created somewhere it cannot be read.
5. **R6-7** — the hex-for-slug leak in filter errors.
6. **R6-4** — document `spaceId` on `anytype://object`, and stop saying the
   link is same-space only. **R6-10** — say which type key a create will use.
7. **R6-8** — dry runs should return the `diff_stats` they would produce.
8. **R6-6, R6-9, R6-11, R6-12** — as scoped above.

# Coverage

All 45 tools and all 17 ops now exercised across six rounds. Sub-claims still
open, each needing a task written for it:

- a collection view carrying a **filter**, asked for membership (R5 caveat);
- `delete_property` on a property still attached to a live type (R5-6);
- the bundled-type half of `delete_object`'s receipt spelling (R4-2);
- naming a removed type's slug in a create or filter after `delete_type` (R4-1
  sub-claim C).

# Status

**R6-7 — done.** Search canonicalizes every filter leaf's property to its
stored key before validating it, so a refusal quoted that key — for a
space-minted property a 24-hex id the caller never sent and cannot look up,
inside an otherwise exemplary message. `keyCanon` now records the caller's
spelling at its one choke point (`canonOrErr`, which every filter, sort and
view rewrite passes through) and `spelledAs` gives it back, so each refusal
raised AFTER canonicalization quotes what the caller wrote: the date-value
refusal and its worked example, the option-name refusal and its
`list_property_options` reference, the unknown-key refusal, and the
missing-condition refusal. A display name sent under `?keys=name` comes back
as that display name. The reference stays usable because the property route
resolves a stored key, a slug and a display name alike. The compact filter
string was already correct: the parser reports offsets into the caller's own
text.

**R6-1 — done.** A stored filter takes a date the way the compact string
does: on `POST /queries` (a view's filters and the top-level filters) and on
`update_view` / `insert_view` (the structured channel and, since the review,
the compact `filter` string too), a string value on a date property that is
RFC 3339 or `YYYY-MM-DD` is converted to unix seconds before the filter is
stored (`convertDateFilterValues`), and a string that is no date is refused,
path-addressed. The conversion runs before key canonicalization on both
routes, so the refusal preserves the caller's own property spelling instead
of substituting the stored key. Two leaf kinds are left alone: a presence
predicate (`empty`, `not_empty`, `exists`), whose value the canonical form
discards anyway, and a leaf carrying a `date_preset`, whose value belongs to
the preset — a counting preset takes a DAY COUNT, so converting
"1970-01-01T00:00:07Z" to 7 would silently read as "seven days ago". The
format lookup follows the same spellings key canonicalization accepts,
including a slug a read emitted for a REMOVED property, which resolves to
that property's stored key rather than to a live property sharing the
spelling as a display name.

Search's structured form is unchanged: it still refuses the string with the
conversion spelled out. Later iteration: teach search the same conversion.
The conversion is property-format-aware, so a text property whose value
looks like a date is never touched and the ambiguity may not arise; should
it, an explicit form on the pattern of SQL's `DATE(2026-01-26)` is the
fallback. The `filters` schema kind and APIV2.md say where a date string is
accepted, and the refusal's hint names `date_preset` with an example.

Remaining scope, recorded rather than fixed: a filter inside a whole
document (`POST /objects` or a template carrying a dataview) and the
generic block payloads (`insert_blocks`, `replace_subtree`,
`update_block.set.views`, a dataview inside `set_cell`) import through the
codec directly and are NOT converted, so a date string persists there as
before; `insert_view.copy_from` reproduces its source's filters verbatim,
including a bad value already stored. Nothing migrates filters written
before this change. Found on the way and fixed: the canonical-form strip
matched only the camelCase `notEmpty`, so a `not_empty` leaf kept its value.
