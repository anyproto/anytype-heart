# ImportV2 Planner — Always Mint Types, Never Merge Vocabularies

Status: design, approved 2026-08-05. Supersedes the type/property targeting rules in
`docs/ImportV2LLM.md` §5 (prompt) and §4 (application rules). Everything else in that
document — the plan phase's position inside `Convert`, the failure model, the trust
boundary — is unchanged.

## 1. Why

The current planner is told to prefer bundled types and to merge same-meaning properties
across containers. Both instructions are wrong, and the second one corrupts imports today.

### The merge defect (live, reproduced)

`llmplan/prompt.go` instructs: *"Merge same-meaning properties across containers by giving
them the same target key."* `schemaplan.CustomRelationKey` then mints one relation per plan
key — deterministically, across every container that names it. That identity is documented
as the feature that makes cross-container merges work. For select vocabularies it is a bug.

A live import of a real 37-container workspace (2026-08-04, gpt-4o-mini) produced a single
`Category` relation shared by four unrelated databases — Launch Checklist, Launch Tracker,
Meal Calendar DB, Recipe SB — with one merged option pool:

```
Beta Testing · Breakfast · Brunch · Dinner · Documentation · Landing Page · lunch · Lunch
· Marketing · Product · Sales · snack · Snack · Social Media · Supper
```

Every recipe's Category dropdown offers "Social Media"; every launch task offers "Brunch".
A kanban grouped by Category on either type sprouts the other's empty columns. The same
collision hit `Owner` (2 containers), `Date` (2), `Project` (4).

This is the failure the use-case authoring guidance already names: *a property is one object
for the whole space, not one per type; two types sharing a select share one option pool.*
Lifecycle selects — status, stage, category, priority, phase — are precisely where the
cross-type union is not a benefit.

### Bundled types are the wrong target

Reusing a bundled type key reshapes the built-in type space-wide and hands it to a migration
that can rewrite its featured properties on the next space load. Beyond that, bundled types
carry a fixed, readonly relation set that rarely matches real data: bundled `Task` is
`Tag, Assignee, Done, DueDate, LinkedProjects, Status`, while the same workspace's "90 Day
Sprint Planner" has Actual Hours, Assigned To, Dependencies, Due Date, Estimated Hours,
Notes, Priority, Sprint, Start Date, Status and Tags — an overlap of three. Typing that onto
bundled `task` discards most of what makes it that database.

There is a secondary benefit: "prefer bundled when the meaning matches" is a judgment call,
and judgment is where plan instability lives. Identical evidence at temperature 0 produced
13 containers typed on one run and 4 on the next. A rule with no judgment in it should be
steadier.

## 2. Goals

- A planned container gets a type built from its own properties, always.
- Two containers' select properties can never share an option pool.
- Bundled relation targets are limited to those that are safe and load-bearing.
- A minted type looks designed, not generated.

**Non-goals.** Changing the plan phase's position, the failure model, the per-entry
degradation model, or anything about how plans reach objects. No persisted-state migration
is needed: plans are per-run inputs, never stored.

## 3. Design

### 3.1 Types: always mint — but one per *kind*, not one per container

Remove `bundledTypeTargets` from the prompt. Every container the planner is confident about
gets a `TypeDefinition` with its own key; `CustomTypeKey` mints the emitted key as today.

**Revised 2026-08-06 (user ruling).** "Always mint" must not become "one type per database".
Containers holding the same kind of thing should share one type — three task trackers are all
tasks. A Notion database already becomes its own collection regardless of its type
(`notion/database.go:116`), so sharing a type yields the right model: **N collections over one
type**, each database keeping its identity as a list while its rows share a shape. Minting a
near-duplicate type per database would fragment the space instead.

The prompt therefore asks for one type per kind and says containers of one kind *should* share
it. Sharing is a first-class case, not an edge case — which is what makes the scoping in §3.2
scope by *type* rather than by container.

`sanitizeNewTypes` currently drops a definition whose key collides with a bundled type
("the bundled type wins"). That changes: such a definition is re-keyed rather than dropped,
so the model spelling its type `task` still yields a working minted type instead of silently
losing the container.

