# API v2 — the plan of outstanding work

Status: plan v1.0 · 2026-08-09 · GO-7383, branch `go-7383-apiv2-phase0`.

This is the **index**, not a replacement for the specs. Work is specified in
`APIV2.md` (the API spec + §8.x as-built notes), `APIV2_SURFACES.md` (the
remaining-surfaces decisions and phases), `ADDRESSING.md` (the identifier
layer), `APIV2_TOKENS.md` (the measured token review) and
`APIV2_SURFACE_REVIEW.md` (the whole-surface audit). Each item below says
where it lives.

Ordering is by **dependency and evidence**, not by size. Waves 0–1 are the
ones with measured numbers or a live defect behind them.

---

## Built and green (for orientation)

Phases 0–7 — read, create, edit (batched id-addressed ops; the
full-document PUT shipped and was **removed** — §8.27), query with
the compact filter DSL, the task-tool wrapper (12 tools, CLI, MCP stdio, two
model tiers), chats, the space periphery; the view family
(`updateView`/`insertView`/`moveView`/`deleteView`); scoped API keys with the
fail-closed route registry; the layout split into `core/api/v2/` with two
OpenAPI documents; the strategy-(a) identity layer (BSON mint + `apiObjectKey`
slug, corpse policy, resolution chain). Each carries an `APIV2.md` §8.x
as-built section and has been through an opus review round.

---

## Wave 0 — cheap, measured, no design risk

Ship these first. Both are trivial, both pay on **every** read, neither has a
dependency or an open question.

**Both landed 2026-08-09** — APIV2.md §8.24 and §8.25.

| # | item | value | where |
|---|---|---|---|
| 0.1 | ~~Compact the embedded envelope values~~ **DONE** — fixed in `encodeEnvelope` (the serving layer); the canonical form is untouched and no golden moved | predicted 16–26 %, **measured −15.5…−26.4 %, corpus −23.2 %** | TOKENS §1.1, action 1; §8.24 |
| 0.2 | ~~Split `?ids=`~~ **DONE + HARDENED** — two shapes: `compact` (edit) and `full` (export); after the three-lens review, **only machine-minted ids relabel** (24-hex bson, view UUIDs — meaningful ids keep their spelling and are reserved), and the legend left the export shape too | legend removal **0.9–11.5 % on the measured corpus** (confirmed); block labels **bimodal**, −19…−22 % on minted-id documents and **0 %** on meaningful-id documents — by rule now, not charset accident; not the flat ~15 % the review assumed | TOKENS §1.2 + §10, action 2; §8.25 + §8.26 |

`edit` (the default read) is: **short labels for minted block ids, full
inline object refs, no pins**. `full`/export is **full ids everywhere**
(no legend on any shape — §8.26; pins remain unshipped), because block
relabelling is lossy. Combined, against the actual served bytes:
**−33.1 % across the corpus**.

Closed by the hardening (§8.26): PUT **refused** a body carrying ids the
object does not own (it used to silently adopt served labels as stored
ids); `?block=` subtree reads are marked partial and no write path accepts
them; create strips the read envelope and warns on label-shaped ids; the
wrapper's client-side relabeling retired. **§8.27 then removed PUT
entirely**, which closes the same trap by construction — no channel takes
block ids literally any more — and retires the item once owed to 2.1
("teach PUT the suffix resolution the ops already use"). `?ids=full` is
now framed as the backup/export shape, not as a write-back read.

## Wave 1 — finish the identity layer (one item fixes a live defect)

The slug surface shipped; the platform half did not, and the gap is not
cosmetic.

| # | item | why now | where |
|---|---|---|---|
| 1.1 | **Heart-side mint hardening** — `injectApiObjectKey` checks nothing for UI creates | prerequisite for 1.2/1.3; v2 currently defends only at resolution | ADDRESSING §7.5 req 1; APIV2.md §8.22 |
| 1.2 | **Backfill `apiObjectKey` for old spaces** (`systemobjectreviser` never sets it) | **live silent wrong-entity write**: in a pre-`apiObjectKey` space a UI property named "Due Date" claims `due_date`, and a `setProperties {"set":{"due_date":…}}` lands in it instead of the bundled property. Not "no stable address" — the wrong address | ADDRESSING §7.5 req 5; surface review |
| 1.3 | **BSON→slug re-spelling sweep** (153 of 194 relation keys, 5 of 29 type keys) | −19 % on property reads **and** it deletes the 722-token `GET /properties` discovery call a model needs today to learn what `6a76…` means | ADDRESSING §7.5a-1; TOKENS §6.2, action 6 |
| 1.4 | Remaining identity deferrals: view-op `set` channels are stored-key-only; the compact filter string is fold-strict | both fail loud today, so they are debt rather than bugs | APIV2.md §8.23 |

