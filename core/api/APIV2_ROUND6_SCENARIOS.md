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

## R6-4 — there is no cross-space object reference of any kind **[audit 2/2]**

S9 task 6 asks to link an observation in one space to an experiment in another.
Both actors found the same wall by three independent routes each and were
refused every time. Objects cannot reference objects across spaces.

That may be a deliberate boundary — but nothing says so. Both actors spent
calls discovering it by failure. If it is permanent, an error naming it as a
boundary would end the search in one call.

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
6. **R6-4, R6-10** — say no by name: cross-space references, and which type key
   a create is about to use.
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