**Corrected during implementation.** An earlier draft also made a container *naming* a
bundled type key a dropped entry. That was wrong and broke every no-LLM path: the naive
planner's verdicts are all bundled type keys, so the rule dropped every one of them. Two
different things were conflated:

- *Defining* a type document under a bundled key reshapes the built-in type space-wide —
  dangerous, and re-keyed.
- *Pointing* a container's pages at a bundled type changes nothing about that type — safe,
  and exactly what `typesuggest` has always done.

Always-mint is therefore enforced in the **prompt**, which governs what the LLM proposes.
`Sanitize` also governs the naive planner, so it stays policy-neutral here and enforces only
the invariants that hold for every planner.

### 3.2 Properties belong to their type; sharing is whitelisted

**Revised 2026-08-06 (user ruling), broader than the original select-only rule.** A property
belongs to the type that declares it. If two databases are different types, their "Status" is
two different properties. Sharing one property across types is the *exception*, and it is
requested explicitly by naming a whitelisted bundled target (§3.3) — `tag` and `genre` exist
for exactly that.

This applies to every format, not only selects. Selects are merely where the damage is
loudest, because a shared select shares an option pool.

The scope is the container's type when it has one, so two containers the plan calls the same
kind of thing do share properties; otherwise the container itself.

**It also applies with no plan at all.** The Notion converter deduped relations by
`name + format`, so same-named properties in different databases collapsed into one relation
on the plain, non-LLM path. In the 2026-08-04 workspace `Status` appeared in **18** databases
and shared a single option pool between them; `Category` in 5, `Type` in 4. Dedup is now by
notion property id only — which still collapses the one case that must (a database and its
pages describing the same property) — with the bundled Tag redirect as the whitelisted
exception. This changes the cassette fidelity snapshot from 762 to 865 objects: relations
140→208, options 178→213, content unchanged at 444.

**Not changed: markdown/Obsidian's schema-less front matter**, which keys relations by
property name across the whole vault. Without a plan, an Obsidian folder is not a type, so
there is nothing to tie the property to; with a plan it goes through the scoping above.
Worth revisiting if folder containers become types by default.

#### Original rule (superseded, kept for the reasoning)

The rule, in two halves:

**Prompt.** Ask for per-container names — `recipeCategory`, `launchCategory`, `taskStatus`,
`projectStatus` — and state why. Name the one exception: a genuinely space-wide vocabulary
belongs on bundled `tag`.

**`Sanitize` enforces it.** The plan is untrusted input, so the prompt is a suggestion and
the sanitizer is the guarantee. When a custom target's settled format is a list format
(`status` or `tag`), its plan key is rewritten to a container-scoped key before the emitted
relation is minted. Nothing is dropped and no information is lost — the model's naming
intent survives, and the collision becomes structurally impossible rather than merely
discouraged.

Shared keys remain legal for every non-list format. Merging four containers' `Project`
object-links onto one relation is correct and useful; only option pools are the problem.

Two consequences to respect in implementation:

- **`anchors` keys by the scoped key.** The map that fixes one format per target
  (`sanitize.go:114`) must see scoped keys, or a cross-container format conflict is reported
  for relations that will never actually be shared.
- **Type definitions must agree with their container.** `TypeDefinition.Properties[].Key`
  is resolved through the same `CustomRelationKey`, so a type declaring `category` while its
  container scoped to `category@<id>` would leave the type's recommended relations pointing
  at a relation nobody emits. Scoping must be resolved consistently across both, and pinned
  by a test.

### 3.3 Shrink the bundled relation allowlist

`AllowedBundledTargets` goes from ten entries to six. Dropped, because under always-mint they
cannot succeed:

| Dropped | Why |
|---|---|
| `assignee` | `objects` targeting `contact`/`participant` — minted people types do not satisfy it |
| `author` | same target constraint; a text Author cannot map to an `objects` relation |
| `company` | `objects`, but Notion's Company is normally text — already rejected live |
| `priority` | `number`, but Notion's Priority is nearly always a select — already rejected live |