## Wave 2 — make the cheap reads usable

| # | item | value | where |
|---|---|---|---|
| 2.1 | **Locators on block-addressed ops** — `match`/`under`/`nth` as flat fields, `find` doubling as the locator on `replaceText`; exactly-one-or-refuse with candidate listing; resolved server-side under the object lock | patching one word costs **2 417 tokens today, ~45–60 with a locator**; and it is what makes an id-free read writable instead of a trap | TOKENS §5, action 3 |
| 2.2 | **`?mode=outline\|text\|props\|edit\|full`**, default `edit`, retiring `include`/`outline`/`ids`/`format` | `gemma4:e2b` **2/8 → 7/8** optimal reads; the current parameters are not chosen wrong, they are not chosen at all | TOKENS §6.2, action 4 |
| 2.3 | md mention-link short form (drop the redundant same-space `spaceId`, or a label + legend appendix) | md is 83–84 % of a default read on mention-heavy docs — the "cheap text mode" is not cheap where it matters | TOKENS §1.3, action 5 |

**Order matters here:** 2.1 and Wave 0 define what each mode can emit, so 2.2
is cheaper to specify after them.

## Wave 3 — surface completion

| # | item | notes | where |
|---|---|---|---|
| 3.1 | **Phase 8** — file byte-download; the chat SSE stream on `/v2` carrying Phase-6 DTOs; tag/option admin; template reads; **the v1↔v2 conformance test** | auth issuance is deliberately excluded (keys are minted in the app); the conformance test is the exit criterion that makes "complete" checkable. Tag admin is blocked on decision D1 below | SURFACES §10.1 |
| 3.2 | ~~**Phase 9** — space-optional object routes~~ **RETIRED 2026-08-11** — superseded by the short space reference (APIV2.md §8.35), which removes the measured failure without a new route class. Phase 9's *unique* remaining value was a cold-pasted object id with no prior `find`; **no eval has ever produced that case**. Two defects found while scoping it are recorded rather than fixed: `ResolveSpaceIdWithRetry` is `retry.Attempts(0)` — infinite, bounded only by the context, so an unresolvable id spins instead of 404ing; and `set_properties` needs a space regardless, because `propertyFormats`, the option-name guard, `@me` and relative dates are all space-scoped | **D2 is moot**, not decided | SURFACES §10.3 |
| 3.3 | `DELETE /v2/spaces/{spaceId}/objects/{objectId}` (archive) — specced, never registered | also the only way to clean up test data; its absence is why eval fixtures accumulate | APIV2.md §3 |
| 3.4 | `GenerateSchema` + store-backed option join — un-501 `types/{type}/schema` | the wrapper's `describe` runs degraded until this lands | APIV2.md §3, `discovery.go:235` |
| 3.5 | **A range block-remove op on PATCH** — `deleteBlocks {from, to}` (or `{all: true}` scoped to the document root), one op that removes a contiguous run of top-level blocks with their subtrees | **the capability PUT nominally served**: "clear the document and write new content" now costs one `deleteBlock` op per top-level block, so a 60-block rewrite is 60 ops against a 512-op batch cap for what is one intent. With this it is one op + one `insertBlocks`, at OP cost rather than DOCUMENT cost — which is the whole reason PUT could be removed rather than replaced (§8.27). Design notes: reuse `matchBlockRef` for both endpoints (no literal-id channel), refuse a range that straddles a container boundary, and make `diffStats.blocksRemoved` the receipt | APIV2.md §8.27 |

## Wave 4 — format-level

