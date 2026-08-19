# API v2 eval findings — go-7383-apiv2-phase0

## Status: 223 → 29 failing subtests (two commits, 87% reduction)

Round 2 (commit `07e8bfa43`) found the same gap one level down: block TYPE
names (`heading1`→`heading_1`, `bulletedListItem`→`bulleted_list_item`,
`toggleHeading1-3`→`toggle_heading_1-3`), already migrated in
`pkg/lib/anyblockjson`'s enum, still spelled the old way in `object.go`,
two served-schema examples, and 5 test files. One confirmed functional
regression in that round: `object.go`'s `outlineHeadingTypes` map — the one
deciding whether a block's text shows up in a `?outline=true` read — was
still keyed on the old spellings, so it matched nothing and outline reads
silently stopped showing heading text for every object. Fixed. **170 → 29
failing subtests** from this round alone.

**Remaining 29**, not yet fixed, roughly three buckets:
- Chat/object mention-tag rendering (`TestChatMessageFromProto`,
  `TestV2GetObjectIdShapes`) — test expects `<mention objectId="...">`,
  the renderer already emits `<mention object_id="...">`. Deliberately not
  touched: this is the inline-markup GRAMMAR (an XML-style attribute inside
  markdown text), not a JSON field — a bigger, more product-visible decision
  than a struct-tag rename, and out of scope without explicit sign-off.
- `TestViewOpReviewFixes` (3) and part of `TestV2SearchPlanConvergence`/
  `TestV2SearchFileLayoutOptIn` — look adjacent to the `filterstring.go`
  boundary already flagged above (out of scope: shared library).
- `TestPatchObject` (`set_cell`×4, `insert_blocks_inside_a_leaf_block`,
  `set_properties_add_on_a_scalar_format`), `TestPatchPayloadIdsResolve`
  (4), `TestPatchReportsMintedNestedIds` (2), `TestV2CreateSet` (2),
  `TestUpdateViewOp` (1) — not traced yet; didn't show an obvious single
  shared root cause in a first pass the way the last two rounds did.

## Status (round 1): 223 → 172 failing subtests

`go test ./core/api/v2/...` on HEAD (before anything in this doc) had **223
failing subtests** — hidden until now because piping the run through
`tail`/`head` swallows `go test`'s real exit code; run it unpiped to see the
true result. Root cause, confirmed directly from failure messages: HEAD's own
merge commit — `88bf52ffe GO-7383 Merge go-7383-anyblockjson: the format's
vocabulary is snake_case` — moved the shared AnyBlock schema/library to
snake_case, but `core/api/v2/service`'s OWN vocabulary (the `kind` dispatch,
block property names, view fields, sort/filter fields, the type/template
envelope, and a large fraction of the test suite's own fixture bodies) was
never updated to match.

**Fixed in this pass, verified by `git stash` A/B and a full test run each
time — 223 → 172 (−51 subtests, 24%), zero new failures introduced:**

- `kind` (`objectType` → `object_type`) — the original bug #1 below
- `template_for`, `type_properties` — the type/template envelope
- 13 view-set fields (`group_by`, `cover_property`, `end_property`,
  `hide_icon`, `card_size`, `cover_fit`, `colored_groups`, `page_size`,
  `default_template_id`, `default_type_id`, `wrap_content`, `list_size`,
  `alternate_rows`) plus sort-item fields (`custom_order`, `include_time`,
  `empty_placement`, `no_collate`) and the column aggregation enum
  (`count_value`, `count_distinct`, `count_empty`, `count_not_empty`,
  `percent_empty`, `percent_not_empty`)
- block schema fields: `object_id`, `icon_emoji`, `icon_image`, `card_style`,
  `background_color`
- `date_preset` (search/sort probes)
- property format `multi_select` (was `multiSelect` — a caller could never
  successfully create a multi-select property through the documented value)
- the structured `filters` kind's `condition` enum in `schemas.go` — was
  documenting `notEqual`/`greaterOrEqual`/etc. when the real validator
  (`pkg/lib/anyblockjson`) already only accepts the snake_case forms; a
  documentation-only fix, the real validator was already correct
- 2 production bugs this uncovered along the way, not just doc/schema
  strings: **`keycanon.go`**'s view-key canonicalizer was still keyed on
  `view["groupBy"]`, so it silently stopped canonicalizing a view's group-by
  property-key value the moment callers sent the (now-correct) `group_by`
  field name; **`keys.go`**'s envelope canonicalization loop was still keyed
  on `"templateFor"`, so a template's target-type key spelling silently
  stopped resolving too. Both fixed to match.
