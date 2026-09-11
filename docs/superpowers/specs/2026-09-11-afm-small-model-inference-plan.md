# On-device small-model inference over the task-tool wrapper — plan

2026-09-11 (rev 2). Status: heart side of workstream A built and recorded as `core/api/APIV2.md` §8.58 (cross-space find, `type=type`, honest empty lists, no-mint admission, manifest text, served instructions, `invalid_arguments`, optional `space`, four eval tasks); §8.59 adds the run context, space names as space arguments, the recents and the global preamble (D1 v1 with recents). D2 dropped in favour of the preamble's recent types; §8.60 settles D4 (the host sets MaxResultChars, the wrapper cuts read by blocks and describe by rows). Remaining: the baseline run, then D6. Branch `go-7383-apiv2-clean`,
HEAD `ea4b561b2`. Input: the Swift-side failure report
(`heart-wrapper-afm-prompt.md`: a two-turn Apple Foundation Models run over
`HeartToolsBridge`, tier `small`, that failed on every axis) plus a read of
the wrapper, the v2 service, the objectstore and the Swift LocalAI code.

Rev 2 changes: the "unloaded space" root cause for problem 3 is withdrawn
(§1.1 states what the code proves and what is still open); cross-space
`find` becomes the primary mechanism and doubles as the type listing
(`type=type`); the filter grammar leaves the session and is replaced by one
line naming its syntax; the device build discrepancy is recorded.

Scope: make Apple Foundation Models (AFM, the ~3B on-device system model,
4,096-token window) and its peers (Gemma 4 E2B, Bonsai 8B over MLX on iOS;
gemma/qwen-class over Ollama/LM Studio on desktop) as capable and as cheap
to drive as the wrapper can make them. Every change below is in heart unless
the section says otherwise; §7 lists what stays on the client.

---

## 0. Summary

1. **What the run proves** (§1). The short space ref WAS resolved, the space
   WAS admitted, and the store's type query ran and returned zero rows. The
   refusal then rendered an empty list as "the space has no types yet" and
   the model believed it. Why the index answered zero for that space on that
   phone is not decidable from the code; §1.1 gives the one-call check. The
   fix does not depend on the answer: an empty type list must never read as
   a fact about the space, and the model needs a way to search and to list
   types that does not require guessing a space first.
2. **Cross-space `find` is the missing primitive** (§3.1). One tool answers
   "find it" with no space named, and with `type=type` lists the types of a
   space or of the whole account — no new tool slot, and the type list the
   report asked for falls out of it.
3. **The context budget** (§2). Measured: the small tier as served is ~8 KB
   of tool text plus ~2.1 KB of filter grammar the client puts in the
   instructions — roughly 2.7k tokens of a 4,096-token window before the
   user has said a word, and `read` results are unbounded. Efficiency is the
   other half of correctness here.
4. **The device did not run HEAD.** The xcframework the report was produced
   with is heart `05d1bb75e`, a rewritten predecessor of this branch (merge
   base `95119e6b4`, 95 commits and 276 files behind HEAD). The type path is
   identical in both, so §1 stands, but every future run must state the
   heart commit and the framework must be rebuilt from HEAD before measuring.
5. **We cannot measure AFM today** (§5). `cmd/apiv2eval` speaks
   OpenAI-compatible HTTP; AFM lives only inside Apple's runtime. A small
   macOS bridge closes that, and should land before the fixes.
6. **Five decisions need the user first** (§6).

---

## 1. Root causes and verdicts (problems 1–6 of the report)

