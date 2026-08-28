# Name addressing — design study and decision

**Status: DECIDED, third revision (study document; no code or spec change
accompanies it).** GO-7383, 2026-08-28. Two earlier decision states are
superseded — normalized name-derived keys (rev 1) and raw-custom /
snake-bundled mixed (rev 2) — and the appendix records every argument that
died between revisions with the evidence that killed it, per the spec's
own §15 practice.

**The decision, in one line:** every property and type — bundled and
custom alike — is spelled by its **display name, raw** (NFC, verbatim):
`"Tag"`, `"Creation date"`, `"Due Date 2"`, `"Задача"`. One uniform rule,
no derived identifier anywhere in the format, no table a writer must
classify against. `api_object_key` leaves the format's read path entirely.
The prerequisite is a small bundle name cleanup Anytype owns (§5.2). The
reasoning is §4, the concrete rule §5.

**Scope:** the FORMAT only. The API surface may keep or adopt
`apiObjectKey` independently — the author has explicitly separated the two
decisions — so nothing below depends on the API and the format agreeing.

**The evidence base.** (a) A 155-space / 28,560-document native corpus and
a 38,123-snapshot export corpus from one production account (caveats: one
account, several agent-generated template spaces inflating bulk-duplicate
rates, English-dominant — 8 emoji-bearing names, zero Cyrillic/CJK among
used custom properties, so the non-Latin argument is architectural). (b) An
A/B generation eval on the map-key convention: `google/gemma-4-e4b`
primary, 192 generations / 208 property observations per arm,
byte-identical except one instruction; `prism-ml/bonsai-27b` secondary, 9
paired cells; graded by `anyblockeval`; independently recomputed.
Artifacts: `/Users/roman/.claude/jobs/5792c699/tmp/keyeval/`. (c) Direct
verification of the bundle tables and both client repos where a claim
depended on them (§2, appendix A7).

---

## 1. What the format does today

A key's wire spelling comes from one of two authorities (SPEC §3):

1. **Bundled keys** spell the derived api slug from the code table that
   ships with every reader (`bundle.ApiSlug`, i.e. `strcase.ToSnake`),
   except the sixteen keys the v0.38 alias table respells
   (`featuredRelations` → `featured_properties`; `alias.go`).
2. **Space-minted keys** spell what the space says, through a ladder, first
   answer wins (`PropertyLabel`, label.go:82; `keyLabel`, label.go:95): the
   stored `apiObjectKey` when it is a legal key — *re-spelled by the display
   name when the two are one fold class* (v0.30) — else the slug normalized,
   else the display **name** normalized (`normalizeKeyLabel`, label.go:181),
   else nothing, and the stored key is written verbatim.

So the format is already name-aware: the name breaks ties inside a fold
class and is the whole answer for slugless keys. SPEC §3 currently states
the priority this decision inverts: *"A minted `apiObjectKey` outranks a
display name, and is the rename-stable address."*

The machinery around the spelling, all of which survives this change:

- **Resolution chain** (SPEC §3): document legend → exact stored key
  (verbatim-first) → bundled table → verbatim; plus the fold layer as the
  forgiving step. The legend (`property_internal_keys` /
  `type_internal_keys`) is written exhaustively — one entry for every
  spelling the bundled table does not bind — so every exported document is
  invertible offline, including after deletes reassign a freed spelling
  (the corpse-after-export hole, SPEC §3).
- **Per-document term ledger** (`propertySlug`, export.go:357;
  `seedTermLedger`, export.go:392): one term, one key, document-wide;
  verbatim-first; a contested term degrades to the stored key.
- **Space-level vocabulary** (`storeresolver/keyvocab.go`): two-pass bind —
  explicit slug claims first (`add`, :128), name-derived labels take only
  what is still free (`addDerived`, :175); two equal claims on one spelling
  drop it from both directions ("the git rule", `bind`, :194).
- **Shape guard** (`writableSlug`, export.go:513): a spelling outside the
  writable-key rule (non-empty, ≤128 bytes, no control characters), or
  `id`/`type`, or a denied key's slug, is refused and the stored key
  written instead — the I1 guarantee's front line.

### 1a. Where spellings appear — and who writes them