- ~11 test files' own fixtures updated to the corrected spelling, plus one
  stale test (`schemas_ops_test.go`'s `KnownBlockProperty` assertions) that
  was asserting the *old* camelCase names were known and the new snake_case
  ones weren't — backwards from current reality, flipped to match

**Deliberately NOT touched — a different package, different blast radius:**
the compact filter string compiler, `pkg/lib/anyblockjson/filterstring/filterstring.go`,
still *emits* camelCase condition tokens (`notEqual`, `greaterOrEqual`,
`notEmpty`, …) that its own downstream validator (also in `anyblockjson`)
rejects — this is the root cause of confirmed bug #3 below. Fixing it means
editing the shared AnyBlock library itself, which has consumers beyond v2 —
out of scope for a v2-focused pass without separate sign-off.
`core/api/v2/service/resolver.go:347` and `viewops.go:596` (`case "empty",
"notEmpty", "exists":`) were deliberately left alone for the same reason:
they correctly match what the compiler *currently* emits, and "fixing" them
in isolation would break value-stripping instead of fixing anything.

**~172 failing subtests remain**, spanning patch ops, locators, search,
chat, and more (`TestPatchObject`, `TestV2SearchObjects`,
`TestChatMessageFromProto`, `TestUpdateViewOp`, …). Spot-checked a few:
they pre-date this pass (confirmed present in the original 223, e.g.
`TestChatMessageFromProto` lives in `core/api/v2/model`, a package this pass
never touched) and look like further instances of the same casing split
(e.g. chat mention tags: test expects `<mention objectId=...>`, renderer
already emits `<mention object_id=...>`) plus at least one block-type enum
issue (`/blocks/0/type: value must be one of 'paragraph', ...`) not yet
traced. Not chased further in this pass — flagging for the team to scope
as its own effort.

For reference, the exhaustive struct-tag sweep of `core/api/v2` production
code that started this — every remaining camelCase `json:` struct tag in the
package, cross-checked against `pkg/lib/anyblockjson`'s own (already
snake_case) canonical spelling for the same field:

| our tag (camelCase) | anyblockjson canonical | where |
|---|---|---|
| `templateFor` | `template_for` | `service/create.go:34` |
| `typeProperties` | `type_properties` | `service/create.go:37`, `service/schema_write.go:441` |
| `groupBy` | `group_by` | `service/list_create.go:253` — bug #2 above |
| `customOrder` | `custom_order` | `service/resolver.go:380`, `service/list_create.go:249` |
| `datePreset` | `date_preset` | `service/search.go:609` |
| `includeTime` | `include_time` | `service/list_create.go:246` |

