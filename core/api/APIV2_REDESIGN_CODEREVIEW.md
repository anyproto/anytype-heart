# API v2 PATCH state-ops redesign — 4-lens review synthesis (2026-07-30)

Four opus lenses over `ef8e4385b..HEAD` (4 commits: `dafb99322` fragment API,
`f987b9244` port, `a53984f7c` state-ops applier, `efe2ebc80` §8.2 v0.3.4):
state-ops correctness · fragment API · validation & adversarial safety ·
contract & tests. Three lenses ran live probes in-package. Deduped and
severity-tiered; cross-confirmations noted.

Baseline verified by the synthesizer before review: tree clean, `go build`
exit 0, all 12 packages green, `ResetToVersion` confined to `ResetObject`
(PUT), PATCH ending in a plain `sb.Apply(st)`. *(2026-08-10: `ResetObject`
and PUT are gone — APIV2.md §8.27 — so `ResetToVersion` has no API caller
at all; findings below that name PUT as the fallback or the comparison
are retired with it.)*

## Verdict

**The redesign's thesis is correct and its foundation is verified sound.** The
state-ops lens actively tried to break the op→state mutations and could not:
insert positions match `position.go`, `replaceSubtree`'s splice index is right
*because* `insertAt` re-picks the parent after the unlink, the `moveBlock`
cycle check reads the pre-unlink view, orphans are collected, the view is
invalidated after every mutating op. **A1/A2/A3/A5/C6 genuinely hold by
construction** — independently verified, including that `resolveLayout` now
sees `cur == new` so the paragraph-eating conversion cannot fire, and that
plain `Apply` leaves `doSnapshot=false` / `addHistory=true`.

**But the damage moved rather than vanished.** Removing the whole-document
`Validate` was load-bearing for more than §8.2 admits (V3 and the
document-wide id domain are now unenforced), and three things are strictly
*worse* than the pipeline they replaced: side effects now escape the
preconditions entirely, the per-op view rebuild is a performance regression
inside the object lock, and table ops re-mint wrapper ids on every edit.

---

## TIER A — regressions vs the old pipeline (fix before anything else)

### A′1 — Create-missing side effects escaped the preconditions [3 lenses: state-ops #1, validation #4, contract #10] — MAJOR
`prewarmCreateMissing` (v2_edit.go:56-57) runs **before** `MutateObject`, i.e.
before `checkObjectEditable`, before `checkEditPreconditions` (If-Match,
sbType), and before the object is known to exist. Pre-redesign these ran
*inside* `finishEdit`, after the precondition checks. Reproduced: `PATCH` an
object that **does not exist** → 404, **and a tag option is permanently
created in the space**. Same for stale If-Match (412), restricted object
(403), and ops aborted by a later op's validation error — and because prewarm
resolves *all* ops up front, one rejected request can mint every option named
anywhere in the batch.
**Fix:** read the object + run `checkEditPreconditions`/`checkObjectEditable`
*before* prewarm (a read costs nothing here); cap created options per request;
attach side effects to the error payload so a rejected agent learns what it
caused.

### A′2 — The per-op view rebuild is a DoS regression inside the object lock [state-ops #3, validation #3] — MAJOR
Every mutating op nils the view; every op then rebuilds it via a full
`snapshotFromState` + `anyblockjson.Marshal` **and a brand-new
`storeresolver`** whose caches start empty (re-running `ListAllRelations` /
`ListRelationOptions` per op). `begin()`'s already-marshaled document is
thrown away instead of seeding the view, so even a 1-op PATCH marshals three
times. With no op cap and a 10 MiB body (~3×10⁵ trivial ops), a batch holds
`DoContextFullID` for O(ops × document) work. **This is precisely the B6
finding the redesign was meant to relieve, made worse** — the old pipeline
parsed once.
**Fix:** seed `a.view` from `begin()`'s `beforeDoc`; reuse the final view as
`afterDoc`; keep ONE resolver on the applier across marshals; cap ops (~512);
check `ctx.Err()` per op. Longer term: address blocks off the state tree
instead of re-marshaling.