**The only author-written map is `properties`.** The authoring schema
(`schema/authoring/object.schema.json`) is `additionalProperties: false`
and admits none of `option_ids`, `property_internal_keys`,
`type_internal_keys` — the legends are export-side machinery a model never
writes (verified against the schema). Map-key slots: `properties`
(export.go:1652 — authored), the two key legends (export.go:589), and
`option_ids` outer keys (optionrefs.go:116 — all three export-only).
Value slots: dataview `properties[].property` (dataview.go:42),
`group_by`/`cover_property`/`end_property` (dataview.go:80-82), sorts
(dataview.go:181), filters (dataview.go:239), columns (dataview.go:621),
the `property` block (export.go:2147), a link block's `properties` list
(export.go:2098), a type's `property_definitions[].property`
(typeproperties.go:234), and the type-key slots (`type`, `template_for`,
`object_types`).

**The §6.2.1 identifier grammar is not a document constraint.** In
documents a filter's property is an ordinary JSON string value; the
identifier grammar binds only the compact filter *string*, which ships
solely on the API v2 request surface (`filterstring`; the document-side
`filter` field stays reserved). Raw-name keys are legal in every document
slot **today** — the eval's validity rate was flat by construction — and
the schema needs no change. What raw names cannot be is *bare keys in a
compact filter string*; that surface is the API's, and the API decision is
separate.

### 1b. What `apiObjectKey` is, on the evidence

Minted once from the name at creation (`bundle.MintApiSlugFromName`:
transliterate → snake-case → sanitize to `^[a-zA-Z0-9_]+$`), then frozen.

- 942 custom relation objects; 585 carry a real slug; **54 of 585 (9.2%)
  are no longer fold-derivable from the current name** — including genuine
  drift where the frozen key now misleads: `quantity` for a property named
  "Rating", `ffff` for "Roadmap Source", `membership` for "Day Pass",
  `date_of_birth` for "Birthday", `website` for "Link". Options: 45 of 514
  (8.8%), same order as the prior 22-of-514 measurement.
- **Custom types: 161 objects, zero carry a slug.** The type namespace
  already runs without the slug in production.
- The transliteration arm is not merely lossy but wrong: `作業内容`
  (Japanese) → `zuo_ye_nei_rong` (Chinese readings), `완료` → `wanryo`,
  `مهمة` → `mhm@`. The format's own label rule already preserves these
  scripts; only the api-slug path degrades them.

---

## 2. Measurements

Custom keys actually used by documents (legend ground truth, 28,560 docs):

| population | properties | types |
|---|---|---|
| distinct (space, custom key) used | 1,140 | 222 |
| with a known display name | 767 | 130 |
| nameless (corpse — entity deleted, key still carried) | 373 | 92 |
| today spelled verbatim stored key | 583 (301 bson-shaped) | 115 (79 bson) |

Name shape, over the 767 named: 390 contain spaces, 61 ASCII punctuation,
8 emoji/non-ASCII; 6 carry a leading/trailing space (`'Email 📧 '`); 2
carry an invisible variation selector; **0** contain quotes, backslashes
or control characters; **0** exceed the 128-byte key bound; **0** change
bytes under NFC; 11 near-twin groups differ only by case or edge
whitespace (`Role`/`role`).

Collision domain under raw names (exact byte equality after NFC): **16
duplicate-name groups, 34 custom properties** across 77 dictionary spaces
(vs 21 groups / 42 under normalized labels — case and emoji variants
become distinct keys), dominated by bulk API creation plus two spaces
holding properties literally named `#` (×2, ×4). **25 custom names equal a
bundled relation NAME** (`Emoji` ×5, `Priority` ×4, `URL` ×3, `Status` ×3,
`Created by`, `Description`…); 4 of 130 named custom types equal a bundled
type name. Per document: two co-used keys tie on one raw name in 35 of
28,560 docs (0.12%), all one space's exact-duplicate `Projects` pair.

**The bundled table, measured for uniform name addressing:**

- **143 of 194 bundled relation names contain spaces; 0 of 194 are
  byte-equal to their current wire spelling** (even `tag` → "Tag") — so
  uniform raw re-spells the bundled surface completely, and every document
  is touched: `creator` appears in **28,560 of 28,560** documents, `name`
  in 28,483, `resolved_layout` 26,800, `created_date` 26,066.
- **Name uniqueness is two renames away.** Exact duplicates among
  wire-reachable entries: `genre`/`audioGenre` (both "Genre") and types
  `space`/`spaceView` (both "Space"). The nine relations sharing
  "Underlying file id" are hidden transients — 0 of 28,560 documents spell
  any of them (verified for `file_id`, `file_variant_ids`,
  `file_source_checksum`). Fold-class collisions across different keys
  (name-vs-name and name-vs-key): **only those same two groups** — the
  uniqueness CI rule is nearly free today.
