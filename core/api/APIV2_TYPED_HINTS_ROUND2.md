# Typed hints, round two — findings and work order

Status: **measured, not fixed.** Everything below is reproduced against a live
build; each finding carries the command that shows it. Companion to
[`APIV2_TYPED_HINTS.md`](APIV2_TYPED_HINTS.md), which is the design brief this
round tested.

Build under test: `dffcdb5e8` (heart) + `8592284` (anytype-mcp, packed local
tarball). Server on `127.0.0.1:31009`, `Anytype-Version: 2025-11-08`.

## What was measured

Three sonnet actors, ten plant tasks, one fresh space each, no self-assessment;
an opus agent then judged from the raw jsonl tool calls. Prompt byte-identical
to the pre-typed-hints baseline apart from space names, so the two rounds
compare directly.

| | this round | baseline `d34ca3ed6` |
|---|---|---|
| tasks achieved | **30/30** | 27/30 |
| anytype tool calls | **162** | 175 |
| error responses | **11** | 2 |
| responses carrying `see_also` | **1** | n/a |
| typed-hint follow rate | **0%** (0 followed / 1 ignored) | n/a |

**Do not read 30/30 as a win for typed hints.** Three sonnet actors improvising
differ by a few calls and a few 400s on temperature alone. The error delta is
fully explained by causes the feature does not touch (see F6). The honest
finding is the last row: the mechanism fired **once in 162 calls**, and that
once was an actor's own synthetic probe (`totally_made_up_test_prop_xyz`).

**The mechanism is sound where it fires. It is wired to almost nothing.** That
is the whole result, and F1 is the whole fix.

---

## F1 — `see_also` is absent from the errors that actually happen

**Priority 0.** The design brief assumed the reference would ride along on
repair-shaped errors. It rides on one branch. Every error below is one a real
actor hit, none carried `see_also`, and each has an obvious referent.

| error | occurrences | has `see_also`? | the referent it should carry |
|---|---|---|---|
| `formatVersion is required` | 6 | no | `get_schema {"kind":"object"}` |
| `did you mean is_favorite?` | 1 | **no** | `list_properties` — *same error class as the one that does* |
| `unknown key "properties" — the PATCH body carries only ops` | 1 | no | `get_op_schema {"op":"set_properties"}` |
| filter `parse error at offset 19` | 1 | no | `get_schema {"kind":"filters"}` |
| option colour not in enum | 1 | no | `get_schema {"kind":"type"}` |

The second row is the sharpest: `unknown property key` **does** attach
`see_also` when it falls through to the list-all branch, and **drops it** when
it finds a near-match and says "did you mean". The did-you-mean branch is the
one a confused caller hits more often.

Work: enumerate every `Issue` construction site that has a natural referent and
attach one. The existing `Ref*` constructors already cover most of them.

```bash
# the did-you-mean branch, with no see_also
curl -s -X PATCH -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"ops":[{"op":"set_properties","set":{"Favorite":true}}]}' \
  "$API/v2/spaces/$SPACE/objects/$OID?dry_run=true"
```

---

## F2 — twelve raw HTTP routes still leak, through the schema-prose channel

**Priority 0**, and the guard test's blind spot: it covers `Hint:` strings but
not schema `description:` strings, which carry the same repair advice to the
same callers. Full inventory from the live server (7 schema kinds + 17 op
schemas probed):

| source | description field | the route |
|---|---|---|
| `schemas/type` | `property_definitions` | `PATCH /v2/spaces/{space_id}/types/{type}`, `PATCH /v2/spaces/{space_id}/properties/{key}` |
| `ops/add_property` | `property` | `GET /v2/spaces/{space_id}/properties`, `PATCH …/properties/{key}` |
| `ops/remove_property` | `property` | `GET /v2/spaces/{space_id}/properties` |
| `ops/move_property` | `property` | `GET /v2/spaces/{space_id}/properties` |
| `ops/insert_view`, `ops/update_view` | `set.filter`, `set.filters` | `GET /v2/schemas/filters` (×4) |
| `ops/insert_blocks`, `update_block`, `replace_subtree`, `set_cell` | `$defs/block` | `GET /v2/schemas/object` (×4) |

The first row matters most: it is the single sentence tasks 3 and 10 depend on,
and it names two routes an MCP caller cannot issue.

These need the same treatment as hints — a structured referent the wrapper can
re-spell — or, where the advice is short, inlining. Extend the guard test to
the description channel in the same change, or it regrows.