### A′3 — Every table op re-mints both layout wrappers [state-ops #2, contract #6] — MAJOR
`tableFromJSON` (table.go:265-273) always assigns `imp.genId()` to the
columns/rows wrappers, so a `setCell` replaces the table's `ChildrenIds`, adds
2 blocks, deletes 2, and re-parents every row and column — while `diffStats`
reports `{BlocksChanged:1}`. **Two devices editing different cells produce
disjoint wrapper structures that merge into a table with duplicated rows and
columns.** The in-code comment "only the edited cell diffs" and §8.2's "the
diff is no longer blind to anything real" are both false here.
**Fix:** feed the live table's existing wrapper ids into the fragment import
(or derive them deterministically from the table id); correct the comment and
the §8.2 claim.

## TIER B — guarantees silently lost with the whole-document validate

### B′1 — V3 (row→column containment) is enforced by nobody [validation #1] — MAJOR
`resolveTarget` pre-checks only `LeafBlockType`, and `row` correctly isn't a
leaf; the only V3 check lives in `checkFlatRun`, which now sees an isolated
fragment, never the spliced result. Three **single-op** repros return 200 and
produce a document that fails `anyblockjson.Validate` — i.e. **the object's own
GET body is no longer PUT-able**, and R5 is violated by construction:
`insertBlocks inside <rowId>`, `moveBlock inside <rowId>`, `replaceSubtree` a
column with a paragraph.
**Fix:** check containment against the **live parent's type** on every splice
(`insertBlocks`/`moveBlock`/`replaceSubtree`/`updateBlock`/`replaceBlock`) with
an `ops[i]` path — the applier already knows the parent.

### B′2 — The dropped depth bound corrupts the applier's own view mid-batch [fragment #4, validation #2] — MAJOR
Fragment validation is run-**relative**, so a 33-deep run passes; inserted at
depth ≥1 it pushes the document past `maxBlockIndent=32`. Because `marshalDoc`
always installs a non-nil `OnWarning`, the exporter takes the **clamp** branch
and `doc()` discards the warnings — so **within the same PATCH**, op N+1 sees
clamped indents: `deleteBlock` computes `descendants == 0`, **skips the
recursive guard and drops an entire subtree**; `moveBlock`'s cycle check and
`updateBlock`'s leaf check are mis-scoped the same way. Afterwards `begin()`
*does* see the warning → **every subsequent PATCH on that object 422s** —
a self-inflicted permanent lockout, repairable only via PUT/app.
**Fix:** carry a base depth into fragment validation
(`UnmarshalBlocks(run, base, opts)` / `Options.BaseIndent`) and pass the
anchor's depth; enforce the bound as a cheap post-op tree walk rejecting with
`ops[i]`; make `doc()` **fail rather than degrade** when the view would clamp.

### B′3 — Make the post-op document validate hard, not env-gated [contract #2] — MAJOR
`moveBlock`, `deleteBlock`, `setProperties`, `addItems`/`removeItems` get **no**
document-level check at all; the replacement is gated behind
`ANYTYPE_API_V2_VALIDATE_EDITS=1` and only logs. Fragment validation also can't
see the document-wide id domain (derived `rowId-colId` cells, other tables'
row/col ids). Decisive: **`afterDoc` is already marshaled for diffStats and
already handed to the debug validator** — the check is one env-var away from
free.
**Fix:** make it a hard failure **by default** (keep the env var to *disable*),
reusing `invalidDocError`. This also backstops B′1/B′2 cheaply.

## TIER C — contract breaks (the agent repair loop depends on these)