Kept: `dueDate`, `done`, `email`, `phone`, `tag`, `genre`. `done` is non-negotiable —
`ObjectType_todo` declares it a required relation, and the title-row checkbox reads it, so a
minted todo-layout type whose completion checkbox maps elsewhere renders a dead checkbox.
`tag` and `genre` are kept precisely because their space-wide union is the point.

`status` stays excluded. It is a genuine select, but one pool per space: admitting it would
have merged fifteen databases' Status vocabularies in the live run. §3.2's scoped custom keys
are how a per-database status gets imported.

### 3.4 A single-database type replaces its collection

**Added 2026-08-06 (user ruling).** When exactly one database backs a minted type, "all
objects of this type" and "members of this collection" are the same list, so emitting both
duplicates it. The type takes the database's place instead: its **source key** — so every
`child_database` block, `link_to_page` and mention still resolves — plus its description,
timestamps, icon and root candidacy. It keeps its own name, since the model named the *kind*
and that reads better than the database's title. Types backed by several databases keep their
collections, because there the collection is what tells the lists apart.

This works because the engine routes by `SbType`, not source key (`engine/sink.go:35`): a
type object always takes the derived path, so it never collides with the minted-id space.
`AssignDerived` re-registers the source key onto the derived id, so references follow, and
the pass-1 claim is not left dangling for the reconciliation to report. That interaction is
pinned by `identity.TestDerivedAdoptsClaimedSourceKey` rather than left to inference.

**Sanity check — containers that share members keep their collections.** A page carries
exactly one type, so two containers holding the same page cannot both be represented by
types: whichever type the page did not take would lose that member with nothing left to
record it. Notion reaches this whenever `/search` returns both a database stub and its own
data source, since `databaseMembers` matches the pages under either id. Such containers are
excluded from the replacement.

**The type inherits the collection's job of listing every property.** The collection's dataview
made every schema property a visible column, whatever the plan named; a type recommends only
what its definition declares, and the prompt asks for 2-4 featured properties, which actively
invites a subset. So a database's remaining schema relations are appended to the type as
regular (non-featured) recommended relations — the model still chooses what is featured, and
nothing imported goes unlisted. Concretely, a "Tasks" database of Priority/Tags/Score whose
plan declares only Score used to yield a type recommending Score alone, while Priority and
Tags sat on every row unlisted.

That backfill is why the type is emitted from `convertDatabase` rather than the plan phase: it
can only name relations that already exist. `ResolveRef` reports an unknown source key as
missing rather than waiting for it (`identity/service.go:81-96`), so definitions-before-use is
load-bearing — a type naming a relation emitted later would degrade to `missingTarget`.

What is genuinely lost: the collection's dataview *block*. That is only the factory's default
view — not Notion's saved views or filters, which this importer never reproduced.

### 3.5 Minted types should look designed

`schemaplan.TypeObject` sets name, layout and recommended/featured relations. It sets no
plural name and no icon, while every bundled type has both. That was tolerable when minting
was the exception; as the only path it means every imported type reads as unfinished.

Add `pluralName` and an icon to `TypeDefinition`, carried through the wire schema and emitted
in `TypeObject`. Both are model-supplied and therefore untrusted — see §4.

Featured properties also matter more now: `Featured` decides the object header, and it is the
main lever the model has to make a type feel intentional. The prompt should ask for two to
four featured properties per type rather than leaving the field incidental.

### 3.6 Bound plan-supplied display names

Already-known gap, folded in here because §3.4 widens it. `notion/properties.go:160-168`
writes `plan.Name` verbatim into a custom relation's name and `schemaplan/emit.go:83` does
the same for a type name; only emptiness is checked. Both gpt-4o-mini and gpt-4.1 write
explanatory prose into that field — one live run produced a relation literally named
`"Contact Type remapped to Contact Type (select)"`.

