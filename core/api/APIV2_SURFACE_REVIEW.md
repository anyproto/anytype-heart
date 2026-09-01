# API v2 — Assembled-Surface Review Triage

2026-08-07 · branch `go-7383-apiv2-phase0` · synthesis of seven independent reviews
(sweep:dialect, lens:edit, lens:grant, lens:query, lens:surfaces, lens:agent, lens:tests)
of the assembled Phase 0–7 surface + scoped keys + the layout move.
Every MUST-FIX item below was re-verified in source by the synthesizer (file:line re-read,
mechanism confirmed) before promotion; items two lenses found independently are marked
**[independent ×2]**. Tags: **[R]** = reproduced by at least one lens, **[read]** = read-only
conclusion. Verdicts on the two previously-open findings: **C4 still open** (and wider —
`setCell` strips the whole table), **E8 still open** (and wider — the entire PUT commit path
is untested, confirmed by mutation testing in two lenses independently).
*(2026-08-10: the PUT half of E8 is retired rather than fixed — APIV2.md
§8.27 removed the surface, so `ResetObject`, `preserveEditorOwnedState`
and their untested guards no longer exist. E8's PATCH half — no change-set
assertion on `sb.Apply` — stands.)*

---

## Verdict

The assembled surface is strong precisely where the phase reviews concentrated: one row
builder feeds objects/search/sets/collections; one filter codec backs both filter forms; the
scoped-key enforcement could not be defeated by a dedicated adversarial lens (route/registry
bijection exact, fail-closed branches all held under probing, JsonAPI scope denied on all
gRPC methods, no token echo, grant-narrowing eviction wired); the marks bridge round-tripped
thirteen adversarial cases exactly; batch atomicity, If-Match re-check under lock, and C8/C9
wiring are genuinely there on every mutation route.

The defects worth fixing cluster in four seams no phase review could see:

1. **The edit surface meets the restriction system**: sets and collections are entirely
   un-editable through v2 — `addItems`/`removeItems`, the only membership write, is dead —
   and every refusal in that family surfaces as a retryable 500 instead of the documented 403.
2. **Refusal classification is string-matching on unpinned middleware English**: the chat
   edit path's foreign-message refusal and every file-upload failure fall through to
   retry-looping 500s; one covering test is green against a string the code path cannot produce.
3. **The oldest surface never got the newest discipline**: Phase-2 bodies bind non-strict and
   unbounded while the served discovery schemas promise `additionalProperties:false`; the
   structured `filters` array silently compiles two malformed shapes to match-everything; the
   generated OpenAPI documents the conventions only on the newest half of the routes.
4. **The safety contracts are real but unpinned**: mutation testing showed C9 dry-run key
   drift, removal of the C8/rate-limit middleware from a write route, and the E8 Apply-flags
   revert all pass the full `./core/api/...` suite; /v2 has zero end-to-end coverage (the one
   real-account fixture constructs the server with empty `V2Deps{}`).

Nothing found crosses the grant boundary, leaks a non-granted space, or loses committed data
on the happy path. Fix section 1, pin section 2's top item, write the top five charter tests,
and Phase 8 can proceed on a surface that is honest about the rest.

---

## 1. MUST FIX before Phase 8

### M1. Sets and collections cannot be edited through v2 at all — `addItems`/`removeItems` are dead in production **[R]**

- **Where**: `core/block/restriction/object.go:43-44` (`objRestrictEdit` — which contains
  `Restrictions_Blocks` — assigned to layout `set`, and to `collection` minus TypeChange only),
  `core/api/objectmutateadapter.go:135-144` (`checkObjectEditable` refuses on Blocks OR
  Details), `core/api/objectreadadapter.go:63` (`EditRefused: checkObjectEditable(sb)`),
  `core/api/v2/service/edit.go:77-79` (returned before any op runs).
- **Failing input**: `PATCH /v2/spaces/{s}/objects/{collectionId}` with
  `{"ops":[{"op":"addItems","items":["objA"]}]}` → refused. Reproduced at the restriction
  layer by lens:edit (`GetRestrictions(...TypeKeyCollection).Object.Check(Restrictions_Blocks)`
  → `restricted: Blocks`; same for TypeKeySet; plain page nil). Verified in source by synthesis.
- **Why it matters**: `addItems`/`removeItems` is the ONLY v2 route that puts an object into an
  existing collection (router has GET reads + POST create, nothing else), and
  `APIV2_SURFACES.md` §6 explicitly retires v1's `AddObjectsToList` in its favour. A collection
  is write-once: seedable at POST (creator adapter, different path), immutable after. The same
  gate also blocks `setProperties` (renaming a set/collection) even though
  `Restrictions_Details` is NOT set on those layouts. No phase review saw it because every
  v2/service test mocks the mutator.
- **Fix**: make the gate per-op instead of per-request — check `Restrictions_Details` for
  `setProperties`/`addItems`/`removeItems` and `Restrictions_Blocks` only for block ops
  (`addItems` mutates the collection store, not blocks); or route the item ops through
  `mw.ObjectCollectionAdd/Remove` as v1 does. Add an adapter-level test driving a REAL
  collection smartblock through `addItems` (charter E4).

### M2. Permanently-refused writes surface as retryable 500s on three surfaces — agents retry-loop forever