- **The property-vocabulary cleanup is 11 names**, not 16 keys: 10
  relation names contain the word "relation" ("Relation key", "Featured
  Relations", "Recommended relations", …) plus the type "Relation option".
  The rest of the v0.38 alias population already carries clean names —
  `relationFormat` is named "Format", and the `relation` TYPE is already
  named **"Property"** in the bundle (verified), so a property document
  spells `type: "Property"` with no rename at all.
- No bundled relation is named `id` or `type` (they are "Anytype ID" and
  "Object type"), so the §2 member refusals collide with nothing.
- 84 of 194 bundled names say a different word than their key's snake
  spelling (`creator` / "Created by") — under uniform raw this is not a
  con but the point: the name is the surface users see.

**The mixed-rule measurement (why rev 2 died):** under raw-custom /
snake-bundled, **30.1% of documents (8,600) visibly mix** the two
conventions inside one `properties` map, and every document carries
bundled keys — so the writer's rule was not "spell the name" but "spell
the name unless the property is one of ~194 keys you must recognise from
memory".

Churn and bytes: custom-key spellings total 1.16 MB of the 211 MB corpus;
raw names shrink them by 68.2 KB. Under uniform raw the bundled surface
re-spells too, so **effectively 100% of documents re-baseline** (§7).
Bytes remain a non-argument; bundled names are on average slightly longer
than their slugs ("Creation date" vs `created_date`), single-digit KB per
thousand documents either way.

---

## 3. The eval — generation fidelity is settled

A/B on the map-key convention, everything byte-identical except one
instruction: arm A "snake_case derived from the name", arm B "the name,
raw". Primary model 192 generations, 208 property observations per arm.

| class | A (snake_case) | B (raw name) |
|---|---|---|
| multi_word | 56/56 | 56/56 |
| control | 60/64 | 64/64 |
| non_latin | 24/48 † | 47/48 |
| punctuation | 36/40 | 32/40 |
| **primary (excl. non_latin)** | **152/160** | **152/160** — p = 1.0, CI [−4.7%, +4.8%] |

† Disqualified as an A/B row: arm A's instruction does not determine a
target spelling for `완료`; scored leniently A is 100% and the overall
result is the p = 1.0 above.

What the eval established:

- **192/192 generations parsed as JSON. Zero dropped spaces (`DueDate`),
  zero case drift, zero edge whitespace, zero non-NFC keys, zero invisible
  format characters, zero quoting anomalies — in both arms.** In B:
  `Due Date` 24/24, `P2P Sync` 16/16, `More Information` 16/16,
  `Manual export & import` 24/24. Copying a raw name byte-exactly is not
  where models fail.
- **The measured hazards run toward NORMALIZATION.** Where arm A had to
  derive, the model improvised — `완료` → `completed` ×11 / `done` ×1,
  `Дата выполнения` → `due_date` ×8 — and improvised *inconsistently
  across documents* (`due_date` 8/8 in one task, `дата_выполнения` 8/8 in
  another). Cross-document key stability: A 85.7%, B 96.6%. The secondary
  model additionally produced malformed derived slugs
  (`manual_export__import`, `lists___in_work`).
- **Cross-slot consistency: B 32/32, A 28/32** — every A divergence the
  same shape: snake key in `properties`, display name in the filter/sort
  value. In this format that silently unbinds the view from the property.
  B cannot make that error; there is one spelling to remember.
- **B's one genuine failure mode: the copy boundary.** `Lists [in work]` →
  `Lists [in work] (text)`, gluing an annotation from the task line.
  Partly confounded with the task-line syntax; not reproduced by the
  secondary model; real nonetheless.
- Secondary model agrees in shape: A 13/19, B 19/19.

What the eval did **not** establish, carried as open risk (§8): emission
only — no read-back; no pathological names (every eval name was NFC-clean,
unduplicated, no edge whitespace, no case twins); no phantom-member test;
validity flat by construction.

---

## 4. The decision, and why

**One uniform rule: every property and type is spelled by its display
name, raw. Bundled included.**

This is the third decision state, and each supersession was forced by
evidence, so the chain is worth stating. Rev 1 (normalized name-derived
keys) fell to the eval: derivation is the measured error site, and a
normalized scheme keeps a derivation step in every model's write path
forever. Rev 2 (raw custom, snake bundled) fell to two things:

1. **Its factual pillar was false.** Rev 2 kept bundled snake "because
   bundled names are localized, so the key is not the user's word".
   Verified false in both repos: `pkg/lib/bundle/relations.json` holds
   fixed English strings and nothing in `pkg/lib/bundle/` or
   `objectcreator/` localizes them; the client's `relationName0..7`
   strings are relation *format* names, not relation names. The author
   confirms: bundled properties are not localized at this moment.
   (Appendix A7.)
2. **The mixed rule reintroduces, in worse form, the very failure the
   raw choice was made to remove.** Rev 2's case for raw was that
   normalization is a derivation step models get wrong; but a mixed
   convention imposes a **classification** step — "is this one of ~194
   bundled keys?" — on every write, and classification is strictly harder
   than derivation: derivation is computable from the name alone, while
   classification requires a table the writer must have memorized. The
   failure is silent and plausible — `"Created Date"` is a valid key
   today, so an example-less writer's wrong guess validates and mints a
   phantom — and it is not an edge: every document carries bundled keys
   (`creator` 28,560/28,560) and 30.1% of documents would visibly mix the
   two conventions in one map. The author's objection, measured and
   sustained. (Appendix A8.)

Between the two *uniform* options that remain:

**(b) all snake_case** (status quo) keeps every cost this study has
measured against derivation — models improvise normalizations and
improvise them inconsistently (85.7% cross-document stability), diverge
between key and value slots in the way that silently unbinds views
(28/32), and the transliteration arm mangles non-Latin names — and it
keeps `apiObjectKey` in the user's mental model of the format. Its
advantages (identifier-grammar keys, bare filter-string expressibility,
zero migration) are real, API-scoped or one-time respectively.

**(a) all raw names** has one rule a writer can hold with no table and no
transform: *a key is the property's name*. The eval says copying names is
a solved behavior at 4B scale and that the one spelling-per-concept
property (B 32/32 cross-slot) is structurally guaranteed. The user flow
carries no derived identifier at all. Non-Latin names survive verbatim.
The costs are real and **owned by Anytype rather than paid by every
writer forever**: a bundle name cleanup (two renames, §5.2), a name-freeze
policy for bundled entries after freeze (renames become breaking and need
a shipped alias — the v0.38 alias mechanism, generalized and held in
reserve), and the largest possible one-time re-baseline (every document,
§7). One residual writer hazard survives: an example-less writer must
still *know* the bundled vocabulary to name it ("Created Date" guessed
for "Creation date" misses) — but that hazard exists under every scheme
(a guessed `created_at` misses `created_date` today), no scheme makes
guessing safe, and the fold layer actually forgives the most likely
guess shape: a guess matching the stored key's fold class
(`"Created Date"` folds to `createddate` = `createdDate`'s class) still
resolves. The §5.7(iii) unknown-term warning names the rest.

**Decision: (a).** The determining structure: (b)'s costs are permanent,
per-writer, and measured; (a)'s costs are one-time, Anytype-controlled,
and bounded by measurements in §2. If the read-back follow-up (§8.1)
surprises, rev 1's normalized machinery remains the measured fallback.

---

## 5. The concrete change

1. **One spelling rule.** The spelling of every property key and type key,
   in every slot, is the entity's display name, NFC-normalized, otherwise
   verbatim — no case fold, no separator collapse, no transliteration, no
   grammar escape. For space-minted entities the name is the space's; for
   bundled entities it is the shipped table's (`relations.json` /
   `types.json` names become the wire vocabulary, replacing the derived
   slug table). `properties` keys, both legends' keys, `option_ids` outer
   keys, all dataview/link/property-block value slots, and the type slots
   spell the same way: `"Creation date"`, `group_by: "Status"`,
   `type: "Page"`, `type: "Property"` on a property document.
   `PropertyLabel`/`TypeLabel` collapse to "NFC(name), else nothing";
   `normalizeKeyLabel` retires from the format path.
2. **Bundle name cleanup — the prerequisite, all Anytype-owned.**
   (i) Uniqueness among wire-reachable entries becomes a hard CI rule
   (extending the dictionarywarn guards): no two names equal, no name in
   another key's fold class. Today that rule is two renames away —
   `genre`/`audioGenre` ("Genre" ×2) and `space`/`spaceView` ("Space" ×2);
   the nine "Underlying file id" transients never reach the wire (measured:
   0 of 28,560 documents spell any of them) and need nothing.

   (ii) **No property-vocabulary renames are required.** An earlier revision
   counted ten or more — "Relation key" → "Property key" and its neighbours.
   Measured, that cost is zero: not one of the eight `relation*` vocabulary
   keys appears as a property KEY in any of the 28,560 corpus documents.
   They are lifted into `property_settings` under fixed member names
   (`object_types`, `include_time`, `max_values`), so their bundled display
   names never become wire spellings and renaming them buys the format
   nothing. They may still be renamed for product reasons; that is not this
   change's business.

   **So the prerequisite is two renames**, both Anytype-owned, both
   mechanical: `audioGenre` "Genre" → "Audio genre" (leaving `genre` as
   "Genre"), and the `spaceView` type "Space" → "Space view" (leaving
   `space` as "Space"; both are `hidden: true`, so no user sees the second).
   The v0.38 wire alias table still dies — its job moves into the names
   themselves and `alias.go` with it — but it does so without a rename
   programme behind it.
3. **Fallbacks — and the failure class raw naming DELETES.** Only three
   things still fall back: an empty name, a name over 128 bytes, and a name
   carrying control characters (the writable-key rule, unchanged). Each
   degrades through the collision rule to the stored key verbatim, always
   its own address. `api_object_key` is not a fallback; it leaves the
   ladder entirely (the corpus's 373 nameless keys are corpses, spelled
   verbatim today and unchanged).

   Everything else that used to need machinery now needs none, and this is
   one of the strongest arguments for the change. Under a NORMALIZED
   spelling a name could fail to produce a key at all — `#`, `☕` and `C++`
   normalize to the empty string or to `c`, so the format carried an
   empty-normalization fallback, a leading-`_` escape for digit-initial and
   keyword names (`50% done` → `_50_done`, `All` → `_all`), and a
   stored-key fallback for names that spelled to nothing. Raw naming has no
   normalization step, so none of these can arise. Verified against the
   current codec: `"#"`, `"☕"`, `"C++"` and `"50% done"` are each a valid
   property key exactly as written. **The escapes, the leading-`_`
   convention and the unspellable-name fallback are all deleted, not
   reimplemented** — a rule that cannot fail needs no repair path.
4. **Collisions are resolved per DOCUMENT, not per space.** A document
   carries a map, and a map already guarantees its own keys are distinct —
   so a name that is ambiguous space-wide but appears once in this document
   spells its plain name. Disambiguation fires only where two properties
   genuinely collide *inside one document*.

   Measured, that is rare and concentrated: **60 of 28,560 documents
   (0.21%)**, across five names — `Projects` (32), `Description` (24),
   `Created by` (2), `Cost & type` (1), `What's missing` (1). An earlier
   revision suffixed every claimant of a space-wide tie, a population an
   order of magnitude larger; that was too eager.

   Where a document does collide, claimants take **(a)** the stored key
   verbatim when it is itself readable (the `Region` pair →
   `producer_region` / `wine_region`); **(b)** else `<name> (<tail6>)`,
   tail6 = the stored key's last six hex — deterministic, immutable,
   visibly synthetic; **(c)** residual tie → the full stored key. A name
   equal to `id` or `type` (§2 pre-resolution refusals) or to another live
   stored key is refused the same way. Bundled names cannot tie with each
   other by CI rule (2.i).

5. **The map-less reader resolves within the type.** An authored document
   need carry no legend, so a reader handed a bare name resolves it against
   **the declared type's properties first**. Unambiguous there — which is
   the overwhelming case, measured at **1 ambiguous type of 1,753** — and
   it is resolved. Ambiguous, or absent from the type, and the reader
   raises a loud actionable error naming the term and asking for the
   property-keys map. It never guesses and never mints a phantom on a bare
   name it cannot place.

   This is what makes the plain-name spelling safe without a legend: the
   type is the disambiguating scope, and the error is the escape hatch when
   the type is not enough.
6. **Legend and ledger unchanged.** Bundled names bind through the shipped
   table (no legend line, as today); every custom spelling still owes its
   `property_internal_keys` / `type_internal_keys` line — same entry count
   as today. The per-document term ledger keeps verbatim-first,
   first-claim, stored-key fallback. I1/I2 hold by the existing
   mechanisms: every emitted key is a writable key (0 violations in 767
   measured names; the schema already admits spaces — the eval's validity
   was flat because both spellings are legal today), and resolution still
   runs legend → verbatim → bundled(names) → fold.
7. **Accept side and continuity.** The chain is unchanged; the bundled
   table's accept half answers names instead of slugs. Legacy spellings
   keep resolving without a compatibility table: **fold(ToSnake(key)) ==
   fold(key) by construction** (ToSnake only inserts `_` and lowercases;
   the fold strips `_`, `-` and case), so every pre-change derived-slug
   spelling — `created_date` — lands in its stored key's fold class, which
   is collision-free in the bundled table once 2.i holds. Custom legacy
   spellings resolve through their exported legends, or the extended fold:
   NFC + casefold + trim + strip default-ignorable code points + drop
   `_`/`-`/spaces, answering only when exactly one candidate remains
   (bridges `due_date_2` ↔ "Due Date 2" and forgives invisible-character
   near-misses). The sixteen v0.38 alias spellings (`featured_properties`,
   …) are the one population outside this proof; §9 Q5 decides between an
   accept-only legacy table until freeze and a pre-freeze hard cut.
8. **Validation warnings, new, all cheap.** (i) A key carrying edge
   whitespace or default-ignorable characters (8 corpus names) — warn,
   never refuse. **Decided: the format does not trim or normalize these.**
   A name is carried exactly as the space holds it; if invisible characters
   are to be cleaned up, the place to do it is where a user creates or
   renames a property or type, not at the export seam — one normalization,
   applied once, at the point the value is authored rather than every time
   it is written out. Recorded as a follow-up outside this change. (ii) A written key that extends a live entity's name with
   trailing text (`Lists [in work] (text)`) — warn "annotation glued to a
   name?": the eval's one real raw-name failure shape. (iii) A term that
   resolves verbatim to a key no live entity holds — warn: the
   stale-or-guessed-name phantom, every scheme's shared hole, now named at
   the seam.
9. **`api_object_key`**: never read by the format; continues to export as
   ordinary data on property documents; remains the API surface's affair.
10. **The dictionary (§2f)** keeps stored-key entry keys (a pure function,
   unchanged); its `installed` list spells the bundled names.

---

## 6. Round trip

**I1/I2** — unchanged mechanics, §5.5. **Fixed point**: generation 2
re-derives identical spellings from an unchanged space; renames move
spellings between generations, correctly, via the legend (custom) or the
shipped table (bundled — post-freeze bundled renames require the reserved
alias mechanism, §5.2).

**Rename between export and import**, three populations:

1. **Exported documents (legend present): fully protected.** The legend
   binds the exported name to the stored key and outranks every
   vocabulary; a document written under "Budget" imports correctly after
   the property becomes "Cost".
2. **Freed-name reuse: protected by the same line.** A new property later
   named "Budget" cannot capture the old document's values.
3. **Legendless (hand-/agent-authored) documents: not protected.** A stale
   name misses silently and mints a phantom key. Accepted as the price of
   any name-addressed scheme, with §5.7(iii)'s warning as mitigation and
   one measured consolation: the likeliest bundled guesses land through
   the fold (a guessed `"Created Date"` folds onto `createdDate`'s class
   and resolves; a guessed `created_at` misses under every scheme).

**Suffix stability**: a suffixed spelling never moves while its neighbors
live; deleting one claimant un-suffixes the other on its next export —
cosmetic churn, correct via legend.

**Options precedent, correctly scoped**: select values are already raw
names with an id-hint legend (`option_ids`) — a *resolution* precedent
with measured duplicate/rename costs (SPEC §15 #3), not an *authoring*
precedent (§1a): the legends are export-only.

---

## 7. Migration

Pre-freeze, so a re-baseline, not a version bump — and this revision's
choice makes it the **largest possible** one: bundled spellings move too
(0 of 194 names byte-equal their slug; `creator` sits in every document),
so effectively **every existing document re-spells its `properties` map**
on its next export, and golden files, documentation examples, and the
snapshot comparators all regenerate once. That is the honest price of
uniformity, and the argument for deciding now: post-freeze the identical
change costs a format version plus a permanent dual-spelling accept layer.

- **Documents already written keep resolving unchanged, in both
  directions**: custom spellings through their exhaustive legends; bundled
  legacy slugs through the fold-class proof of §5.6 (no compatibility
  table needed); the sixteen v0.38 alias spellings per §9 Q5.
- **The bundle name cleanup (§5.2) lands first** — two renames plus the CI
  uniqueness guard — since the wire vocabulary becomes those names.
- External tooling written against current exports sees the re-spell once;
  the changelog entry must be loud, and the SPEC §3 authority text and
  `alias.go` are rewritten rather than patched.

---

## 8. Open risks — none gating, one with a deadline

1. **Read-back (the eval's untested half).** Whether a model can resolve,
   filter on, and re-use raw keys it is *handed* — bundled names with
   spaces now included, which raises the exposure over rev 2. **Not
   gating**: emission errors are silent corruption and emission is
   settled; read errors are loud and recoverable. **Decided: not run
   before freeze.** The reasoning that made it non-gating is the reasoning
   that makes skipping it tolerable — a read failure is visible at the
   moment it happens, and rev 1's normalized machinery remains the measured
   fallback if the field disagrees. This is the largest knowingly-accepted
   unknown in the change, and it is recorded as such rather than closed.
2. **The example-less writer.** Uniform raw removes the classification and
   derivation steps but not the need to know the vocabulary; a guessed
   bundled name that escapes the fold class mints a phantom. Mitigated by
   §5.7(iii) and by the discovery surfaces (index, dictionary, property
   documents) that already serve names; residual risk accepted under
   every scheme.
3. **Pathological names.** Edge whitespace, invisible characters, case
   twins (8 + 11 corpus cases) were absent from the eval. Mitigated by
   §5.7(i) and the §5.6 fold; accepted.
4. **The copy boundary.** One model glued an annotation onto a name;
   partly confounded, not reproduced by the second model; mitigated by
   §5.7(ii); re-check in the read-back eval.
5. **Bundled rename policy.** After freeze a bundled display rename is a
   wire-breaking change; the reserved name-alias mechanism (§5.2) must
   exist before the first such rename, and the CI uniqueness rule guards
   the table meanwhile. This is the one *new* permanent obligation
   uniform raw creates, and it is Anytype-owned.
6. **Phantom members**: untested in either arm; predates this change;
   neither widened nor narrowed on current evidence.
7. **Rename → spelling churn** and **legendless writers** (§6.3):
   inherent to name addressing, accepted.

---

## 9. Revised open questions

Closed since rev 2: the bundled/custom split (dead with its localization
pillar and the mixed-rule measurement — §4); the map-key placement and
normalized-vs-raw questions (§3–§4); the namespacing con (A1);
rename-churn acceptance (§8.7). **Deferred by scope**: the API surface's
key convention; the only format-side note is that a future document-side
compact filter field would need a quoted-key form, since raw names are not
bare identifiers.

Remaining, genuinely the author's:

1. **The sixteen v0.38 alias spellings** (`featured_properties`, …): keep
   an accept-only legacy table until freeze, or hard-cut now? (They are
   outside the fold-continuity proof; existing bundles re-export either
   way.)
2. **Import warning scope** (§5.8(iii)): every verbatim-resolved unknown
   term, or `properties` members only?
3. **Suffix shape**: `<name> (<tail6>)` as proposed, or another visible
   convention? Low stakes now that the rule fires on 0.21% of documents
   and five names — worth a glance, not a debate.

**Answered since rev 3:**

- **The bundle name cleanup list** — settled, and much smaller than rev 3
  claimed. It is **two renames**, not thirteen: `audioGenre` "Genre" →
  "Audio genre" and the `spaceView` type "Space" → "Space view" (both
  `hidden: true`, so the second is invisible to users). The property
  vocabulary needs nothing: measured, not one of the eight `relation*`
  keys appears as a property key in any of the 28,560 corpus documents —
  they are lifted into `property_settings` under fixed member names, so
  their display names never reach the wire. The nine "Underlying file id"
  transients likewise never reach it.
- **Invisible bytes at mint** — the format spells names verbatim and warns;
  it does not trim. Normalization, if wanted, belongs where a user creates
  or renames a property or type, applied once at authoring time rather
  than at every export. Recorded as a follow-up outside this change.
- **The read-back eval** — will not run before freeze. Accepted knowingly:
  a read failure is loud and recoverable, and rev 1's normalized machinery
  stays the measured fallback. The largest accepted unknown in the change.

---

## Appendix — arguments superseded across revisions, with the evidence

- **A1. "Namespaced slugs say something the name does not"
  (`restaurant_rating`).** Dead: a model filling a property always has the
  type context, so `Rating` on a Restaurant is unambiguous; the prefix
  bought *global* api-key uniqueness, which per-object resolution never
  needed. (Author's correction, rev 1 → 2.)
- **A2. "Bundled names duplicate ×9, so bundled cannot name-address."**
  Overstated: all nine "Underlying file id" relations are hidden
  transients — 0 of 28,560 documents spell any of them. The live
  duplicates are `genre`/`audioGenre` and type "Space", two renames under
  a CI rule the fold measurement shows is otherwise already satisfied.
  (rev 1 → 3.)
- **A3. "`option_ids` proves the format already accepts raw-name map
  keys."** Wrong as an *authoring* precedent: the authoring schema admits
  no legends; `properties` is the only author-written map. Kept only as a
  resolution-layer precedent. (rev 1 → 2.)
- **A4. "Raw keys risk small-model emission drift."** Falsified by the
  eval for the copying path (192/192 parsed, zero drift of any measured
  kind); the drift lives in the *derivation* path, which raw removes.
  (rev 1 → 2.)
- **A5. "Format/API divergence argues against raw."** Descoped: the API
  key convention is a separate decision by the author's instruction.
  (rev 1 → 2.)
- **A6. Rev 1's recommendation itself (normalized name-derived keys).**
  Superseded by the eval's derivation finding; its machinery (the
  normalization ladder, fold-class analysis, collision counts) remains
  measured in §2 and stands as the fallback should read-back falsify raw
  names. (rev 1 → 2.)
- **A7. "Bundled names are localized, so the snake key is the stable
  cross-locale identity."** **False**, and it was rev 2's load-bearing
  argument for the split. Verified: `pkg/lib/bundle/relations.json` holds
  fixed English strings with no i18n anywhere in `pkg/lib/bundle/` or
  `objectcreator/`; the client's `relationName0..7` are relation *format*
  names ("Text", "Number"), not relation names. Author's confirmation:
  bundled properties are not localized at this moment. (rev 2 → 3.)
- **A8. The mixed rule (raw custom / snake bundled).** Dead on
  measurement and on its own logic: every document carries bundled keys
  (`creator` 28,560/28,560), 30.1% of documents would visibly mix the two
  conventions, and the rule replaces a *derivation* step (computable from
  the name) with a *classification* step (a ~194-entry table the writer
  must memorize) whose failure — `"Created Date"` for a bundled key — is
  silent, plausible, and validates as a phantom. (Author's objection,
  rev 2 → 3.)

---

## Implementation addendum (GO-7383, landed with the v0.48 re-spell)

The decision above is built. Four calls were made where the study was
silent, under-specified, or overridden by the author; recorded here so the
study stays an honest account of what shipped.

1. **The Space rename went the other way.** §5.2 proposed `spaceView`
   "Space" → "Space view"; the author directed `space` "Space" → "Space
   settings" instead (it maps to the Workspace smartblock — the settings
   object), with `spaceView` keeping "Space". Consequence: spaceView's name
   sits in `space`'s fold class, so the CI guard pins that one class as the
   tolerated exception rather than holding "no name in another key's fold
   class" absolutely — the exact spellings stay unambiguous, only the
   forgiving layer declines for near-misses of that pair, and both keys are
   measured at 0 documents.
2. **Attribution claimants yield in the per-document ladder.** §5.4's
   "every claimant degrades" missed an asymmetry the corpus sweep caught:
   the attribution keys (`creator`, `lastModifiedBy`) are written by export
   and dropped by import, so a claimant they had suffixed un-suffixed in
   generation 2 and the round trip stopped being byte-stable — measured in
   the production space holding a custom multi_select named "Created by".
   An attribution claimant now never contests a spelling: alone it takes
   its name, contested it takes its own stored key, and the normal
   claimants keep the verdict they will re-derive without it. The §6
   fixpoint is what forced the rule.
3. **§9 Q1 (the sixteen v0.38 alias spellings): hard cut.** Pre-freeze, no
   back-compat owed, existing bundles re-export either way; an alias
   accepted "until freeze" would freeze in, the reasoning v0.41 already
   recorded for input aliases generally.
4. **§9 Q2 (import warning scope): every verbatim-resolved unknown term**,
   at every key slot, both namespaces, deduplicated per term per document —
   the seam is identical everywhere and the dedup keeps it cheap. §9 Q3's
   suffix shape shipped as proposed (`<name> (<tail6>)`).