| # | item | notes | where |
|---|---|---|---|
| 4.1 | **Pins + the D1 kill + the SPEC revision** — the pin table, the label-minting algorithm, the total resolution rule | **pins are export-only** (decided): they protect rename/cross-account round trips that PATCH-first agents never perform, at ~22 tok per custom key. D1 (the silent id-as-name fallback) cannot be fixed without them | ADDRESSING §7.1, §7.6 steps 1–2; TOKENS action 7 |
| 4.2 | §7.4 strict write defaults — the kind × verb rule (only options implicitly created; POST permissive, PATCH strict) | supersedes R9's blanket default; free only pre-ship | ADDRESSING §7.4 |

## Quality track — runs alongside, not after

| # | item | notes |
|---|---|---|
| Q1 | **The e2e charter** (E1–E5): a real-account `/v2` fixture, then C9 dry-run really not writing, C8 across a real retry, a PATCH landing in the CRDT with change-set assertions, and a scoped key minted the real way | E1 is the precondition for the rest. E4 also closes **E′8** below. `tests/integration/chat_test.go` is the pattern |
| Q2 | **C′4** — `updateBlock` merges on exported JSON, so `Restrictions`, exotic `Fields` kinds and int64 precision vanish on the touched block even when never named in `set` | contradicts the published "only the named fields change"; `replaceText` does it correctly |
| Q3 | **E′8** — no change-set assertion, so reverting `sb.Apply(st)` to `Apply(st, NoRestrictions, DoSnapshot)` keeps the suite green | the M7 work pinned marshal *counts*; the change-set assertion is still open |
| Q4 | **The B4 model benchmark rerun** — the small-tier ID-vs-locator arms | blocked on the remote Ollama host (Metal-compiler wedge); the exact rerun recipe is in TOKENS §5.4 |
| Q5 | `make openapi` regeneration | pending across the view family, the sorts `id` addition, the insertView schema and every §8.2x change |
| Q6 | The 15 SHOULD-FIX items from the whole-surface review | headlined by pinning the C9/C8/E′8 safety contracts |

## Decisions needed from a human

| id | decision | lean | blocks |
|---|---|---|---|
| D1 | Tag/option rename semantics under names-as-identity | rename = create + migrate + delete server-side; no id-addressed escape hatch (C2) | 3.1 |
| D2 | ~~Object in a space the key does not hold: 403 naming the space, or 404 hiding it~~ **MOOT** — the question only existed for Phase 9's space-less routes, and Phase 9 is retired (3.2). No route reaches an object without a space today, so nothing is waiting on this | — | nothing |
| D3 | `apiObjectKey` mutability — freeze, or keep v1's re-pointing | keep mutable, address-only | 4.1 |
| D4 | The five surface-review decisions: If-Match on types/properties, set-read base scope, the `GlobalAuthExempt` allowlist, per-credential rate limiting, the space-kind schema split | — | the allowlist one should land **before** 3.1 adds SSE and file routes |
| D5 | ADDRESSING's remaining open questions (twin-slug repair depth, uniform strictness for integration scopes, MUST-vs-SHOULD identifier-shaped labels) | — | 4.1 |

## Tickets outside API v2

- **`POST /sets` still runs a whole-document creating-resolver import** — the same dangling-name minting the view ops fixed, on a path that predates them. Needs its own change: the caller authored the whole document, so "did they mean this?" cannot be answered the way it was for view ops. *(PUT was the other half of this item and left with §8.27.)*
- **Date objects** — a view op on one dies inside `sb.Apply` with `state.ErrRestricted` (a different sentinel from `restriction.ErrRestricted`), likely surfacing as a 500. Pre-existing and shared with every block op.
- **GO-5969** — the type default-view visibility regression, cherry-picked to `develop` as PR #3235. Independent of this branch; every type created since Nov 2025 has an all-columns-hidden "All" view until it merges.
- Two eval documents remain in the throwaway test account (no object DELETE — see 3.3); they double as the Q4 rerun fixtures.

## The shortest useful path

If only three things ship: **0.1 + 0.2** (a third off every read, for an
afternoon's work), **2.1 locators** (the 50× edit flow, and it makes the cheap
modes honest), and **1.2 backfill** (it is a live silent wrong-entity write, not
a deferral).