```bash
# reproduce the inventory
for k in type object filters; do curl -s -H "$AUTH" "$API/v2/schemas/$k"; done \
  | grep -o '"description":"[^"]*/v2/[^"]*"'
```

---

## F3 — `dry_run` is implemented everywhere and declared on 18 of 26 mutators

**Priority 0, and the only data-loss risk in this document.**

The C9 middleware honours `?dry_run=true` globally — **verified**. The OpenAPI
declares it on 18 mutating operations and omits it on three that implement it:

- `update_property` (PATCH)
- `delete_property` (DELETE)
- `delete_type` (DELETE)

The generated MCP wrapper mirrors the declaration, so it **drops the argument
before the request is sent** and the write commits for real. The response is
shape-identical to a genuine write — no `"dry_run":true` echo — so a caller
cannot tell a rehearsal from a commit.

This is not hypothetical: it is how Garden Alpha's task-10 rename landed. The
actor sent `dry_run:"true"`, the wrapper dropped it, the rename committed. It
got the right end state only because the rename was what it wanted anyway.

For `delete_type` this is a rehearsal that deletes.

Fix: add the `dry_run` query annotation to those three handlers, `make openapi`,
re-run the prose tests. Then consider the general rule — **never silently drop
an argument a caller sent.** An undeclared argument should 400, not vanish.

```bash
# server honours it (type survives, response echoes dry_run:true)
curl -s -X DELETE -H "$AUTH" "$API/v2/spaces/$SPACE/types/$KEY?dry_run=true"
curl -s -o /dev/null -w '%{http_code}\n' -H "$AUTH" "$API/v2/spaces/$SPACE/types/$KEY"   # 200
# but the tool schema has no dry_run parameter
```

---

## F4 — the one `see_also` that fired was not usable verbatim

**Priority 1.** The single reference emitted in 162 calls carried two entries:

```json
"see_also":[
 {"op":"list_properties","params":{"space_id":"h7ii3i"},"tool":"API-list-properties","args":{"space_id":"h7ii3i"}},
 {"op":"create_property","params":{"space_id":"h7ii3i"},"tool":"API-create-property","args":{"space_id":"h7ii3i"}}]
```

`API-create-property` requires `space_id` **and** `body`. Passed through as
supplied it is rejected for a missing required argument.

If the contract is "these args are ready to send", it must hold — a partial
payload is worse than none, because it invites a call that fails. Either fill
required arguments or mark the reference as needing completion. Decide which,
and state it in the `Ref` schema description, which currently promises neither.

---

## F5 — served prose names affordances the caller does not have

**Priority 1.** Same disease as F2, different channel. `list_properties`
declares exactly one parameter, `space_id`. Its own success message says:

> `"43 matches — showing 25 from offset 0; request the next offset"`

There is no declared `offset`. The server honours one anyway (verified:
`?offset=25` pages correctly), so the wrapper cannot expose it and the caller
must guess an undeclared argument — which is exactly what the sonnet actor did.

And the `see_also` hint from F4 says *"list all with API-list-properties"*. It
cannot list all: 25 of 43, no declared paging.

Fix: declare `limit`/`offset` on `list_properties` (and audit the other list
endpoints for the same gap), or stop telling callers to page.

---

## F6 — `formatVersion is required` does not state the value, and the fix already exists

**Priority 1**, and the cheapest win here: **6 of this round's 11 errors.**

```json
{"path":"/formatVersion","message":"formatVersion is required"}
```

No hint, no `see_also`, and it never prints the one legal value. The adjacent
branch — same endpoint, one field different — gets it right:

```json
{"path":"/body","message":"unknown key \"body\" — the shortcut accepts type, name, properties, markdown",
 "hint":"to send a full AnyBlock document, include \"formatVersion\":\"2.0\""}
```

Copy that hint onto the required-field branch. `formatVersion` has exactly one
legal value (`{"const":"2.0"}`); an error that withholds it is a riddle.

---

## F7 — the If-Match hint misfires on unknown PATCH body keys

**Priority 2.** Reproduced verbatim:

```json
{"path":"/properties","message":"unknown key \"properties\" — the PATCH body carries only ops",
 "hint":"the If-Match precondition is a header, not a body field"}
```

The message is correct and useful. The hint is about `If-Match`, which the
caller never sent — a canned hint attached to the wrong branch. It is served on
every unknown body key. Already noted as out of scope in the original brief;
it is now observed in the wild, costing an actor a call.

Delete it, or make it fire only when the key actually is `If-Match`.

---