- **C′1 — R5 issue paths lost their `ops[i]` prefix** [validation #5, fragment #9, state-ops #10, contract #3 — all four lenses]. Fragment issues surface as `/blocks/0/type`, so in a 4-op batch an agent cannot tell which op failed; `replaceText`'s markup failure carries **no path at all**; and `runPathFor` maps every non-top id (table rows/columns/cells) to index 0, pointing the repair loop at an innocent block. **Fix:** re-prefix and re-base fragment issue paths onto `ops[i].blocks[j]` in `invalidDocError`; give `replaceText` issues `ops[i].replace`; attribute non-top ids to their owning top block, falling back to `ops[i].blocks`.
- **C′2 — dry_run double-reports every created option** [contract #1, state-ops #4]. `OptionId`'s dry branch appends to `sideEffects` without memoizing, and prewarm + the in-lock pass both call it → `created.options` lists each option twice; the real run lists one. **Breaks C9's dry≡real on the one field dry-run exists to preview.** **Fix:** memoize the dry-run miss / dedupe by ref.
- **C′3 — dry_run skips the adapter guards** [validation #8, state-ops #9, contract #11]. `checkObjectEditable` and `guardBundledRevision` live in `MutateObject`, so a dry run returns 200 where the real PATCH 403s; and the dry state is built from the read snapshot, so local details/relation links/structural blocks the live doc owns are absent — `applySetProperties`'s `inDoc` escape and `begin()`'s C11 guard can therefore reach different verdicts. **Fix:** hoist both guards to a service-level check both paths call.
- **C′4 — `updateBlock` silently drops non-format fields** [contract #7]. The merge happens on the *exported* JSON and re-imports, so `Restrictions`, exotic `Fields` value kinds and int64 precision vanish on the touched block even when never named in `set` — contradicting the published "only the named fields change". `replaceText` does it correctly (`b.Copy().Model()`). **Fix:** overlay onto a copy of the live model, or amend the schema/§8.2.

## TIER D — known-open, unfixed by the port (re-confirmed, now measured)

- **D′1 — `replaceText` markup injection (B3)** [fragment #3, state-ops #8]. Now quantified: on `"the cat sat"`+Bold, `replace:"a*b"` **destroys the mark and leaves raw asterisks in stored text**; `replace:"*star*"` invents an italic; `replace:"<mention objectId=…>"` **mints a mention at an arbitrary object id from plain text**. The fragment API *hinders* the fix — it exports the codec pair but no escape helper. **Fix:** export an escape helper and escape `op.Replace` for text-bearing blocks (leave code/embed literal), or run find/replace on plain text with offset-shifted marks.
- **D′2 — `revision`/`sourceObject` still settable (B4)** [validation NOTE, state-ops #6]. `setProperties` marks an ordinary object bundled-derived and pins it above future revisions — defeating `guardBundledRevision`, which reads the same child state the op just wrote. The mirror case (`unset`) silently no-ops. **Fix:** add both to the output-only denylist; reject `LocalAndDerivedRelationKeys` minus export exemptions; validate values by format.
- **D′3 — `leafBlockTypes` incomplete** [validation NOTE, state-ops #7, fragment]. `code`/`file`/`image`/`video`/`audio`/`pdf`/`widget`/`row`/`column`/`group` missing; `insertBlocks inside <image>` now has **nothing** behind it since the net was removed. Needs a deliberate decision — SPEC §5 permits legacy nesting under file blocks.
- **D′4 — B1 (dangling refs from compact reads), B9 (raw internal errors in 500s), B8 (float64)** all unchanged. B8 is *wider* than §8.2 claims: it also affects **client-supplied payload numbers** in insertBlocks/replaceBlock/replaceSubtree, which never touch the view.

## TIER E — new-substrate defects and coverage debt

- **E′1 — Structural guard misses table cells** [fragment #2]. Reproduced: a `featuredProperties` or `title`-styled block inside `rows[].cells[]` passes, via `setCell`, `insertBlocks`, `replaceSubtree`. **Fix:** recurse the guard over cells in both object and array forms.
- **E′2 — `MarshalBlockSubtree` honours compaction flags with nowhere to put the legend** [fragment #1]. Ids and object refs silently truncate (`aaaaaaaa…`→`aaaaa`, link target→`ectid`) and feed back truncated. **Fix:** reject/force-clear the compaction+OmitIds flags in the fragment marshal.
- **E′3 — Primary dataview id unprotected** [fragment #5]. `replaceSubtree`/`deleteBlock` on a set's `"dataview"` block mints a fresh id or removes it; the editor then adds a second default view. **Fix:** refuse to re-id/delete it on set/collection/objectType documents.
- **E′4 — `setProperties.unset` has no key validation** [state-ops #5]. `unset:["id"]`/`["type"]`/`["spaceId"]` pass into `RemoveDetail` (self-repairing via `injectDerivedDetails`, but the hole is real and typos silently no-op). **Fix:** run `unset` through the same lifted-key + unknown-key checks as `set`.
- **E′5 — Relation link can contradict the value** [contract #8]. `propertyFormat` falls back to `longtext` while the value was decoded under the same failed resolution → a `longtext` link on a value the format layer read differently: the A1 class the link exists to prevent. **Fix:** thread the resolved format into the link, or refuse unresolvable keys.
- **E′6 — Delete-then-recreate the same id in one batch is wrongly rejected** [validation #6]. `deleteBlock` only unlinks, so `st.Exists` stays true → 400 "it already exists" for a block the view no longer shows. **Fix:** track unlinked-this-PATCH ids.
- **E′7 — `UnmarshalPropertyValue` has no error channel** [fragment #6]. `dueDate:"tomorrow"` silently stores a string on a date property with a 200.
- **E′8 — Coverage: the port was mechanical** [contract #5, #4, #9]. Old and new suites have **byte-identical `t.Run` name lists** — 45 in, 45 out, **zero new cases for anything the redesign introduced**: view invalidation, `checkFreshIds`, `replaceLive`, id reuse, prewarm ordering, the debug validator. Concrete regression that ships green: **deleting any `a.mutated()` call** (every block test is single-op). Worse, `TestMutateObject` stops at final state, so **switching `sb.Apply(st)` back to `Apply(st, NoRestrictions, DoSnapshot)` — the exact thing the redesign escaped — keeps every test green.** And the atomicity test now only restates the mock's wiring. **Fix:** assert `st.GetChanges()` kinds/count for a one-block edit (A5 proof); add insert-then-address-created-id, move-then-address, delete-then-sibling-suffix, duplicate-payload-id path, prewarm ordering; drive one PATCH end-to-end through the real adapter over `smarttest`.
- **E′9 — `fields._detailsKey` not stripped on write** [fragment #7]. A payload can mint a second name-bound "title" block — what the §7 guard exists to prevent. Pre-dates the redesign (PUT has it too).
- **E′10 — SPEC §13 stale** [fragment #8]: no `fragment.go`, none of the six new exports listed, and it still asserts "the inline codec is internal — not part of the public API", which `fragment.go` falsified.

## Corrections to §8.2's own claims (verified false)
1. "Per-block restriction checks ride the Apply path" — **inert**: `Apply` calls `s.ParentState().CheckRestrictions()`, and for `sb.NewState()` the parent's own parent is nil, so it returns immediately. A pre-existing upstream quirk shared with every `Block*` handler, but the redesign did not gain it.
2. "The diff is no longer blind to anything real" — false for table wrapper churn (A′3).
3. "B8 narrowed to the one re-imported block" — omits client-supplied payload numbers.
4. "Only the edited cell diffs" (in-code comment) — false (A′3).

## Disposition

Nothing here invalidates the redesign — keep it. Fix order:
1. **Tier A** (regressions): prewarm behind preconditions; seed the view + one
   resolver + op cap; pin table wrapper ids.
2. **Tier B** (lost guarantees): live-parent containment check; depth base +
   fail-don't-clamp; **turn the post-op validate on by default** (cheapest,
   backstops the other two).
3. **Tier C** (contract): `ops[i]` paths, dry-run dedupe + guard parity,
   `updateBlock` merge fidelity.
4. **Tier D/E** as a seams-and-coverage pass — with E′8's change-set assertion
   first, since it is what keeps all of this from silently regressing.