| # | Problem | Verdict | Root cause (file:line) | Fix |
|---|---|---|---|---|
| 1 | `TASK` read as an enum value; "would you like to create a task" | **design gap** | `manifest.go:168,201,214` (`"a type name, e.g. Task"`), the three `Example` maps carrying `"type": "Task"`, and `any-block …/filterstring/grammar.go:45` (`type IN ("task","bug")`) served verbatim into the session. Tuned against 4B–27B models that read "e.g." as an example; a 3B reads the only noun it has as the vocabulary. | §3.3, §4.2, §4.5 |
| 2 | No route to a type name in tier `small` | **design gap** | 8 tools, none lists types; `describe` requires `type` (`manifest.go:201`). The type-unknown hint says "find results show each object's type" (`steer.go` restVocab) — a loop the model cannot exit. | §3.1 (`find type=type`) |
| 3 | "the space has no types yet" ended the run | **bug in the refusal; the empty index is open** | See §1.1. `knownTypeKeys` (`refs.go:120`) returns the (empty) list and `listKnown` (`refs.go:314`) renders it as a fact about the space; a load error would render the same sentence. | §3.2 |
| 4 | Only global search checks whether a space is loaded | **design gap, low priority** | `OpenedSpaceIds()` is consulted once (`search.go:981`); per-space routes call `SpaceIndex`, which mints an empty index for any id (`anystoreprovider.GetSpaceIndexDb`, `provider.go:293`) and marks it opened. Not shown to be the cause of this run, but a read must not create store state. | §3.2 (prevent the mint) |
| 5 | Wrapper has no cross-space find | **design gap — the primary one** | `runFind` (`tools.go:118`) always POSTs `/v2/spaces/{space}/search`; `POST /v2/search` exists and is never called by the wrapper. Handles carry no space (`session.go Handle`). | §3.1 |
| 6 | Pre-flight validation costs a repair | **design gap** | `Host.Call` (`host.go:62`) maps every error to `Code: tool_error`; `validateArgs` errors are plain `fmt.Errorf`, distinguishable from `*ToolError` today but not surfaced. `space` is `Required: true` on find/describe/create/create_type with no default. | §3.4 |

### 1.1 Problem 3: what the code proves, and what only the phone can say

Traced on both HEAD and the device build `05d1bb75e`
(`core/api/v2/service/service.go ensureSpace`, `spaceref.go`, `search.go
buildSearchPlan`, `refs.go`, `handler/error.go`):

- `hpujze` was resolved. The route middleware (`core/api/v2/spaceref.go`)
  rewrites the `space_id` param in place; an unresolved ref would have hit
  `ensureSpace` → `GetSpaceViewDetails` → 404 "space not found". The run got
  the type refusal instead, so the space was resolved and admitted. The
  message spells `hpujze` because `handler/error.go echoSpaceRef` substitutes
  the caller's spelling back into every message.
- The type query succeeded with zero rows. `buildSearchPlan` calls
  `liveTypes` first; a store error there is a plain error → `RespondError`
  → 500. The run got a 400 `validation_failed`, so the query ran, returned
  nothing, `resolveTypeInput("TASK", [])` fell to the bundled fold (`task`,
  `Id == ""`), and `unknownTypeKeyError` → `knownTypeKeys` (a second query,
  same result) → `listKnown([])` → "the space has no types yet".
- A loaded space cannot legitimately have zero types: every load runs
  `tryLoadBundledAndInstallIfMissing`, which installs `bundle.SystemTypes`
  (Page, Collection, Query, Bookmark, Chat, …) — chat and one-to-one
  spaces included. So the phone's per-space index held no object with
  `resolvedLayout = objectType` for that space at that moment. Loaded
  (`spaceLocalStatus = Ok`) and indexed are decoupled: the optimistic-Ok
  fast path reports Ok before the background build, tree sync and the
  indexer fill the index, and a fresh install of a dev build starts with an
  empty index for every space.

**The one-call check on the phone**, on the same build: ask for `find` with
`space: hpujze` and no query/type/filter. It lists what the index holds. "the
space is empty" ⇒ the index had nothing for the space (sync/index lag or an
absent store dir); a list of pages with zero types ⇒ a type-object-only
gap, which would be a real indexer bug worth its own issue.