The spec promises 403 (`APIV2.md:1278` for restrictions; the chat handler's own swagger text
for foreign edits). `RespondV2Error` (`core/api/v2/handler/error.go:19-23`) turns any
non-`*v2model.Error` into 500 `internal_error`. Three independent producers hit that fallback:

- **(a) Restriction refusals on PATCH/PUT** **[R]**: `edit.go:78` returns `cur.EditRefused`
  bare (a `fmt.Errorf` wrapping `restriction.ErrRestricted`); the mutator error path wraps in
  `mapReadError` whose fallback is `fmt.Errorf("read object %s: %w", …)`
  (`core/api/v2/service/object.go:142`) — a read-shaped 500 for a refused write. Combined with
  M1, every collection PATCH is an infinite retry loop. (lens:edit + lens:tests, independent.)
- **(b) Editing another member's chat message** **[R]**: the edit path emits
  `errors.Join(storestate.ErrValidation, "can't modify someone else's message")`
  (`core/block/editor/chatobject/chathandler.go:246`; `ErrValidation` = `"validation"`,
  `storestate/error.go:7`), surfacing as `"push change: validation\ncan't modify someone
  else's message"` — matches NONE of `v2ChatRpcError`'s arms (`"validate:"` needs the colon,
  `"not own message"` is the DELETE wording from `chathandler.go:193`) → 500
  (`core/api/v2/service/chat.go:631-645`, verified in source). **The covering test is green
  against behavior that does not exist**: `chat_test.go:482-504` feeds the DELETE string
  `"can't delete not own message"` into the EDIT path.
- **(c) Every file-upload failure** **[read, pinned by the existing test]**: `mw.FileUpload`
  has exactly one error branch, always `UNKNOWN_ERROR` (`core/file.go:152-154`);
  `UploadFile` wraps it bare (`core/api/v2/service/file.go:40-42`) → 500 for a bad URL, an
  unreachable host, an oversized file alike. `file_test.go:68` pins the raw string, not a code.
- **Fix**: produce the `*v2model.Error` where the verdict is made — map
  `restriction.ErrRestricted` → 403 in a `mapWriteError` used by PATCH/PUT; widen the chat
  forbidden arm to the edit wording (better: export the two sentinels from
  `core/block/editor/chatobject` so a rewording is a compile error) and fix the test to feed
  the real edit string; add `v2FileRpcError` with description arms like chat/space have.
  The space classifier (`space.go:246-252`) matches the same unpinned way — pin its strings too.

### M3. The structured `filters` array silently matches EVERYTHING on two malformed shapes **[R]**

- **Where**: `pkg/lib/anyblockjson/dataview.go:485-497` (`Operator != ""` → group; Property
  ignored; empty `filters` → empty AND = true), `core/api/v2/service/search.go:570-579`
  (validator takes the same branch, walks the empty child list, `continue`),
  `pkg/lib/anyblockjson/filters.go:145-147` (condition validated only when non-empty),
  `pkg/lib/database/filter.go:71-73` (`Condition_None` → filter dropped). All verified in source.
- **Failing inputs** (space with 3 objects, one matching):
  `{"filters":[{"operator":"and","property":"severity","condition":"equal","value":"High"}]}`
  → total 3, no warning. `{"filters":[{"property":"severity","value":"High"}]}` (no
  condition) → total 3. `{"filters":[{"property":"severity","conditon":"equal","value":"High"}]}`
  (typoed key) → total 3. The correct form returns 1.