`Sanitize` gains a name guard applied to every plan-supplied display name (type names, plural
names, relation names): trim surrounding whitespace, strip control characters, cap at 64
runes, and fall back to the source property or container name when the result is empty or was
truncated mid-word. `SummarizeError` already does the equivalent for error text (first line,
200 runes); this closes the same hole on the names users actually see.

## 4. Trust boundary

Unchanged in principle, wider in surface. Everything the model supplies is untrusted: keys,
formats, names, plural names, icons. Every new field in §3.4 needs a sanitizer rule before it
reaches a snapshot. The icon is a closed vocabulary — `core/api/model/icon.go` defines
`IconName` with 390 constants — so it must be validated as a member of that set and dropped
otherwise, never passed through as a free string. Offering that list to the model is also
what makes an icon worth asking for: an unconstrained guess would mostly miss.

The §3.2 scoping rule is deliberately a *rewrite*, not a *rejection*. Rejections lose user
data and are reported as warnings the user must interpret; a rewrite silently does the right
thing. Reserve rejection for entries that cannot be repaired.

## 5. Testing

**Archetype corpus** (new, opt-in live). `[]schemaplan.ContainerSchema` fixtures fed straight
to `llmplan.Plan` and `Sanitize` — no HTTP, no converter, no Notion. Each archetype carries
must-hold assertions and the run reports metrics (rejection rate, bundled-hit rate, %
properties mapped, latency). Because the input is source-agnostic, the corpus covers the
markdown/Obsidian planner path for free.

Seed archetypes: PARA/second-brain, CRM, sprint tracker, recipe box, bug tracker, reading
list, plus adversarial ones — two near-identically-named databases (which produced the
observed property-id cross-contamination) and two databases with same-named select
properties.

The corpus's first invariant is the regression test for the defect in §1: **two containers
with same-named select properties must not share a relation key.** It needs no Notion
fixture.

Skipped without `OPENAI_API_KEY`, per the decision that these run opt-in rather than
cassette-replayed in CI. The trade-off accepted: an ordinary CI run does not catch prompt or
sanitizer regressions, so the corpus must be run deliberately when either changes.

**Deterministic pins** (CI). The corpus stops at `Sanitize`, so it cannot prove the split
survives into emitted relations. One scripted-plan test in `notion/plan_test.go` — an
established pattern there — asserts that two containers mapping same-named selects emit two
relations with disjoint option sets. Further scripted tests pin: a bundled type key for a
container is dropped; a bundled-keyed type definition is re-keyed; a dropped allowlist target
is rejected; a prose name is bounded; a type definition and its container agree on a scoped
key.

**Cassette parity.** The no-plan path must stay byte-identical — the naive planner still
reproduces the type suggestor's verdicts verbatim, and an import without `aiParams` is
unchanged. The existing cassette fidelity snapshot is the guard.

## 6. Risks

- **Plans get larger.** Always minting means a type definition per container; a 37-container
  workspace produces 37 types where today it might produce a handful. On real workspaces the
  plan call already consumes 76–80s of a 90s budget, and more output tokens will push it
  further. The budget may need raising, and `maxCompletionTokens` (8192) may bind. Measure
  before assuming — and note that `llmclient` never inspects `finish_reason`, so a truncated
  completion currently surfaces as a parse error rather than as truncation. Worth fixing
  alongside, since this change makes truncation more likely.
- **More custom relations.** Scoping select keys per container multiplies relation objects.
  Correct, but it grows the space's property list; the per-type grouping in the UI is what
  makes this readable.
- **Losing bundled-type familiarity.** A container that genuinely is just Contacts now mints
  its own type instead of landing on bundled Contact. Accepted deliberately.

## 7. Out of scope

Notion fixture infrastructure — the hand-authored builder and the seeded generator for scale
and fault injection — is a separate spec. It shares nothing with this work except the
archetype corpus, which is defined here and needs no Notion fixture at all.

The cassette remains the real-shape oracle in both specs: a fake built on our own structs
cannot catch our structs being wrong, which is exactly the class of bug the 2026-07-05
recording found.