Either way the fix is the same and is in §3.1–§3.2; on-demand space loading
in the API (rev 1's D3) is withdrawn.

---

## 2. The context budget, measured

Small tier, as served by `BuildManifestForTier(TierSmall)` at HEAD:

| Component | Bytes | ≈ tokens (÷4) |
|---|---|---|
| 8 tool descriptions + JSON schemas + examples | 7,994 | 2,000 |
| Filter EBNF (client puts it in instructions) | 1,800 | 450 |
| Filter examples | 284 | 70 |
| Client's own instructions (`LocalAIRuntimeTools.instructions` prose + `LocalAIChatService` constants) | ~600 | 150 |
| **Fixed cost before the prompt** | **~10.7 KB** | **~2.7k** |

AFM's window is 4,096 tokens for instructions + tools + transcript + output.
Apple renders tool definitions in its own format, so the true count is
unknown until the bridge in §5 measures it, but ~1.3k tokens is what is left
for the user turn, every tool result, and the answer. One `read` of a
medium page (unbounded today — `runRead` serves the document verbatim) or
one `describe` of a type in a space with 60 properties exceeds it on its
own. The MLX models on iOS have larger windows but pay the same per-token
latency, so the cut helps them too.

Targets for the small tier (checked by a test, §5.3):

- fixed cost ≤ 4.5 KB (tools + served instructions + examples);
- any single tool result ≤ a per-profile cap (default 2,000 chars) with a
  stated continuation, never a silent cut;
- `find` rows ≤ 60 chars each; `describe` ≤ ~25 rows by default.

---

## 3. Workstream A — correctness (heart)

### 3.1 Cross-space `find`, which also lists types

Where: `wrapper/tools.go runFind`, `wrapper/session.go`, `wrapper/runner.go
resolveObject`, `wrapper/routes.go`, `wrapper/manifest.go`;
`v2/service/search.go buildSearchPlan` (server side of `type=type`).

- **`space` optional on `find`.** Omitted → `POST /v2/search` (global over
  loaded spaces, grouped and merged by the existing fan-out). The response's
  `warnings` (`incompleteSearchIssue`, per-space skips) are rendered into
  the text exactly as per-space find already renders warnings, so the model
  knows when an answer is partial.
- **Handles carry their space.** `Handle` gains `Space` (the short ref the
  global row serves in `space_id`); a global find sets `Session.Space = ""`
  and numbers handles across spaces; `resolveObject` returns the handle's
  own space; the explicit-space redirect refusal stays for single-space
  finds. Each row prints its space once: `3. Prague trip (Page, in Weekend
  Trips)`; single-space rows stay as they are.
- **`type=type` lists types.** `type` naming the type-of-types (the display
  name `Type`, key `object_type`, and the bare `type` all resolve through
  the existing fold) is a search whose rows are the type objects: per space
  (`find space=X type=type`) or account-wide (no space). Server side: the
  base row scope (`appendBaseRowScope`, `util.ObjectLayouts`) excludes the
  `objectType` layout; the plan widens it when the resolved type key is
  `objectType`, the same opt-in `includeFileLayouts` is for file types. The
  wrapper renders type rows with their object count when the row carries
  it, and `describe` accepts a handle that resolves to a type object, so
  `find type=type` → `describe 2` composes.
- **The refusal names the repair.** The wrapper-side type-unknown text (and
  the `restVocab` line "check the type name (find results show each
  object's type)") becomes "list the types with find type=type — add space
  to list one space's". `spaces` tells the model the same in its footer.
- Budget: `limit` default 10 across spaces; the global route's
  `maxGlobalSearchOffset` is irrelevant at this depth.

### 3.2 An empty list is not a fact about the space

Where: `v2/service/refs.go listKnown / knownTypeKeys / knownPropertyKeys`,
`v2/service/service.go ensureSpace`, `wrapper/steer.go`.

- `listKnown` with an empty list says what was looked up, not what the
  space is: *"no types are indexed in this space on this device yet —
  retry in a moment, or search across spaces with find"*. A load error in
  `knownTypeKeys` / `knownPropertyKeys` stops degrading to `nil` and
  becomes its own sentence (*"could not read the space's types: …"*), so a
  store failure and an empty index never render the same.
- Reads do not mint store state: before `SpaceIndex(spaceId)` on a read
  path, the service checks the space is in `OpenedSpaceIds()` or has a
  store on disk; otherwise it refuses with the sentence above instead of
  creating `objects.db`. This closes problem 4's second consequence
  (global search counting a minted index as searched-and-empty) without
  any loading logic in the API.
- Test: a `v2service` fixture where the space view exists and the index
  holds no type rows — the refusal must not contain "no types yet" and must
  name `find`.

### 3.3 The manifest stops teaching `Task`, and the grammar leaves the session

Where: `wrapper/manifest.go`, new `wrapper/examples.go`, `wrapper/mcp.go`.

- `type` descriptions: *"a type name (find type=type lists them)"* — no
  "e.g.". Examples: find shows the `query` form with no space
  (`{"query":"prague trip"}`) — the call a user's first sentence maps to;
  describe shows `{"type":"Page"}`; create keeps `Page` (bundled, present
  in every space), never `Task`.
- **Filter syntax is one line**: *"filter is a SQL WHERE clause over
  property names: `Done = false AND Due_date < currentWeek()`; strings in
  double quotes, multi-word names with underscores"*, plus one more
  example. The EBNF and the any-block example list stop being served into
  the session. The full grammar stays in the manifest JSON
  (`filterGrammar.ebnf`) for hosts and UIs that render help, and it is
  explorable on demand: a parse error already carries a repair hint
  (`filterstring.Error.Hint`, e.g. *"a condition starts with a bare
  property key, e.g. status IN ("Done")"*), and the eval measures whether
  that is enough before anything more (a `help` argument, a syntax line in
  the parse error) is added.
- The manifest gains `instructions` per tier (the `mcpInstructions(tier)`
  text, tightened — §4.1) so the client stops authoring its own.

### 3.4 Errors a client can budget, and a first call that can succeed

Where: `wrapper/host.go`, `wrapper/runner.go validateArgs`,
`clientlibrary/service/tools.go`, `wrapper/manifest.go`.

- New envelope code `invalid_arguments` for wrapper-side pre-flight
  failures (unknown arg, missing required, wrong type, bad enum) — a
  non-`ToolError`, non-transport error in `Run` before the executor runs.
  `tool_error` keeps meaning "the request was made and refused". The Swift
  turn policy can then leave `invalid_arguments` out of the repair budget
  (client change, §7).
- `space` becomes optional on describe/create/create_type as well.
  Resolution order: explicit arg → `Session.Space` (last single-space
  find, or the space of the handle a create is next to) → the host's
  default space (§4.4) → refusal that auto-runs `spaces` and names them
  (the `steer.go` pattern), so the repair is in the refusal, not one more
  call away. `find` needs none of this: with no space it is global.

---

## 4. Workstream B — efficiency for a 4k-token model

### 4.1 Served text on a budget

- **Descriptions.** Cut every small-tier description to ≤ 160 chars: first
  sentence what it does, second the one rule that changes behaviour. The
  measured findings behind the current prose (§8.21, §8.33, §8.51) are
  preserved as behaviour and as error text, not as description text — the
  record says prose does not steer these models ("arm B2 measured that it
  does not", `manifest.go` find comment). Decision D6 covers whether this
  is one description for all tiers or a `Brief` field.
- **Schemas.** Drop `maxLength` from the served schema (AFM does not
  enforce it; ~20 chars × 30 args); keep `enum`, `minimum` / `maximum`.
  Argument descriptions ≤ 60 chars; `spaceArgDescription` (150 chars)
  rewritten.
- **Instructions** served in the manifest (§3.3): ≤ 500 chars — the loop in
  four lines, the filter line, dates and `@me`, "follow the error once".
- **A byte-budget test** (`manifest_test.go`): small-tier fixed cost ≤ 4.5
  KB, fails on growth — the role `TestManifestExamplesAcceptedByGrammar`
  plays for C13.

### 4.2 Tool results on a budget

Where: `wrapper/host.go` (profile), `wrapper/tools.go runRead`,
`wrapper/describe.go describeText`, `wrapper/tools_list.go`.

- A **result cap** per host profile (D4): default 2,000 chars for the small
  profile, none for large. A cut is explicit: *"… (showing 14 of 41 blocks
  — read with mode=outline for the rest, or name a block)"*. Applied in
  `Host.Call` for every tool, with `read` and `describe` cutting smarter
  first:
  - `read` full: blocks in order until the cap; the cut names the next
    block label. `read` outline is already ≤ 80 runes per block.
  - `describe`: the type's own rows first (already), then at most N other
    settable rows, then a count; options per select ≤ 8 with "+N more, ask
    with options=<name>" (`describeOptionsLimit` is 25 today).
  - `find`: `n. name (Type)` with names cut at 60 runes.
- Warnings and hints survive the cap (appended after the cut).

### 4.3 Fewer calls per task

- The first `find` needs no `spaces` call (global); the first `create`
  resolves its space through §3.4.
- `create`/`set_properties` refusals keep listing candidates (the measured
  repair mechanism, §8.21) but cap candidate lists at 10.

### 4.4 Session context the model does not have to fetch

Where: `wrapper/host.go`, `clientlibrary/service/tools.go`,
`core/api/service.go`.

- `ToolsSetContext(json)` export → `Host.SetContext{DefaultSpace, Locale,
  Now}`. The host defaults `space` (§3.4), resolves relative dates against
  `Now`/timezone, and renders a **preamble** on request.
- `ToolsPreamble()` export (or a field on `ToolsManifest`) renders ≤ 400
  chars of live workspace facts for the client to prepend to the user turn
  or the instructions: today's date, the working space name, the other
  space names, and the working space's type names. This is the "no
  workspace context in the prompt" gap from the report, rendered by heart
  so MCP `initialize` can serve the same text (D1).

### 4.5 Session-scoped manifest (D2 — the largest lever, gated on measurement)

`BuildManifestForSession(tier, ctx)`: same table, but `type` on
find/describe/create carries `enum` = the working space's live type names
(≤ 24; beyond that, free string plus the list in the description), `space`
is optional with its description naming the default, and examples are real
(`"type": "Trip"`). Guided generation on AFM enforces the enum, so the
`TASK` class of error becomes impossible rather than repairable. Costs: the
manifest is no longer pure (it needs an account), and the Swift session's
tools are fixed at `LanguageModelSession` init, so a working-space change
means a new session (FoundationModels can carry the transcript across
sessions — client work, §7). The static manifest stays for the pre-login
case and for hosts that cannot rebuild sessions.

---

## 5. Workstream C — measure on the real model

### 5.1 An AFM arm for `cmd/apiv2eval` (D5)

The harness needs an OpenAI-compatible `/v1/chat/completions` that runs
Apple's model with tool calling. Two ways:

- **Build it** (recommended): a ~300-line Swift CLI in
  `anytype-swift/Tools/afm-openai-bridge` on macOS 26 — `LanguageModelSession`
  with the request's tools converted through the SAME schema adapter the
  app uses (`LocalAIRuntimeTools.foundationModelSchema`), instructions from
  the `system` message, transcript replayed per request, tool calls
  returned as OpenAI `tool_calls`; reports Apple's own token count where
  the API exposes one, which settles §2's estimate.
- **Reuse** a third-party bridge if one exists and exposes tools (verify
  first; its schema adapter would then differ from the app's, which is the
  thing under test).

The harness itself needs: `-arms wrapper/small` against
`OLLAMA_BASE_URL=<bridge>`; `armPreamble` variants (with the heart-rendered
preamble, and with none — the report's condition); and the heart commit
printed in every `summary.txt` header (the report was produced on
`05d1bb75e`, not HEAD).

### 5.2 Eval cases that would have caught the run

1. `find-anywhere` — two spaces, the target in the non-working one, the
   prompt names no space. Passes when `find` with no space finds it.
2. `list-types` — "what kinds of things are in <space>"; passes when
   `find type=type` (or the equivalent global call) lists the fixture's
   types and no `Task`/`TASK` argument is emitted.
3. `find-non-task-type` — a `Trip` type with `Prague trip` in it; "tell me
   about my prague trip". Fails on any `TASK`/`Task` argument.
4. `first-call-no-space` — the wrapper arm with no preamble at all (the
   report's condition). Passes when the first `find` succeeds with no
   `spaces` round-trip, or after exactly one.
5. `read-over-budget` — a 60-block fixture; every tool result ≤ cap, the
   cut is stated, and the edit still lands.
6. `empty-index-refusal` — a `v2service` unit test (§3.2): with no type
   rows the refusal names `find type=type` and never says "no types yet".

### 5.3 Static checks

- Manifest byte budget (§4.1), per tier.
- `-probe` on AFM: one-turn schema emission for each small-tier tool with
  the session-scoped manifest — the enum either constrains or it does not.

---

## 6. Decisions needed before building

| # | Decision | Options | Recommendation |
|---|---|---|---|
| D1 | Where workspace context (spaces, working space, date, type names) enters the model | (a) client builds it from its own state · (b) heart renders `ToolsPreamble` and the client injects it · (c) both | **(b)** — one definition, MCP gets it through `initialize.instructions`, and it is measurable in the eval without the app. |
| D2 | Session-scoped manifest (§4.5) | (a) static manifest + `find type=type` only · (b) session-scoped now · (c) static first, session-scoped as phase 3 gated on the AFM probe | **(c)** — the static fixes are cheap and needed anyway; the enum is the strongest lever but changes the manifest contract and forces session rebuilds on the client. |
| D4 | Result cap ownership | (a) fixed per tier · (b) a host profile set by `ToolsSetContext` (defaults per tier) · (c) the client truncates | **(b)** — the cut must be stated in the tool's own vocabulary (block labels), which only the wrapper knows; the client knows the window, so it sets the number. |
| D5 | How AFM is measured | (a) build the macOS bridge · (b) Gemma 4 E2B as a proxy · (c) manual on-device runs | **(a)**, with (b) as the regression proxy on every run. |
| D6 | Small-tier descriptions | (a) shorten the one description for all tiers · (b) add `Brief` per tool, tiered · (c) shorten all, add `Brief` only if the bonsai-27b sweep (§8.51: 30/34) regresses | **(c)** — keeps the one-definition rule until a measurement says otherwise. |

D3 (load an unloaded space on demand) is withdrawn with the root cause; the
API only stops minting store state on reads (§3.2).

**Decisions taken 2026-09-11 (user):**

- D1: heart renders ONE global preamble, never a per-space one (an account
  can hold 100 spaces). v1 carries the date, the working space and the
  space count; a later iteration injects the N most recently used space
  names and the M most recently used type names — small models forget
  fast, so the preamble carries the most useful facts, not all of them.
- D5: measured on macOS 27's native Foundation Models once installed; the
  bridge is built there. Until then Gemma 4 E2B and the Bonsai models are
  the proxies.
- D6: shorten the descriptions, but only after a baseline exists on the
  target models.
- Targets widened: Bonsai 8B and 27B (larger windows, strong tool use) are
  first-class alongside AFM, so nothing built for the 4k window may cost
  the larger models — caps and text budgets are per profile, never global.
- D2 and D4: see the open questions below.

Also to confirm: whether `find type=type` with no space should list every
type in every space or refuse and ask for one (recommendation: list, with
the space per row, since "what is a Trip" is a cross-space question);
whether type rows should carry an object count (one extra query per row —
probably only on the per-space form); and the `read` full cut rule (by
block count or by chars).

---

## 7. Client-side (anytype-swift) — for the report's author, not this repo

- Print the heart commit (`Lib.xcframework` build id) in the `[LocalAI]`
  console header, and rebuild the framework from HEAD before the next run.
- Repair budget: count `invalid_arguments` outside the two-repair budget;
  keep `tool_error` inside it.
- Use the manifest's served `instructions` (and `ToolsPreamble`) instead of
  `LocalAIRuntimeTools.instructions`; stop sending the EBNF and the example
  list.
- Call `ToolsSetContext` with the current space, locale and time zone at
  session start and on space switch; on a working-space change under D2(c),
  rebuild the `LanguageModelSession` with the new tools and the carried
  transcript.
- Handle `exceededContextWindowSize`: summarise or drop the oldest tool
  outputs and start a new session with the trimmed transcript; today the
  policy is `.preserveTranscript` and the error surfaces as a failed turn.
- Run the §1.1 one-call check on the phone once, on the same build, and
  attach the answer to the report.

---

## 8. Sequencing

| Phase | Content | Exit criterion |
|---|---|---|
| 0 — measure | AFM bridge (D5); eval cases §5.2 added and failing; byte-budget test added and failing; framework rebuilt from HEAD; baseline run: AFM + gemma4:e2b, small tier, 17 existing + 6 new tasks, n=3 | numbers in `eval-out/` with the heart commit in the header |
| 1 — correctness | §3.1 cross-space find + `type=type`; §3.2 refusal text + no mint on reads; §3.3 manifest text + grammar line; §3.4 codes + space default | cases 1–4 and 6 pass on gemma4:e2b; §3.2 unit test fails on revert |
| 2 — efficiency | §4.1 text budget; §4.2 result caps; §4.4 context + preamble | fixed cost ≤ 4.5 KB; case 5 passes; AFM completes the report's two turns |
| 3 — session manifest | §4.5 if the phase-0 probe shows the enum is enforced | AFM `TASK` rate 0 on case 3 |
| 4 — record | `core/api/APIV2.md` §8.58 "decisions as built"; `docs/local-ai-tools.md` on the Swift side updated with the new exports | — |

Phases 1 and 2 are independent of each other and can run in parallel;
both depend on phase 0 only for the numbers, not for the code.

---

## Appendix — things checked and found fine

- `spaceArg` already accepts the `Name — id` row shape; short space refs
  (`xjwg44`) are what `spaces` prints, so the §8.34 truncation class is
  closed for AFM.
- The route middleware resolves short refs on every `/v2/spaces/:space_id`
  route, on HEAD and on the device build alike (`core/api/v2/spaceref.go`,
  `setRouteParam`).
- `isUninstalled != true` matches type objects that never carried the field
  (any-store compares a missing value as null, and null ≠ true), so old
  accounts are not filtered out by that clause.
- The A2 option guard, the case fold and the block-locate on `edit_text`
  are all on the small tier and cost no context.
- `Host.Call` serializes through `Runner.mu`; nothing here needs
  concurrency changes.
- The GBNF artifacts are unused by AFM (no grammar hook) and by Ollama/LM
  Studio (they constrain from JSON schema); they stay in the manifest for
  llama.cpp hosts and cost nothing in the session.