- **Why it matters**: this is the exact promise the surface makes ("unresolved → did-you-mean,
  never a silent no-match", `search.go:16-17`) inverted into match-everything — and it is the
  ONE input channel with no GBNF grammar (documented C13 exception), i.e. where a 3-4B model is
  most likely to emit these shapes. The served `filters` kind schema makes shape (b) more
  likely: its leaf arm marks `condition` optional (`schemas.go:154`).
- **Fix**: harden `validateStructuredFilters` (the v2 gate, not the shared codec — stored
  dataviews legitimately carry `None`): reject a node carrying both operator/filters and
  property as ambiguous; reject a leaf with `value` but no `condition`. Share the gate with
  POST /sets (same field). Separately, `UnmarshalFilters` should refuse a group whose
  `filters` array is empty rather than emitting a match-everything AND.

### M4. Sending the Idempotency-Key that C8 mandates caps file uploads at 10 MiB with a misleading 413 **[R, independent ×2]**

- **Where**: `core/api/v2/middleware.go:193-201` (`io.ReadAll(io.LimitReader(body,
  MaxRequestBody+1))` on the RAW multipart body when a key is present; 413 above 10 MiB),
  `core/api/v2/router.go` `registerCreateRoutes` (POST `/files` carries `idempotencyMW`).
  Verified in source; reproduced independently by lens:surfaces and lens:tests.
- **Failing input**: 11 MiB multipart POST `/v2/spaces/{s}/files` — without the header → 201
  (`size:11534336`); with `Idempotency-Key: k1` → 413 `request_too_large` naming the body, not
  the header. C8 says every mutation honors the key with no exceptions, so the disciplined
  agent is exactly the caller that cannot upload >10 MiB — and the error steers it to shrink
  the file, never to drop the header. Secondary: every keyed upload ≤10 MiB is buffered whole
  in RAM and re-parsed by multipart.
- **Fix**: skip body buffering for `multipart/*` — hash method+path+query (+ Content-Length or
  a caller digest) and leave the body streaming; or move idempotency inside the upload handler
  keyed on the staged file's digest; or exempt the route and record the C8 exception in
  APIV2.md §1 (which currently claims none).

### M5. Create-missing is unbounded: one FAILING PATCH permanently created 5,000 tag options **[R]**

- **Where**: `core/api/v2/service/resolver.go:171-219` (`prewarmCreateMissing` — no cap on
  array length, no `ctx.Err()`, one `ObjectCreateRelationOption` RPC per unresolved name, no
  rollback; verified in source). The skip covers add∩set but not set∩unset — the probe never
  reads `unset` — so `{"set":{"tag":["Q3"]},"unset":["tag"]}` creates the option and then 400s.
- **Failing input**: PATCH with
  `[setProperties{set:{tag:[5000 unknown names]}}, updateBlock{id:"doesNotExist"}]` (~60 KB)
  → op 2 is a 404, the batch fails, and 5,000 real option objects exist in the space. Scaled
  to the 10 MiB cap: 10^5–10^6 permanent objects from one erroring request. v2 has no
  option-delete surface, so a hallucinated tag array (or a retry) pollutes the space
  irreversibly. APIV2.md documents the leak-on-validation-failure trade-off, but not that it
  is unbounded.
- **Fix**: cap create-missing side effects per request (a few dozen) and reject above it with
  a path-addressed error BEFORE any create RPC; honour ctx cancellation inside prewarm; extend
  the skip to any key claimed by more than one of set/unset/add/remove (mirroring `claim()`).

### M6. The five Phase-2 JSON bodies bind non-strict and unbounded, contradicting the served `additionalProperties:false` schemas **[R]**

- **Where**: `core/api/v2/handler/create.go:218,248,302,332,380` (`c.ShouldBindJSON`; gin's
  `DisallowUnknownFields` is nowhere set) vs `core/api/v2/service/schemas.go:59-90` (the served
  `property`/`set`/`collection`/`file` kinds all declare `additionalProperties:false`, plus
  name/key/url bounds nothing enforces). Both halves verified in source. The idempotency cap
  only engages when the header is present (`middleware.go:188`), so a keyless request to these
  five routes is read unbounded — the very hazard `search.go:25-30` documents as the reason
  for its own cap.
- **Failing input** (reproduced by sweep:dialect against the real binder):
  `POST /v2/spaces/{s}/properties` with
  `{"name":"Priority","format":"select","option":[{"name":"High"}],"bogus":123}` → 200-path,
  `Options` empty — the typo silently drops agent intent while GET /v2/schemas promises a
  rejection. A 1 MiB name and key `"my key!!"` are likewise accepted though the schema declares
  them invalid. Bind errors also break the C6 dialect (gin text, no `issues`).
- **Fix**: route the five handlers through `decodeStrictJSONBody` (per-surface caps for free —
  closes the unbounded-body hole in the same edit), and enforce the advertised bounds from the
  same constants the schemas serve (the space service at `space.go:90-98` is the in-repo
  precedent, comment and all).

### M7. One PATCH can hold the object lock for tens of minutes — the 512-op cap bounds the ops, not the document they inflate **[R]**

- **Where**: `core/api/v2/service/edit.go:37-40` (the cap's own comment names O(ops×document)),
  `stateops.go:189-201` (every mutating op invalidates the view; the next op re-marshals the
  WHOLE document under the smartblock lock).
- **Failing input** (reproduced by lens:edit): a 1,004,532-byte body — one `insertBlocks` of
  24,000 paragraphs + 400 trivial `replaceText` ops — spent **36.8 s** inside PatchObject;
  the same shape at 175 KB took 3.0 s (confirming the product). Extrapolated to the 10 MiB
  cap × 512 ops: ~15–20 minutes of held lock from one HTTP request, during which ObjectOpen,
  sync and every RPC on that object block. `ctx.Err()` only helps if the client disconnects.
- **Fix**: cap total blocks per PATCH (the markdown channel already has
  `v2MaxMarkdownBlocksPerOp=256`; the blocks channel has none) and/or the post-op document
  size; cheaper long-term: maintain the block index incrementally instead of re-marshalling
  per op.

---

## 2. SHOULD FIX — real but survivable

### S1. Pin the three unpinned safety contracts (C9, C8-binding, E8) — mutation testing passes today **[R, E8 independent ×2]**

Not live bugs (lens:tests probed the real applier over a real smarttest child state: change
sets are minimal and correct today), but the entire safety story is convention, and Phase 8
adds routes. Three mutations leave `go test ./core/api/...` fully green:

- **(a) C9 key drift**: `apiv2.dryRunKey = "dry_run"` (`middleware.go:260`) and
  `v2handler.v2DryRunContextKey = "dry_run"` (`handler/create.go:23`) are two independent
  literals across a package boundary (verified in source; v2handler cannot import apiv2).
  Drifting one makes **every** `?dry_run=true` a real committing write — deletes blocks,
  deletes chat messages with their irreversible attachment GC — and nothing fails.
  Fix: one shared constant (v2model or a tiny ctx package) + a router-level test: mutation
  route with `?dry_run=true` → `dry_run:true` AND the creator/mutator mock never called.
- **(b) C8/rate-limit binding**: deleting both `deps.WriteRateLimit` and `idempotencyMW` from
  `PATCH …/objects/:object_id` passes the suite. The grant conformance walk pins authz per
  route but nothing pins the middleware chain. Fix: extend the walk — for every
  `RouteVerbWrite` registry entry, send twice with one key and assert
  `Idempotency-Replayed: true`, and 409 on a mutated body.
- **(c) E8 wider than recorded**: `sb.Apply(st)` → `Apply(st, NoRestrictions, DoSnapshot)`
  green (`objectmutateadapter.go:72`); deleting `preserveEditorOwnedState`,
  `checkObjectEditable` or `guardBundledRevision` from `ResetObject` (`:86-130`) each green —
  there is no `TestResetObject` anywhere. *(2026-08-10: `ResetObject` and
  `preserveEditorOwnedState` are gone — §8.27 — so three of the four
  untested guards no longer exist; `checkObjectEditable` and
  `guardBundledRevision` survive on the PATCH path, where `TestMutateObject`
  covers both. The `sb.Apply` mutation is still green.)* Root cause verified in source: the mutator mock
  builds a fresh ROOT state via `NewDocFromSnapshot` (`edit_test.go:64-72`) where production
  hands a CHILD state, so ApplyState diffs nothing and restrictions never run in tests.
  Fix: an `expectMutateLive` fixture over smarttest + assertions on `st.GetChanges()` (no
  RelationRemove/BlockDelete/snapshot beyond what the ops named). Charter E4 is the e2e twin.

### S2. `moveBlock`/`insertBlocks` can orphan a column at the document root; normalization then destroys the 2-column layout **[R]**

`pkg/lib/anyblockjson/validate.go:565-566` checks containment one-directionally only ("a row
block can only contain column blocks" — verified; nothing requires a column's parent to be a
row). Reproduced at both layers by lens:edit: root-append
`{"op":"moveBlock","id":"colOne1"}` passes the R5 net; at the state layer the row collapses
(`normalizeLayoutRow`), the sibling column is unlinked, and a bare column persists at root —
a shape the editor never produces. No content lost; the user's layout is destroyed by an op
that asked to move one block. Fix: add the missing direction to the anyblockjson containment
check (fragment + post-op net both inherit it) and/or refuse moves that re-parent a column
outside a row. E2E: reopen such a page in the app (charter E10).

### S3. C4 is wider than recorded: `setCell` strips editor-owned block state from EVERY cell of the table **[R]**

C4 (updateBlock merges on exported JSON — `stateops.go:832-862`, "merge on the JSON shape"
comment verified in source) is recorded open; the NEW fact: `setCell` (`stateops.go:1219-1253`)
rebuilds the whole table through the format and `replaceLive` overwrites every produced block,
so an untouched cell's `Restrictions{Edit:true}` vanished in lens:edit's 2×2 repro. The C11
marshal guard cannot see it (exporter warns only on indent clamps and unmapped types). Fix:
merge non-format fields (at minimum Restrictions) onto the live block by id in
`replaceLive`/`setBlocks`, and extend the guard to fail a PATCH that would drop a field the
caller never named.

### S4. `?fields=` is validated everywhere except GET /v2/spaces/{s}/objects — the C5 canonical list **[R, independent ×2]**

`object.go:416-427` passes `fields` straight to the row builder (verified in source — no
`validateListFields` call), while POST /search 400s with did-you-mean and the set/collection
reads validate; `APIV2.md:1369-1371` states the rule as a contract. `?fields=statuss` → 200,
every row lacks the key, indistinguishable from "no value set". Fix: one call to
`s.validateListFields(spaceId, fields)` after `ensureSpace`; add the case to the guard test
pinning the other three entry points.

### S5. The shared pagination middleware answers in gin's envelope, pre-auth, and is lax and strict at once **[R]**

`core/api/pagination/pagination.go:24-46` (verified): `limit=0`/`limit=1001` →
`{"error":"limit must be between 1 and 1000"}` — no status/code/issues, unparseable by a C6
client; `limit=abc`/`offset=-1` silently coerce to defaults; and it is the FIRST middleware in
the /v2 group (`router.go:69-76`), ahead of Auth — so an unauthenticated caller gets 400 on a
real route vs 404 on a non-route, a route-existence oracle and a hole in the "every /v2 route
401s a credential-less request" property. Fix: a v2 pagination middleware (or an OnError hook)
emitting `v2model.ValidationFailed` with `Issue{Path:"limit"}`, rejecting non-numeric/negative
values the same way, installed AFTER `deps.Auth`.

### S6. `ensureSpace` lacked the liveness predicate — a deleted/left space stayed fully addressable on ~20 routes **[R, resolved 2026-08-31]**

`service.go:99-113` checks only `GetSpaceViewDetails` existence (verified in source);
`isLiveSpaceView` guards exactly three call sites (`discovery.go:58`, `space.go:79`,
`search.go:768` — verified by grep). Reproduced by lens:grant through the real engine: a
granted space with `spaceAccountStatus=SpaceDeleted` → `GET /v2/spaces` empty,
`GET /v2/spaces/{id}` 404, but `…/types`, `…/objects`, `…/chats`, `POST …/search` all 200 —
and `SpaceIndex(spaceDead)` was minted as a side effect, the very thing the guard's doc comment
exists to prevent. Writes failed deep with 5xx instead of the clean 404. Resolved by moving the
liveness check into `ensureSpace` (hence `ensureSpaceWrite`), making the predicate one choke point;
the four quarantined nested-list routes have direct regressions and declare the shared 404.

### S7. Every /v2 request — including unauthenticated ones — warms v1's account-wide caches that v2 never reads **[R]**

`router.go:75-76`: `deps.CacheInit` before `deps.Auth` (verified in source). Reproduced:
`GET /v2/spaces` with NO auth header → 401, but `crossSpaceSubService.Subscribe` already ran —
four cross-space subscriptions over EVERY space, pre-credential. `V2Service` is constructed
without the v1 service (`service.go:50`), so the caches are pure cost. Fix: drop `CacheInit`
from the /v2 group.

### S8. Discovery artifacts on the primary generation target are self-inconsistent (ops schemas) **[R]**

Two verified-in-source defects on `/v2/schemas/ops/{op}`, the surface built for constrained
decoding: **(a)** all 10 ops serve a single-op schema (`opSchema` — `additionalProperties:false`,
required op fields) with an ENVELOPE example (`{"ops":[…]}`, `schemas_ops.go:67-148`) — the
example fails its own schema (lens:agent validated all 22 kinds: the 10 ops are the only
failures); an agent constraining on the schema emits a bare op and gets a 400 whose hint talks
about If-Match. **(b)** C13 break: `columns`/`rows` in the shared `$defs/block`
(`schemas_ops.go:65-66`) and `set.views` (`schemas.go:75`) are arrays with NO `items` — the
repo's own search-kind test states why that poisons strict decoders, but covers one kind.
Also: `updateBlock.set` has no `additionalProperties` (unrecorded C13 exception), and the
collection/addItems/removeItems examples carry truncated `bafyreieqh63jv…` ids with a literal
Unicode ellipsis. Fix: wrap the op schemas in the `{"ops":[…]}` envelope; give the three
arrays `items`; hoist the strictness assertion into a loop over ALL kinds and ops; add
example-validates-against-own-schema to `schemas_ops_test.go`; full-length synthetic ids.
(Related decision on the `space` kind: D5.)

### S9. The generated OpenAPI is a two-dialect document — the conventions its preamble promises are declared only on the newest half **[read, independent ×2]**

Verified against handlers + the generated document by two lenses with matching counts:
Idempotency-Key declared on 8 of 21 mutations (missing from all 13 Phase-2/3 writes incl.
PATCH/PUT objects); `dry_run` missing on PATCH/DELETE types and properties though all four
honor it; offset/limit missing on 5 paginated lists; the five typed DTOs
(CreateProperty/UpdateProperty/CreateSet/CreateCollection/UploadFile) exist but bodies render
as bare `{"type":"object"}`; POST /files documents neither the `file` form field nor the JSON
`{url}` mode; `SchemaKindV2Handler`'s description lists 9 kinds of 16 served; the brand-new
scope/grant 403s (`space_not_granted`/`write_not_granted`, WWW-Authenticate, two different
envelopes) appear nowhere but whoami. A generated client loses retry-safety, dry-run and
paging affordances on exactly the oldest routes — C12 broken at the parameter level. Fix: one
annotation pass + `make openapi`; then close the drift channel — add
`make openapi && git diff --exit-code core/api/docs` to the CI codegen step
(`.github/workflows/test.yml` runs `make generate` only — verified claim by lens:tests) and a
test asserting embedded-document paths == engine route table.

### S10. Global search: the materialization bound guards `offset` but `need = offset+limit` is what fans out **[read]**

`search.go:821,845`: `runSearchQuery(space.id, plan, 0, need)` per space with `need` up to
3000 while `maxGlobalSearchOffset=2000` checks offset alone — 50% past the bound on every
granted space simultaneously (~234k detail structs on a 78-space account). Fix: bound
`offset+limit`, or a per-space fetch cap reported through C11 warnings.

### S11. Full-text `total` is a moving lower bound asserted as fact in the C10 steering message **[R]**

The lower-bound mechanics are documented (`APIV2.md:1421-1424`); NEW: `search.go:125-139`
renders it as "26 matches — showing 25…" and the number grows per page (26 → 51 → 76 over 120
real matches), so "total/limit" planning under-fetches ~5×, and nothing on the wire
distinguishes exact from clipped. Fix: "at least %d matches" + a `total_is_lower_bound` flag
or C11 warning when `len(all) == offset+limit+1`.

### S12. The idempotency store is keyed by (space, key) — never by credential **[R, independent ×2]**

`middleware.go:219-221` (verified: `store.begin(spaceId, key)`, hash covers method/path/query/
body, never the caller). Two credentials sharing a client-chosen key collide: the second
caller's write silently never executes and it receives the first caller's stored body with
`Idempotency-Replayed: true`. Contained today (the grant gate runs before replay, so no
cross-space leak), and the shipped wrapper mints random keys — but deterministic third-party
keys ("create-doc-1", a date) are a natural pattern. Fix: fold the key id (already on the ctx
for whoami) into `idempotencyStoreKey`. One line.

### S13. Chat edit-path merge holes: blocks-composed messages and replyTo **[R]**

Three small verified defects in `EditChatMessage` (`chat.go:267-281`): **(a)** text-clearing
is refused when `len(Attachments)==0` even though chatmodel accepts blocks-only messages —
v2's own read shows the message non-empty (`blocksText`) while v2 refuses to edit it to that
state; fix: include `len(existing.Blocks)` in the emptiness test. **(b)** the edited proto
never carries `ReplyToMessageId` — the reply survives ONLY because `storeObject.EditMessage`
modifies the content key alone (`chatobject.go:583`, verified in source), a middleware detail
v2 does not control and no test asserts; fix: one line + assert in the merge test. **(c)**
`replyTo` is the one reference AddChatMessage never resolves — a dangling id is accepted
silently; fix: resolve via `getChatMessageProto` like edit/delete/reaction do.

### S14. Edit-surface polish: empty-list residue and delete-then-recreate **[R; (a) resolved 2026-08-31]**

**(a)** `setProperties` remove of the last entry left `key: []` instead of unsetting
(`stateops.go` formerly used unconditional `SetDetail`): present-but-empty ≠ absent in the
§3 presence contract. Resolved with `RemoveDetail` when a non-empty list is consumed; a
regression distinguishes that result from intentional `set: []`. **(b)** reusing a block id
deleted earlier in the same batch is rejected as "duplicate … already exists" —
`checkFreshIds` tests `a.st.Exists` (`stateops.go:453`, verified) and `deleteBlock` only
unlinks; the natural delete-and-recreate pattern fails with an actively wrong message. Fix:
track ids unlinked by this PATCH as free.

### S15. Small dialect nits (four one-liners) **[read]**

- Oversized POST /v2/validate → 400 `validation_failed` instead of 413 `request_too_large`
  (`handler/validate.go:35-37`) — the one surface off the shared code.
- `?outline=1`/`True` silently coerces to false (`handler/object.go:37` string-compare) while
  `dry_run` 400s on the same input — use the tri-state parse.
- C8 replay drops the ETag header (`middleware.go:249-254` stores status/CT/body only); body
  etag survives. Store and restore the header.
- Member role `no_permissions` is the lone unrecorded snake_case enum in a v2 body
  (`service/discovery.go:164`) — see D4.

---

## 3. DECISIONS, not fixes

### D1. If-Match on type/property mutations: honor it or stop advertising it

GET types/{t} emits ETag + envelope etag; PATCH returns a fresh etag — the full C7 vocabulary —
yet none of the four type/property mutation handlers reads If-Match (verified: zero `If-Match`
occurrences in `handler/create.go`) and no exemption is recorded (chats: APIV2.md:1713,
spaces: APIV2.md:2043 — types/properties absent). Stale-etag PATCH → 200 last-write-wins.
**Recommendation**: honor it (compare against `ComputeEtag` of the object's heads, 409 on
mismatch) — these surfaces already ship every other half of C7; recording an exemption on a
surface that actively emits etags would be the confusing choice.

### D2. Set/collection reads: apply the base row scope or record the exemption

`list_read.go:206-260` never appends the layouts/no-template/no-hidden triple that both
SearchObjects and ListObjects apply — same space, same type: set read returns 4 rows (incl. a
hidden object and a relation row), search returns 2 **[R]**. v1 behaves identically, so it is
a v2-internal divergence, not a regression; APIV2.md:1340 records the base scope as a property
of "the query surface" without noting the opt-out. **Recommendation**: append
`appendBaseRowScope` in `listObjects` (with a documented exception when the stored view itself
filters on isHidden/layout); if instead the raw behavior is wanted, say so in §8.4 and both
route descriptions — today code and spec disagree silently.

### D3. `GlobalAuthExempt` is the single door out of the fail-closed registry — make it self-limiting before Phase 8

No current defect (the walk verifies the class behaviorally; verified fail-closed everywhere
else). But the procedure for shipping an unauthenticated /v2 route is: register outside the
group + classify auth-exempt, and CI stays green — and Phase 8's file byte-download and chat
SSE stream are exactly the streaming routes someone registers outside the gin group.
**Recommendation**: pin the auth-exempt set to a literal allowlist (the two docs paths) and
make the conformance walk FAIL on any new route carrying the class, so a third is a reviewed
edit, not a passing test.

### D4. One write-rate budget for all credentials, and one snake_case role

The write limiter keys on RemoteAddr — always 127.0.0.1 — so all keys share 1 write/s burst 60
(`server/router.go:24-25`, `server/middleware.go:329-338`); the multi-key scoped model is
precisely several agents at once, and one bulk edit starves the rest ("the API randomly
429s"). **Recommendation**: key on the session/key id (resolved before the per-route limiter
runs). Separately `no_permissions` (S15): rename to `none` now while v2 is unreleased, or
record the carve-out next to dry_run/has_more.

### D5. The `space` kind describes two contracts with one schema

`schemas.go:103-108`: `required:["name"]` but the endpoint string says PATCH takes the same
fields "both optional — at least one" — a model generating a PATCH under the schema can never
change only the description, and minProperties:1 is expressible in neither.
**Recommendation**: split `space` / `spaceUpdate` (or drop `required`, add `minProperties:1`).

---

## 4. ACCEPT / RECORD

- **C4 remains open as recorded** (updateBlock merges on exported JSON, `stateops.go:832-862`
  verified unchanged): Restrictions, exotic Fields kinds and int64 precision vanish on the
  touched block. S3 records the new blast radius. Record both in APIV2.md until fixed.
- **Global search is an N+1 over spaces for any non-bare query** (~300 store queries on a
  78-space account: knownPropertyKeys + aliases + type resolution per space,
  `search.go:179-232`), and the merge comparator is built from the FIRST space's plan
  (`:843,862`) — under a shadowed alias (`size`→`sizeInBytes` active in one space only) the
  comparator reads a key other spaces' rows don't carry and their rows fall to the id
  tiebreak; `_final_score` is likewise compared across independent BM25 indexes. Record; cache
  per-fan-out and per-origin translation are the eventual fixes.
- **Collection reads with no stored sort materialize the whole membership per page**
  (`list_read.go:233-241`, deliberate for the honest total): O(n²/limit) to page a 50k-member
  collection to the end. Record the cost in APIV2.md §8.4; bound or approximate past a size
  threshold when it bites.
- **Multipart staging has no size cap and writes the body to disk twice**
  (`handler/create.go:405-443`; shared with v1; authenticated, localhost). Also `?dry_run=true`
  stages the whole file before "uploaded nothing". Record; `http.MaxBytesReader` + advertised
  cap + dry-run short-circuit when touched next.
- **POST …/read answers an unqualified 200 even when it marked zero messages** (stale
  lastStateId → strict subset marked; `markedCount` is discarded at
  `core/block/chats/service.go:811-829` and the RPC has no field). Not fixable in v2 alone —
  record on the endpoint description; plumb `marked` through the RPC when the middleware is
  next touched.
- **The C8 store is a 1024-entry in-process LRU** — replay protection has an eviction horizon
  and does not survive restart. By design; record next to C8.
- **/v2 has zero end-to-end coverage**: `tests/integration/chat_test.go:107` constructs the
  server with empty `V2Deps{}` so `RegisterRoutes` returns at line one. This is the
  precondition for the whole charter below — recorded here so it is never mistaken for tested.
- **Verified-good, for the record**: route/registry bijection exact both directions; empty
  `:space_id`, case/whitespace/percent-encoding variants all refused; JsonAPI scope denied on
  all gRPC methods (no gRPC escape from a scoped key); no token echo anywhere; ListSpaces/
  spaceRefs/whoami intersect the INPUT set (totals cannot leak); marks bridge exact on 13
  adversarial cases; batch atomicity, in-batch id addressing, If-Match re-check under lock,
  duplicate-id detection, table pinning, unlink sweeps all correct; SKILL.md's filter examples
  all parse and are in the served GBNF; the wrapper's 60s dedup claim matches the code.

---

## 5. E2E CHARTER (ranked)

Ranked by risk × invisibility-to-mocks. E1 is the precondition for E2–E10.

1. **E1 — The real-account /v2 fixture.** Extract the chat_test bootstrap
   (`tests/integration/chat_test.go`) into a shared helper and construct the server with real
   `V2Deps` (needs exported test constructors for the three package-private adapters in
   `core/api`). Why no mock can substitute: every /v2 test today is service-vs-mock or
   engine-vs-mocked-services; no HTTP request has ever hit /v2 backed by a real account, store,
   or CRDT tree.
2. **E2 — C9 dry-run really does not write.** For PATCH objects (deleteBlock+insertBlocks),
   POST objects, POST types, DELETE properties, DELETE chat message: send `?dry_run=true`,
   assert 200 + `dry_run:true`, re-read over HTTP, assert the store byte-identical. Why:
   the dry-run key-drift mutation (S1a) passes the entire suite — nothing today proves the
   flag reaches the handlers; the blast radius of a silent flip is irreversible deletion.
3. **E3 — C8 idempotency across a real retry.** POST objects with key K → 201 + id; resend
   byte-identical → same id, `Idempotency-Replayed: true`, and a search proves ONE object
   exists (the count is the real assertion); mutated body under K → 409; PATCH retry appears
   once; two identical keyed POSTs concurrently from two goroutines → one object. Why: the
   middleware-deletion mutation (S1b) is invisible to the suite; the begin/pending reservation
   has only stub-handler tests.
4. **E4 — A PATCH that lands in the CRDT, with change-set assertions, surviving evict+reopen —
   plus `addItems` on a REAL collection.** Create from markdown, PATCH a batch covering every
   op kind, assert diffStats; capture the emitted change set (no RelationRemove/BlockDelete/
   snapshot beyond the ops — closes E8; the ResetObject guards it also named went away with
   PUT, §8.27); restart
   against the same repoDir and re-GET (title/featured row/custom relation intact, etag moved).
   Drive `addItems`/`removeItems` on a real collection smartblock — the ONE test that would
   have caught M1. Why: the mutator mock builds a root state; ApplyState diffs nothing in
   tests, ever.
5. **E5 — A scoped key minted the REAL way.** Second space via `WorkspaceCreate`; key via
   `AccountLocalLinkCreateApp{Scope: JsonAPI, Grant:{Spaces:[A], Perm: Read}}`; assert:
   GET spaces/B → 403 `space_not_granted` + WWW-Authenticate; POST to A → 403
   `write_not_granted`; GET /v2/spaces lists only A; global search over [A,B] returns only A;
   tech space → 403; whoami echoes [A]/read; the key on /v1 refused; then narrow the grant
   mid-session (`LinkLocalUpdateApp`) and assert the very next request is refused. Why: every
   current enforcement test hand-builds `ApiSessionEntry` — the sealed-app-link →
   `WalletCreateSession` → `util.ApiGrant` conversion has zero end-to-end coverage.
6. **E6 — Two accounts in one shared space: foreign chat edit/delete.** Account B PATCHes and
   DELETEs A's message; assert 403 with the C6 envelope on BOTH. Why: only a real second
   identity produces the middleware's actual "can't modify someone else's message" string —
   the mocked test fed the wrong string and stayed green (M2b).
7. **E7 — File upload, both modes, both sizes.** >10 MiB multipart with and without
   Idempotency-Key (pins the M4 fix); bad URL via JSON mode (pins M2c's 4xx); attach the
   uploaded file to a chat message immediately and read back — the attachment kind is inferred
   from the file object's asynchronously-indexed layout, so a fast attach may downgrade
   image→link (mock-invisible race).
8. **E8 — If-Match against a genuinely concurrent writer.** GET etag; mutate the object
   out-of-band via gRPC (`BlockTextSetText`); stale If-Match → 412 carrying the current etag;
   no header → succeeds; refreshed → succeeds. Why: `EtagMatches` is tested against string
   literals — nothing proves the etag derives from heads that move when the tree does; the
   spec predicts 409/412 noise under sync, and only live contention shows the rate.
9. **E9 — Chat round-trip byte-identity.** Send bold/mention/link markup, GET, PATCH the
   returned text verbatim, GET: text/marks/replyTo/style/attachments/blocks byte-identical.
   Why: the marks bridge is unit-exact, but nothing exercises the real store's
   marshal/unmarshal in the loop — where replyTo and Style survive only by accident (S13b).
   Include a cold-process first GET (chatState/lastStateId populated with no prior open) and
   cursor paging while another device writes.
10. **E10 — Query surface at real scale + renderer round-trip.** (a) FT search past the
    engine's 2000-doc candidate limit: does has_more terminate at the true end? (b) global
    search on a many-space account at offset=2000&limit=1000: wall time, peak heap, sane
    ranking (S10 + the comparator hazard). (c) page a several-thousand-row set/collection to
    the end on lastModifiedDate ties — duplicates/gaps only show against real data. (d) reopen
    a page after the S2 orphan-column move in the desktop client to see what the renderer does.
11. **E11 — The document and schemas against real consumers.** Generate a client from
    `docs/v2/openapi.json` (openapi-generator) and run the four SKILL.md walks — the missing
    parameters (S9) are only felt there; feed the served op schemas to OpenAI strict mode and
    llama.cpp's json-schema-to-grammar (S8's vendor half); fetch /v2/schemas/* over HTTP and
    diff against service-layer output. Plus the §10.1 v1↔v2 conformance test: nothing e2e
    asserts either served surface matches its generated spec.

---

## 6. What no reviewer covered

- **Anything requiring a live account** — the single biggest gap, and the charter's reason:
  real CRDT change-set contents under sync, concurrent PATCH-vs-editor merges, cache
  eviction/reopen, wallet-minted grants, two-device space deletion mid-session, real file
  bytes.
- **Whether opening an object on a read-classified route can commit a migration change to the
  tree** — lens:grant explicitly declined to evaluate the smartblock Init path. If it can, a
  read-only grant performs writes as a side effect. Worth a targeted look before Phase 8.
- **The wrapper/CLI and eval harness against the shipped surface** — SKILL.md was statically
  checked (all examples parse), but no lens ran the 12 tools end-to-end, with or without a
  scoped key.
- **Vendor acceptance of the served schemas** (OpenAI strict, llama.cpp GBNF conversion) and
  **generated-SDK usability** — reviewed at the JSON-Schema-rule level only (E11).
- **HTTP-level serialization of the discovery payloads** — validated in-process, not the
  `json.RawMessage` the handler emits.
- **Scale**: global-search memory at fan-out, FT beyond the candidate cap, 50k-member
  collections — all read-only extrapolations.
- **SSE and file byte-download** — Phase 8, does not exist; D3 is the guardrail to land first.
- **Origin/host check and the analytics middleware** — no lens probed them.

## Process note

During lens:grant's run a concurrent agent briefly mutated this worktree
(`core/api/v2/router.go` had `ensureSpaceGrant()` removed and restored; two stray
`zzprobe_test.go` files appeared under core/api/v2/service and core/block/restriction). All
lens findings were observed with the gate intact, and the tree was verified clean
(`git status` empty) at synthesis start and end. If any finding above seems to contradict
HEAD, re-check against `aa52f3b9f` before acting on it.