## F8 — warnings carry no `see_also`, and what they do carry is noise

**Priority 2.** 6 responses carried `warnings`, 20 entries in total, `see_also`
present in **zero**. The brief already flagged that `Hint` is rendered only on
the error path (`client.go:208`); this confirms the encoding gap too.

All 20 entries were the same string, served on every `get-type` / `get-object`:

> `"lastOpenedDate" cannot be a legend value (import refuses the internal keys export strips), so its spelling "last_opened_date" is not written; the stored key is its own address`

A user reading their own type gets three paragraphs about export legends. This
is backup-layer vocabulary leaking into the apiv2 layer — the three-layer
identity split says it should not be here at all. Worth its own look.

---

## F9 — writes echo nothing a caller can check

**Priority 2.** `patch_object` returns `diff_stats`, and it is the single best
affordance in this API: task 6 ("change ONLY this sentence") was clean in all
three runs precisely because the server proved the constraint held —
`{"blocks_added":0,"blocks_removed":0,"blocks_changed":1}`.

Nothing else does this. `update_property` → `{"id","key"}`: no name, no etag.
`create_collection` → `{"id","type","etag"}`: no item count, and no run in the
round ever verified its collection actually contained the two objects.

Give mutating endpoints an echo of what changed. It converts "I think it
worked" into "the server says it worked" and removes a read-back call.

---

## F10 — `is_favorite` is undiscoverable, and cost 25 calls

**Priority 2 (product), but it dominated one task.** "Put it somewhere I'll see
every morning" cost Alpha 10 calls, Beta 8, Gamma 7. `is_favorite` appears in
no schema, no tool description and no example; `set_properties` accepts it and
`get-object` echoes it, but it does not appear in `list_properties`. Two of
three actors found it only by 400-ing into the did-you-mean.

Note for scoring any future round: **writing it and reading back `true` proves
persistence, not function.** It is a dead flag — there is no favourite/pin
mechanism behind it. Either implement it, surface it, or reject it on write.
Silently accepting a write that does nothing is the worst of the three.

---

## F11 — every write tool declares `body` with no schema and no description

**Priority 1 for call cost.** `{"type":"object","additionalProperties":true}`,
no description, on every mutating tool. Consequence: **34 of 162 calls (21%)
were schema fetches** before any work happened, plus the six F6 failures.

This is the finding with the most leverage on the embedded-wrapper decision in
[`APIV2_WRAPPER_IN_HEART.md`](APIV2_WRAPPER_IN_HEART.md): a curated tool with
flat arguments has no envelope to get wrong and no schema to fetch. Feed this
number into that research.

---

## The guard test

It asserts nothing route-shaped survives onto the wrapper surface, and for
`Hint` strings it holds. It does not cover:

1. schema `description` fields (F2, 12 live leaks)
2. success-path `message` fields (F5)
3. whether a `see_also`'s `args` are actually sufficient for the named tool (F4)

Extend it to all three in the same change, otherwise each regrows independently.

---

## Ranked work order

1. **F3** — declare `dry_run` on the three mutators; adopt "never silently drop
   an argument". Data-loss risk, smallest diff.
2. **F1** — attach `see_also` to the five error branches that actually fire,
   starting with did-you-mean.
3. **F2** — structured referents for the 12 schema-prose routes; extend the
   guard test to the description channel.
4. **F6** — copy the existing `formatVersion` hint one branch over. 6 of 11 errors.
5. **F5** — declare `limit`/`offset` on `list_properties`; stop promising paging
   that is not exposed.
6. **F4** — decide and document whether `see_also.args` are send-ready.
7. **F7, F8, F9, F10, F11** — as scoped above.

## What held up

Worth knowing, so nobody re-litigates it:

- **The type op channel works.** `add_property` used 4 times across 3 runs,
  **zero** `property_definitions` re-sends, all five fields intact on read-back.
  That was the wipe it was built to prevent.
- **Property propagation works, on the harder case.** The task-3 property
  appeared as a column in the `default` view that was minted in task 1, *before*
  the property existed — no reopen, no view edit.
- **`diff_stats` works** (F9), and is the model for the rest.

## Caveat on the evidence

Sonnet's baseline was already 27/30, so this round had almost no headroom and
cannot show an improvement even if one exists. Typed hints were built for
callers that cannot make the REST→tool inference; sonnet makes it silently. The
haiku round — where the baseline was 12/30 with 119 errors and 80 of 212 calls
spent guessing one op's shape — is the discriminating test. Its result appends
here.