**All six — plus everything else in the "Fixed in this pass" list above —
are now fixed.** `templateFor`'s earlier "not reproduced" verdict in this
doc was wrong: `pkg/lib/anyblockjson` already spoke `template_for`
end-to-end (its own validator's error message is literally `template_for is
only valid on templates (type "template")` — the exact message the original
probe hit), and the real bug was `docEnvelope.TemplateFor`'s stale
`json:"templateFor"` tag never binding a spec-correct request.
`typeProperties` was worse — **silent**: a spec-correct `type_properties`
array just failed to bind (no error), so `CreateType`'s
create-missing-properties feature quietly did nothing.

---


Run: 13 parallel Haiku subagents drove 122 live HTTP calls against every route
in `core/api/v2/router.go`, against `http://127.0.0.1:31009`, space `3ovz6u`
("API eval"). Every finding below was independently re-reproduced by hand
with `curl` before being kept — see [Retracted](#retracted-not-real-bugs) for
what didn't survive that pass.

Result: 122 calls, 115 pass, 3 confirmed bugs, 1 unresolved lead, 4 retracted.

All three confirmed bugs share one root cause: the snake_case migration this
branch is doing landed unevenly — one validator got updated, the validator
next to it didn't, and the two now disagree about what a request is allowed
to say. In every case that makes the feature **completely unreachable**, not
just inconvenienced — there is no request body that satisfies both sides.

---

## Confirmed bugs

### 1. Type creation is unreachable — no value of `kind` satisfies both validators

**Severity:** High · `POST /v2/spaces/{id}/types`

```
# kind: "objectType" (camelCase) — the value the endpoint's own schema example uses
curl -X POST /v2/spaces/{id}/types -d '{"version":1,"kind":"objectType",...}'
→ 400 "/kind value must be one of 'object_type', 'bundled_object_type', ..."

# kind: "object_type" (snake_case) — the value that error just asked for
curl -X POST /v2/spaces/{id}/types -d '{"kind":"object_type",...}'
→ 400 "POST types accepts kind \"objectType\" documents only"
```

A dispatch check runs first and hard-requires the old camelCase spelling; the
generic AnyBlock document validator runs second and only accepts the new
snake_case enum. Each value clears one gate and fails the other.

**Root cause:**
- `core/api/v2/service/schema_write.go:111` — `kind != "objectType"` hardcoded pre-check, never updated
- `pkg/lib/anyblockjson/json.go` — the AnyBlock `kind` enum, already snake_case

---

### 2. `insert_view` can't create a grouped/kanban view — `groupBy` vs `group_by`

**Severity:** High · `PATCH .../objects/{id}` op `insert_view`

```
# groupBy — what the op's own error message says is allowed
{"op":"insert_view","set":{"type":"kanban","groupBy":"status"}}
→ 400 "additional properties 'groupBy' not allowed" (document-level check)

# group_by — what the resulting view object actually stores (confirmed via GET)
{"op":"insert_view","set":{"type":"kanban","group_by":"status"}}
→ 400 "unknown view field \"group_by\" — allowed: ...groupBy..." (op-level check)
```

The op-level validator's own allow-list names `groupBy` as correct; the
document-level check that runs on the result rejects exactly that spelling.
Neither order works — a view can be created (confirmed working without a
group-by), but never with `group_by` set at creation time.

**Root cause:**
- `core/api/v2/service/keycanon.go:168` canonicalizes view fields as `groupBy`
- the other side of the check — wherever `/blocks/0/views/N` gets validated
  against the object document schema — still expects the snake_case form

---

### 3. Compact filter `IS NOT EMPTY` — documented, rejected by its own validator

**Severity:** High · `POST .../search`, compact `filter` string

The exact example from `core/api/v2/SKILL.md` and from
`GET /v2/schemas/filters`'s own `grammar_examples`:

```
curl -X POST /v2/spaces/{id}/search -d '{"filter":"assignee IS NOT EMPTY"}'
→ 400 unknown condition "notEmpty" — allowed: ...not_empty...
```

The grammar compiler turns `IS NOT EMPTY` into the internal token `notEmpty`
(camelCase); the very next validation step only accepts `not_empty`
(snake_case) from its own allow-list. A syntactically perfect,
spec-documented filter can never pass.

**Root cause:**
- `core/api/v2/service/resolver.go:344` and `core/api/v2/service/schemas.go:156`
  both still spell the value-less conditions `notEmpty`
- `core/api/v2/service/viewops.go:583-596` carries the same spelling into a
  second code path

---

## Unresolved lead (not confirmed)

### Compact filter: `due_date < today()` combined with `AND` drops a matching row

**Severity:** Medium (unconfirmed cause) · `POST .../search`, compact `filter` string

`name CONTAINS "QA Search" AND due_date < today()` returned only the object
with **no** `due_date` (the one the response's own warning says the
comparison spuriously includes) — an object whose `due_date` was genuinely in
the past did not come back. The equivalent structured `filters` array
returns all matching rows correctly, so the gap is specific to compiling a
compact `date < …` leaf combined with another leaf via `AND`. Reproduced
directly but not traced to a line — worth chasing.

---

## Retracted (not real bugs)

Flagged by an agent, didn't survive a second, careful pass:

- **"Template creation expects camelCase `templateFor`"** — not reproduced;
  `template_for` (snake_case) is accepted as a field name once the rest of
  the body is well-formed. The original 400 was a cascading error from an
  unrelated malformed body (a stray top-level `name`), not a casing
  rejection.
- **"Option-ID-shaped value accepted as a new option name"** —
  `GET .../properties/{key}/options` exposes only `name` (and `color`), no
  `id` field at all. The test used a made-up ID-shaped string with no real
  option behind it, so create-missing correctly treated it as a new name.
  There's no way to reach a *real* option ID through this endpoint to test
  the guide's actual warning.
- **"`match` substring specificity" in block locators** — the agent flagged
  this and then correctly explained its own mistake in the same breath:
  `match:"Draft timeline"` is a substring of three different blocks' text,
  so the API's refusal was exactly per spec.
- Duplicate log of the above, from the same task.

---

## What passed clean (115 / 122 calls)

Every other endpoint behaved exactly as `core/api/v2/SKILL.md` describes,
including the negative paths that are *supposed* to fail: idempotency replay
+ conflict, `If-Match` etag mismatch, atomic PATCH-batch rejection with
untouched state on failure, did-you-mean hints, the recursive-delete guard,
ambiguous `match`/`id` refusals, offset-pagination rejection on chats, and
the own-output-only 403 on deleting a foreign object. Full per-task call log
is in the workflow transcript if needed — this file only carries the
problems.
