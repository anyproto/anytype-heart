# Fuzzing AnyBlock JSON — options, verified

Scope: `pkg/lib/anyblockjson` at v1 freeze. Read: SPEC.md §11 (round-trip), §12
(validation), §13 (API), `snapshotdiff/`, `roundtrip_test.go`,
`markdownblocks_test.go` (the one existing fuzz target), `cmd/anyblockroundtrip`,
`.github/workflows/test.yml`.

---

## 0. Verified library facts (checked 2026-08-29)

| Thing | Verified status | Verdict |
|---|---|---|
| Go native fuzzing (`go test -fuzz`) | Go 1.25.7 declared in go.mod, toolchain 1.26.5 locally. `f.Fuzz` args limited to `string, []byte, int*, uint*, float*, bool` (go.dev/doc/security/fuzz). Seeds from `f.Add` + `testdata/fuzz/<Name>/`; generated corpus in `$GOCACHE/fuzz`. Automatic minimization; failing input written to `testdata/fuzz/`. | **Use it. Zero deps.** |
| Go fuzz per-input hang detection | **Verified in local GOROOT source** `src/internal/fuzz/worker.go:492`: `time.AfterFunc(10*time.Second, func(){ panic("deadlocked!") })` per input. The `workerTimeoutDuration = 1s` constant is *worker shutdown*, not per-input — the go.dev doc's "1 second per execution" is wrong/misleading. | 10 s hard watchdog. Too coarse — add your own budget. |
| Go fuzz OOM detection | None. `GOMEMLIMIT` only makes GC work harder. | Must hand-write a memory oracle. |
| `pgregory.net/rapid` | v1.3.0, published **2026-03-30**; repo `flyingmutant/rapid` last code push **2026-04-30**; 870★; **MPL-2.0**. Has `rapid.MakeFuzz` bridging any rapid test into `testing.F`. Reflection `Make[T]`/`MakeCustom` with `MakeConfig{Types, Kinds, Fields}`. | **Maintained. Best PBT lib for Go.** License needs a `license_finder` decision (see §5). |
| `rapid` on interface fields | **Verified in `make.go`**: no `reflect.Interface` case; falls to default and **panics** `"unsupported type kind for Make: %v"`. `MakeConfig.Types[ifaceType]` is checked *before* the kind switch, so an interface key is a working escape hatch. | Loud failure, not silent. Good. |
| `AdaLogics/go-fuzz-headers` | Last **code push 2024-08-06** (≈2 y stale); no tagged release, pseudo-version only; 111★; Apache-2.0. | **Dormant.** |
| `go-fuzz-headers` `AdaptArbitrary` | **Does not exist.** Full exported index: `NewConsumer`, `GenerateStruct`, `GenerateWithCustom`, `AddFuncs`, `GetString/Int/Bytes/…`, `FuzzMap`, `TarBytes`, `CreateFiles`. The brief's premise is wrong. | Correct the assumption. |
| `go-fuzz-headers` on interface fields | **Verified in `consumer.go`**: `fuzzStruct`'s kind switch has **no `reflect.Interface` case**; an interface field falls through and is left **nil, with no error**. | **Disqualifying** for this format — see §1. |
| `google/gofuzz` | **ARCHIVED**; last push 2022-11-07. | **Rule out.** |
| `leanovate/gopter` | Last push 2026-04-20; 637★; **MIT**. | Maintained fallback if MPL blocks rapid. Weaker than rapid. |
| `thepudds/fzgen` | Last **code push 2024-07-23**; 116★; Apache-2.0; author's own README still says "work in progress… approaching beta quality". | **Dormant + self-declared pre-beta.** Skip. |
| `santhosh-tekuri/jsonschema/v6` | **Already a direct dependency** (v6.0.2), used by `index.go` and `authoring.go`. Draft 2020-12. | Schema oracle is free — no new dep. |
| OSS-Fuzz | FAQ: **"My project is not open source. Can I use OSS-Fuzz?" → no.** Acceptance: "significant user base and/or critical to global IT infrastructure", case-by-case, weighted on "exposure to remote attacks" and dependent-project count. | anytype-heart ships under **Any Source Available License 1.0** — source-available, *not* OSI open source. **Do not pursue.** |
| ClusterFuzzLite | `google/clusterfuzzlite`, last push 2026-02-12, not archived, Apache-2.0, 535★. GitHub Actions / GitLab / Cloud Build / Prow; Go supported. | The realistic "OSS-Fuzz-grade" option — but see §5 for why a cron `go test -fuzz` beats it here. |
| `AdamKorcz/go-118-fuzz-build` | Last push 2025-12-22, active, Apache-2.0, 31★. (The shim that makes `testing.F` targets run under libFuzzer, needed for OSS-Fuzz/CFL.) | Only relevant if you pick CFL. |
| `hypothesis-jsonschema` | Last push 2025-12-05, active, MPL-2.0, 280★. Generates instances from a JSON Schema (Python/Hypothesis). | The only credible off-the-shelf schema-driven generator. Cross-language. See Option 4. |

**Could not verify:** whether `go-fuzz-headers`' `AddFuncs`/`Funcs` map actually
dispatches on an *interface* `reflect.Type` key (the source excerpt I read did not
show the lookup site). Moot — see §1.

---

## 1. The gating problem: where do snapshots come from

This is the crux and deserves the most space. Everything in the Marshal direction
(oracles I1, byte-fixpoint, snapshot-equivalence) needs a supply of
`*model.SmartBlockSnapshotBase` values.

### Why the reflection generators fail here

Two fields carry all the information in the format, and **both are gogo-protobuf
oneofs, i.e. unexported Go interfaces**:

- `model.Block.Content` — `isBlock_Content`, **18 variants** (`BlockContentOfText`,
  `…OfFile`, `…OfDataview`, `…OfTable`, `…OfLink`, `…OfLatex`, `…OfWidget`, …).
- `types.Value.Kind` — `isValue_Kind`, 6 variants (null/number/string/bool/struct/list),
  and `SmartBlockSnapshotBase.Details` is a `*types.Struct` of them.

Consequences, verified against each library's source:

- **`go-fuzz-headers.GenerateStruct` leaves both nil, silently.** You would get
  a fuzzer that generates blocks with no content and details with no values,
  explore roughly nothing, and never be told. This is the worst failure mode
  available — a green fuzzer that isn't testing anything.
- **`rapid.MakeCustom` panics** `unsupported type kind for Make` — you find out
  immediately, and `MakeConfig.Types[<iface type>]` (obtainable via
  `reflect.TypeOf(model.Block{}).FieldByName("Content").Type`, since the
  interface itself is unexported) is a documented override point.

So: **whichever library you pick, you hand-write the oneof dispatch anyway.**
~18 block-content constructors + 6 value kinds + the dataview/table sub-shapes.
Realistically 300–500 lines of generator. The library buys you the *plumbing
around* that code, not the code itself. Judge the strategies accordingly.

### The four strategies, compared

**(a) Mutate the real protobuf corpus — recommended, and it needs zero generator code.**

`cmd/anyblockroundtrip` already writes every exported object as a
`pb.SnapshotWithType` protobuf binary (`main.go:537` reads `.pb` files with
`proto.Unmarshal`, `:699` writes `original.pb`). So the corpus already exists as
a by-product of the tool the team already runs.

The fuzz target is then, in full:

```go
func FuzzSnapshotRoundTrip(f *testing.F) {
    // seeds: every .pb from a sweep run, plus goldens re-serialized
    f.Fuzz(func(t *testing.T, data []byte) {
        if len(data) > 1<<16 { return }
        var sw pb.SnapshotWithType
        if proto.Unmarshal(data, &sw) != nil { return }   // the structure filter
        base := sw.Snapshot.GetData()
        if base == nil { return }
        ... Marshal → Validate (I1) → Unmarshal → Marshal → byte-compare
    })
}
```

Why this is the best defect-per-effort here:

- Protobuf binary is **unusually mutation-friendly**: field tags are varints, most
  single-byte flips land on a valid wire type, and length-prefixed submessages
  survive splices. A high fraction of mutants decode. Contrast random bytes into
  a JSON parser, where almost nothing survives.
- `proto.Unmarshal` is a *free, exact* structural filter. You get well-formed
  snapshots with hostile *contents* — which is exactly what you want, because
  the snapshot's contents are documented as untrusted (§11: "The snapshot's
  block graph is untrusted").
- It reaches shapes a hand generator would never think of: invalid UTF-8 in
  strings (gogo's generated `Unmarshal` does not validate UTF-8 — there is
  already an `invalidutf8_test.go`), out-of-range enums, cyclic `ChildrenIds`,
  duplicate block ids, oversized keys.
- **`BlockContentDataviewFilter.NestedFilters []*BlockContentDataviewFilter` is
  recursive in the proto** (models.pb.go:5018). Self-splicing a filter submessage
  doubles nesting depth per mutation — the deep-filter-tree defect is reachable
  *from this direction too*, geometrically fast.
- Coverage guidance does the corpus curation for you: the engine keeps mutants
  that reach new branches in `export.go`/`validate.go`.

Hazards, and the mitigations:

- That same proto recursion means `proto.Unmarshal` itself can recurse deeply on
  hostile input. Go grows stacks to 1 GB then takes an **unrecoverable** fatal
  error. Cap input length (`if len(data) > 1<<16 { return }`, the pattern
  `FuzzMarkdownImports` already uses) — at ~3 bytes per nesting level that caps
  depth around 20 k frames, which is fine.
- Mutants are not distributionally realistic. That is a feature for exact oracles
  and a **liability for `snapshotdiff.Compare`** — see §2, Option 1.

**(b) Hand-written `rapid` generators — the right *second* step, not the first.**

You write the 18-way content generator and the value-kind generator, compose
snapshots from them, and drive with `rapid.Check` / `rapid.MakeFuzz`. What this
buys over (a):

- **Shrinking of a *snapshot*, not of bytes.** When a mutated-proto target fails,
  Go minimizes the *byte string*; the minimized bytes still decode to a snapshot
  you have to read by hand. rapid shrinks the *structure* — "a document with one
  paragraph whose text is `" "`". For a format whose failures are structural,
  this is a real ergonomic difference.
- **Controlled distribution**, which is what makes the `snapshotdiff` oracle
  usable: you know what went in, so a finding is either a real loss or a
  normalization you need to admit.
- Reaches valid-but-rare combinations the corpus simply does not contain (a
  template with a `template_for`, a type document with all four recommended roles
  empty, an `iconOption` of exactly 1 with no other icon channel).

Cost: the 300–500 lines above, plus MPL-2.0 license paperwork (§5).

**(c) Derive snapshots from fuzzer bytes with a hand-written byte cursor.**
i.e. your own 60-line `type source struct{ b []byte }` with `next() byte`,
`nextString()`, feeding the same hand-written generator as (b). Middle ground:
coverage-guided like (a), structured like (b), no new dependency, no MPL question.
Worth knowing about, but it duplicates what (a) gets for free from
`proto.Unmarshal`. **Only build this if coverage shows (a) can't reach something.**

**(d) Reflection-based struct filling (`go-fuzz-headers`, `rapid.Make`, `gofuzz`).**
Rejected above. `gofuzz` is archived; `go-fuzz-headers` silently nils the oneofs;
`rapid.Make` panics without a hand-written override — at which point you are in (b).

### Verdict

> **Corpus mutation beats generation here, decisively, and by more than usual —
> because the corpus is already produced by an existing tool, and because the
> transport (protobuf binary) happens to be one of the most mutation-friendly
> formats there is.** Start at (a). Add (b) in month two, specifically to unlock
> the `snapshotdiff` oracle and structural shrinking. Never build (d).

---

## 2. The options

Ordered by what I would do first.

---

### Option 1 — Native `go test -fuzz`, corpus-seeded, with explicit invariant *and resource* oracles

**What it is.** Three (eventually five) `testing.F` targets in
`pkg/lib/anyblockjson`, no new dependencies. Two of them take document bytes, one
takes protobuf bytes (§1a). The novel part is not the fuzzer, it is that each
target asserts *typed invariants and a resource budget*, not "did it panic".

The resource oracle is the piece that matters most and that nobody ships for you:

```go
func budgeted(t *testing.T, in []byte, f func()) {
    var s [1]metrics.Sample
    s[0].Name = "/gc/heap/allocs:bytes"     // cheap, no stop-the-world
    metrics.Read(s[:]); before := s[0].Value.Uint64()
    start := time.Now()
    f()
    metrics.Read(s[:]); alloc := s[0].Value.Uint64() - before
    // amplification, not absolute: bytes allocated per input byte
    if alloc > 32<<20 && alloc > 2000*uint64(len(in)) {
        t.Fatalf("alloc amplification: %d bytes in, %d allocated", len(in), alloc)
    }
    if d := time.Since(start); d > 500*time.Millisecond {
        t.Fatalf("slow input: %d bytes in, %s", len(in), d)
    }
}
```

Use **allocated bytes as the primary budget and wall time as a loose secondary** —
allocation counts are near-deterministic and machine-independent; wall time on a
GitHub `macos-15` runner is not, and a tight time bound is the single biggest
flake source available. (Go's own 10 s watchdog stays as a backstop.)

**Directions and oracles.**

| Target | Direction | Oracles |
|---|---|---|
| `FuzzDocumentBytes` | bytes → Validate/Unmarshal | **I2** (`Validate(d)==nil` ⟺ `Unmarshal(d,bare)==nil`), crash, **resource**, and on success **I1** + **byte fixpoint** |
| `FuzzSnapshotRoundTrip` | snapshot → Marshal → … | **I1**, **byte fixpoint** (§11.3), crash, **resource** |
| `FuzzFragmentAgreement` | both | **cross-surface agreement** (oracle 3), I2 |
| later: `FuzzIndex`, `FuzzPropertyDictionary` | bytes | crash, resource, schema conformance |

I2 caveat, and it is real: **`Validate` takes no `Options`; `Unmarshal` does.**
Run I2 with `Options{GenerateId: seqIds("f")}` and nothing else wired. With
resolvers wired, §3's resolver-dependent rejections (ambiguous spelling with no
`ScopedKeyVocabulary`) make disagreement *legal* and the oracle becomes noise.

Schema conformance is a fourth oracle and costs one line, because
`santhosh-tekuri/jsonschema/v6` is already a dependency: after every successful
`Marshal`, run the output through the compiled `object.schema.json`. Note this is
*weaker* than I1 (Validate = schema **plus** ~15 semantic rules §12), so it earns
its place mainly on the `authoring/` subset, where the subset schema is a
separate artifact that can drift from `ValidateAuthoring`.

**Benchmark defects it would plausibly have caught.**

- 2500×2500 table, 15 KB → 635 MB / 34 s — **YES, high confidence.** Repeating a
  `{"id":"c1"}` entry inside a `columns` array is a bread-and-butter splice
  mutation from a table seed, and the alloc-amplification bound fires long before
  635 MB. The 10 s watchdog would also catch the 34 s version.
- Deep filter trees, 382 KB → 9.1 s / 3.7 GB — **YES.** Note 9.1 s is *under* Go's
  10 s watchdog, so crash-only fuzzing would have **missed** it; the alloc bound
  catches it at a fraction of the depth. Reachable from both directions (the
  proto `NestedFilters` recursion).
- `checkNumbers` pointer rebuild, 703 KB → 12.2 s / 4.9 GB — **YES.** Quadratic in
  node count; a byte fuzzer grows documents readily. Caught by the alloc bound
  early, by the watchdog late.
- 1 000 000-char property key / raw NUL, CR, ESC — **YES, via I1 from the snapshot
  direction, and this is the nicest hit in the list.** The schema bounds property
  names at `maxLength: 128` (mirrored at `validate.go:1625`, `maxPropertyKeyLen`).
  Mutate a detail key in the proto corpus to 200 chars → `Marshal` emits it →
  `Validate` rejects it → **I1 violated**, reported with the exact key. No
  expected output needed.
- Duplicate JSON keys, last-wins — **NO. Honestly, none of these options finds it.**
  `encoding/json` last-wins is deterministic and both `Validate` and `Unmarshal`
  do it identically, so I2 is silent; the document round-trips byte-stably. It
  needs a purpose-built duplicate-key scan over the token stream. Worth writing as
  a plain unit check; it is not a fuzzing problem.
- NFC/NFD equivalent keys — **MEDIUM, and only with the right seed.** Go's mutator
  is byte-level; it will not synthesize a valid NFD sequence by chance. Put a
  seed containing both forms of `é` in `testdata/fuzz/`, add the postcondition "no
  two keys in a `Marshal` output are NFC-equal but byte-distinct", and duplication
  mutations find it. General lesson: **a byte fuzzer finds what the seed corpus
  makes reachable.**
- Fragment surface dropping unknown members — **YES**, but by Option 3's oracle;
  see there.
- No decompression-ratio bound in the archive chain — **NO, and not in this
  package.** That code is `core/block/import/common/source/zip.go`. It wants its
  own ~30-line `FuzzZipSource` target with an output-size budget, which would
  find it in minutes. Separate ticket, high value, low effort.

**Integration effort.** ~250 lines total for the three targets plus the
`budgeted` helper. The hard parts, in order:

1. **Determinism.** Fuzz targets must be deterministic or the engine reports
   phantom crashers. `Options.GenerateId` must be the counter (`seqIds`, already
   in `roundtrip_test.go:54`). Guard against map-iteration leakage by running each
   input twice and comparing outputs for the first week — `json.go` is described
   as an ordered canonical writer, so this should hold, but verify rather than
   assume.
2. **Seed conversion.** `testdata/fuzz/<Name>/` files must be in Go's corpus text
   format, not raw JSON. Avoid the converter entirely: `//go:embed testdata/*.json`
   and `f.Add(...)` in the target, so the goldens seed themselves and stay in sync.
3. **Picking the amplification constants.** Measure the existing corpus first, set
   the bound ~10× above the observed p99, tighten later.

**CI story.** Two distinct things, and conflating them is the usual mistake:

- **Regression (every PR, free).** Files under `testdata/fuzz/<Name>/` run as
  ordinary unit tests under plain `go test`. The existing `test.yml` job picks
  them up with no change at all. Every crasher the engine finds becomes a
  permanent regression test the moment you commit it.
- **Discovery (nightly).** A new scheduled workflow: `go test -run=XXX
  -fuzz=FuzzDocumentBytes -fuzztime=20m ./pkg/lib/anyblockjson/`, one step per
  target, `actions/cache` on `$GOCACHE/fuzz` keyed per target so the generated
  corpus carries over between nights. On failure the engine writes the minimized
  input to `testdata/fuzz/` — upload it as an artifact and open an issue; a human
  commits it. `nightly.yml` already exists as the cron pattern to copy.
  Note `test.yml` runs on `macos-15` runners; fuzzing is CPU-bound and Linux
  runners are cheaper and faster — use `ubuntu-latest` for the fuzz job.
- **Flake risk: low**, if you use allocation budgets rather than wall-clock, and
  keep `snapshotdiff` off the fuzz path (see below).

**Ongoing cost.** Low. No dependency to track. Decay mode is the usual one:
targets rot when the API changes, and nobody notices the nightly job going red.
Mitigate by making the nightly failure open a Linear issue rather than an email.

---

### Option 2 — `pgregory.net/rapid` generators for the snapshot direction

**What it is.** v1.3.0 (2026-03-30), MPL-2.0, actively maintained, 870★, no
dependencies outside stdlib. Hand-written generators over the block/property model
(§1b), driven either by `rapid.Check` in a normal test or by `rapid.MakeFuzz`
inside a `testing.F` so the same property runs under coverage guidance.

**Directions and oracles.** Snapshot direction only, but it is the *only* option
that can safely drive the strongest oracle:

- **I1**, **byte fixpoint** — same as Option 1, redundantly.
- **Snapshot equivalence (`snapshotdiff.Compare`)** — this is the one that needs a
  controlled distribution, and it is the reason this option exists.

**Why `snapshotdiff` must not run on mutated input.** `Compare` is calibrated
against real accounts and consults the format's own exported predicates
(`DroppedMissingObjectRef` at `refs.go:182`, `DroppedDeletedIconRef` at
`refs.go:119`, `DroppedTypeProvenanceKey` at `typesettings.go:151`,
`OmittedBundledRelation`/`RelationInstallArtifactKey`/`InstallStampedDefault` in
`omittedrelation.go`). The package's own comments record that an *unadmitted*
normalization once produced **1 344 false failures in a single sweep**. Feed it
adversarial mutants and you will manufacture that situation on purpose, every
night. So:

> **Exact oracles (I1, I2, byte fixpoint, resource, crash) on mutated input.
> Judgement-laden oracles (`snapshotdiff.Compare`) on controlled input only.**

That line is the single most important design decision in this whole report.

**Benchmark defects.** Adds little over Option 1 on the listed eight — the
resource defects are byte-side, and rapid biases toward *small* values, so it
would need an explicit "occasionally 5 000 columns" generator to find the table
blow-up. Its real value is the class of defect **not** on the list: silent field
loss that is byte-stable in both directions and therefore invisible to the
fixpoint oracle. That is precisely what the steer is pointing at, and only
`snapshotdiff` sees it.

**Integration effort.** The big one: 300–500 lines of generator (18 block-content
variants, 6 value kinds, dataview views/filters/sorts, table column/row/cell,
envelope kinds incl. `object_type` and `property`). Hard parts:

1. The oneof override. `MakeConfig.Types` keyed on
   `reflect.TypeOf(model.Block{}).FieldByName("Content").Type` — the interface is
   unexported, so reflection is the only way to name it. Verified as a supported
   path (`Types` is consulted before the kind switch), but I have not run it.
2. Keeping the generator's *legality* aligned with the spec as the spec moves —
   this is the decay mode, and it is worse than Option 1's.
3. Deciding which `Compare` findings are new normalizations vs. real loss. This is
   human work per finding, permanently.

**CI story.** Runs as a plain unit test (`rapid.Check`) on every PR in a few
seconds, and as a `-fuzz` target nightly via `rapid.MakeFuzz`. rapid does its own
shrinking and prints a reproducible seed. Flake risk: **moderate** — every new
legal normalization added to the format shows up as a red build until admitted.
That is arguably correct behaviour, but budget for it.

**Ongoing cost.** Highest of the options. One owner, and the generator must be
updated in the same commit as any format change — the same "same-commit
discipline" the package already applies to the `Dropped*` predicates.

**License note.** MPL-2.0 test-only dependency. `test.yml` runs `license_finder`
against `anyproto/open`'s `decisions.yml` — **this will need a decision entry
before the build goes green.** If that is a fight you don't want, `gopter` (MIT,
maintained, last push 2026-04-20) is the fallback, or drive the same hand-written
generator from a byte cursor (§1c) and add no dependency at all. rapid wins on
merit — generics, better shrinking, the `MakeFuzz` bridge — but the margin over a
hand-rolled byte cursor is smaller here than usual, precisely because you are
hand-writing the generators either way.

---

### Option 3 — Cross-surface differential / metamorphic targets

**What it is.** Not a tool — a family of ~40-line assertions where the *expected
output is another code path in the same package*. No dependency, no generator, no
oracle to design.

The relations available:

1. **I2**: `Validate(d) == nil` ⟺ `Unmarshal(d, bare) == nil`. Spec §12 states it
   flatly: *"Validate and Unmarshal agree, in both directions."*
2. **Fragment vs. whole document**: the same block run through
   `UnmarshalBlocks(run, opts)` vs. embedded in a `{"version":2,"blocks":[…]}`
   envelope and run through `Unmarshal`; the same filter tree through
   `UnmarshalFilters` vs. inside a dataview block; the same subtree through
   `MarshalBlockSubtree` vs. cut out of a full `Marshal`.
3. **Round-trip through the fragment surface**: `MarshalBlockSubtree ∘
   UnmarshalBlocks` fixpoint.
4. **Authoring subset**: `ValidateAuthoring(d) == nil` ⟹ `Validate(d) == nil`
   (the subset is documented as *strict*, so one-way implication is the relation).
5. **`filterstring` vs. the JSON filter tree** — there is already a
   `filterstring_agreement_test.go`, so the pattern is established in the package.

**Benchmark defects.** Catches the fragment-drop defect —
`UnmarshalFilters`/`UnmarshalSorts` silently dropping unknown members while the
whole-document path refused them — **directly, and it is the only option that
does.** That defect has already occurred once, which makes this the only oracle in
the report with a *demonstrated* hit rate on this codebase. It catches nothing
else on the list.

**Integration effort.** Lowest by a wide margin. ~40 lines per relation. The hard
part is stating each relation precisely enough that legal differences don't
register — e.g. the fragment surface mints ids where the document path may not, so
compare after id-normalization.

**CI story.** These are ordinary table tests *and* fuzz targets — write them as
`testing.F` so they get both. Zero flake risk once the relations are stated
correctly. No corpus management.

**Ongoing cost.** Near zero. Each new fragment entry point added to
`fragment.go`/`filters.go` should come with its agreement relation; that is a
review-checklist item, not a maintenance burden.

**Verdict on the framing question "does metamorphic/differential deserve its own
option?" — yes, emphatically.** It is the highest defect-per-line item here, it
needs no generator, and the format's own spec hands you the relations. It is not
a substitute for Option 1 (it finds no resource defects) but it should ship in
the same week.

---

### Option 4 — Schema-driven generation from the published JSON Schemas

**What it is.** Use `schema/object.schema.json`, `index.schema.json`,
`properties.schema.json` and `schema/authoring/*` as a *grammar*: generate
conforming instances, then mutate them in targeted ways (drop a required member,
violate an `enum`, exceed a `maxLength`, swap a discriminator `type`).

Tooling reality:

- **Nothing off-the-shelf does this in Go.** I found no maintained Go
  schema→instance generator. `santhosh-tekuri/jsonschema/v6` validates; it does
  not generate.
- The credible off-the-shelf generator is **`hypothesis-jsonschema`** (Python,
  MPL-2.0, active as of 2025-12), driven against a small Go CLI harness that reads
  a document on stdin and prints the verdicts of `Validate`/`Unmarshal`/`Marshal`.
  That is the only genuinely *language-agnostic* option in this report.
- A hand-written Go schema walker is ~200 lines for this schema and gives you
  in-process speed and no Python in CI.

**A property of *this* schema that matters.** §12 states the block definition is
**deliberately non-recursive** (no `children`; table cells use a separate
`cellBlock` definition to cut the block↔cell cycle), and the *only* recursive
definition left is the dataview filter tree. Non-recursive schemas are exactly the
ones generators terminate on cleanly. So schema-driven generation is more
tractable here than for a typical document format — and the one recursion it has
is the very place the deep-nesting defect lives.

**Directions and oracles.**

- Conforming instances → **I1's contrapositive is not available** (schema-valid ≠
  `Validate`-valid, since `Validate` = schema + ~15 semantic rules), so you cannot
  assert "generated ⟹ accepted". What you *can* assert: schema-invalid ⟹
  `Validate` rejects (the schema is a subset of the rules), and every accepted
  document must round-trip.
- Deliberately non-conforming instances → **the error-quality oracle**: exactly one
  fault should produce exactly one issue, at the right JSON pointer. §12 makes a
  point of this ("`oneOf` reported 10 issues for one wrong member and never named
  the alternatives; `if`/`then` reports one and does"). A generator that injects
  *one* schema violation at a known pointer and asserts the reported pointer
  matches is a genuinely good, and genuinely unusual, test — and directly serves
  the LLM-producer consumer the spec calls out.
- **Coverage seeding.** The best pragmatic use: generate a few thousand conforming
  instances once, keep the ones that add coverage, commit those as
  `testdata/fuzz/` seeds for Option 1. Schema-driven generation as a *corpus
  bootstrapper*, not as the running engine.

**Benchmark defects.** Weak on the list. It reaches the table and filter shapes
only if you explicitly tell the generator to emit large arrays and deep nesting —
the schema has 21 `maxItems`/`maxLength` occurrences and (per the brief) no bound
on grid size, so an unguided generator emits *small* instances. It finds none of
the resource defects on its own; it would not find the fragment disagreement (it
generates whole documents); it would not find the duplicate-key or NFC issues.

**Integration effort.** Medium-high, and cross-language if you take
`hypothesis-jsonschema`: a Go CLI harness (~100 lines), a Python driver, a Python
toolchain in CI. The Go walker avoids that at the cost of writing and maintaining
a generator for draft 2020-12 features you actually use (`if`/`then`, `$ref`,
`propertyNames`, `const` discriminators).

**CI story.** Awkward. Property-based Python in a Go repo's CI is a maintenance
liability, and the schemas move as the format moves. Best run as a one-off
corpus-bootstrapping exercise, and as a permanent *small* Go-side test for the
error-quality oracle.

**Ongoing cost.** Medium, and it decays badly: the generator must track schema
edits, and nobody will remember it exists.

**Verdict: do the cheap 20 % of it.** The error-quality single-violation test in
Go, yes. Full schema-driven generation as the main engine, no.

---

### Option 5 — Continuous fuzzing infrastructure (OSS-Fuzz / ClusterFuzzLite)

**OSS-Fuzz: not realistic. Two independent blockers, both verified.**

1. The FAQ answers *"My project is not open source. Can I use OSS-Fuzz?"* with a
   flat no. `anytype-heart` ships under **Any Source Available License 1.0** —
   source-available, not OSI open source. This alone likely ends it.
2. Acceptance requires *"a significant user base and/or be critical to the global
   IT infrastructure"*, weighted on remote-attack exposure and dependent-project
   count. A document codec inside one desktop app is a hard sell even setting
   licensing aside.

Integration cost, had it applied: a `projects/anytype-heart/` directory upstream
(Dockerfile, `build.sh`, `project.yaml`), a Google-account committer contact, and
`go-118-fuzz-build` to compile `testing.F` targets under libFuzzer. Non-trivial,
and it puts your build in someone else's repo.

**ClusterFuzzLite: technically viable, probably not worth it.** Active
(2026-02-12), Apache-2.0, GitHub Actions supported, Go supported. It gives you
PR-scoped fuzzing, crash deduplication, coverage reports and corpus persistence.
But it requires the same OSS-Fuzz-style Docker build, and this repo's build is
heavy (protoc, tantivy, CGO — see `test.yml`). You would spend a week on the
container to gain deduplication and a coverage dashboard.

**What to do instead: a cron `go test -fuzz` job.** ~30 lines of YAML, no
container, corpus persisted with `actions/cache`, minimization and regression
capture already built into the Go toolchain. That is 95 % of the value for 5 % of
the effort, and it is the right answer until the nightly job is actually finding
things and someone wants a dashboard.

---

## 3. The three framing questions, answered

**Go-specific or JSON-generic?**
**Go-specific, decisively.** Three reasons particular to this case: (i) the two
strongest oracles are *snapshot*-level (I1 and snapshot-equivalence) and a
`*model.SmartBlockSnapshotBase` cannot cross a process boundary without you
writing a serializer for it — you would be building a CLI harness to lose
fidelity; (ii) `snapshotdiff.Compare` is a Go API with `Options` and resolver
capabilities as parameters, and reimplementing its ~1 300 lines of admitted
normalizations out-of-process is absurd; (iii) the resource oracle needs
in-process allocation counters, which no external mutator can give you. A JSON-
generic mutator would be confined to the hostile-bytes half — the half that is
already well served by `go test -fuzz` with a good seed corpus. The single
exception where language-agnostic tooling earns its keep is schema-driven
*instance generation* (Option 4), because no Go tool does it — and even there the
output is just seed files for the Go fuzzer.

**Attack via the JSON Schema, or by other terms?**
Three sub-strategies, and they pay off very differently here:

- *Coverage-guided byte mutation* (Option 1) — **the main engine.** Wins because
  the seeds are real and plentiful and the code paths are deep. Finds every
  resource defect on the benchmark list.
- *Grammar/structure-aware mutation* — **the surprise winner, in an unusual form.**
  The right grammar for the Marshal direction is not the JSON Schema, it is
  **protobuf wire format**, mutated by the generic byte fuzzer with
  `proto.Unmarshal` as the structural filter. You get structure-aware mutation
  without writing a grammar.
- *Schema-driven generation* (Option 4) — **best as a corpus bootstrapper and as
  the driver of the error-quality oracle**, not as the running engine. Its
  fundamental limit is that `Validate` is schema **plus** semantics, so
  schema-conformance is neither necessary nor sufficient for acceptance, and the
  strong oracles do not attach to it.

**What genuinely automates this, versus what must be hand-built?**

*Automated, off the shelf:* the mutation engine, corpus management, crash
minimization, regression capture, the 10 s hang watchdog — all in the Go
toolchain, today, for free. Structural generation of snapshots — free, via
`proto.Unmarshal` over the existing `.pb` corpus. Schema validation — free,
`santhosh-tekuri/jsonschema/v6` is already a dependency.

*Must be hand-built, and nobody sells it:* every oracle. The alloc/time
amplification budget (~20 lines, and it is the highest-value 20 lines in this
report). The I1/I2/fixpoint assertions (~15 lines each, trivial once stated). The
cross-surface agreement relations (~40 lines each). The `snapshotdiff` gating
policy. The seed corpus curation. And, if you go to Option 2, the 300–500-line
oneof-aware snapshot generator — which **no library will write for you**, because
gogo oneofs defeat every reflection-based filler in the Go ecosystem.

The honest summary: **the tooling is ~10 % of this job and the oracles are ~90 %,
and this format is unusual in that its oracles are already written down.**

---

## 4. Recommendation

**Do first (week 1):** Option 1 (native fuzzing, corpus-seeded, with the
allocation-amplification oracle) + Option 3 (cross-surface agreement). Together
they are ~300 lines, add no dependency, need no license decision, plug into the
existing `test.yml` for free as regression tests, and would have caught **four of
the eight** benchmark defects — including all three resource-exhaustion ones and
the fragment disagreement.

**Add later (month 2, if week 1 pays off):** Option 2 (`rapid` + hand-written
snapshot generators), specifically and only to unlock `snapshotdiff.Compare` on a
controlled distribution and to get structural shrinking. Gate it behind the
license decision for MPL-2.0; if that stalls, drive the same generators from a
hand-rolled byte cursor and skip the dependency.

**Do cheaply and separately:** a `FuzzZipSource` target in
`core/block/import/common/source/` with an output-size budget. Thirty lines, and it
finds the decompression-ratio defect — which nothing in `anyblockjson` can reach.

**Skip:** OSS-Fuzz (license + acceptance criteria, verified). `go-fuzz-headers`
(dormant, and silently nils the oneofs — the worst possible failure mode).
`google/gofuzz` (archived). `fzgen` (dormant, self-declared pre-beta). Full
schema-driven generation as an engine (keep only the single-violation
error-quality test). ClusterFuzzLite, for now.

**Accept as not-fuzzable:** duplicate JSON keys last-wins. Write it as a unit
check over the token stream and move on.

---

## 5. Concrete first-week sketch

**Target 1 — `FuzzDocumentBytes`** *(hostile input; oracles: I2, I1, fixpoint,
resource, crash)*

```
data → Validate(data)                            → vErr
     → Unmarshal(data, Options{GenerateId: seq}) → uErr
assert (vErr == nil) == (uErr == nil)                          -- I2
if uErr == nil:
    out := Marshal(sbType, snap, bare)
    assert Validate(out) == nil                                -- I1
    snap2 := Unmarshal(out, …); out2 := Marshal(…)
    assert out == out2                                         -- §11.2 fixpoint
all of it wrapped in budgeted()                                -- resource
```

Seeds: `//go:embed testdata/*.json` (`rich.json`, `rich_compact_ids.json`,
`rich_compact_omit.json`, `rich_omit_ids.json`, `containers.json`,
`testdata/authoring/*`) — plus a script-extracted set of the JSON literals already
embedded in the package's `*_test.go` files (there are hundreds, covering every
block type, mark case and envelope variant), plus one NFC/NFD seed and one
astral-plane seed. If a sweep has been run, add `-dump-json` output.

**Target 2 — `FuzzSnapshotRoundTrip`** *(round-trip; oracles: I1, byte fixpoint,
resource, crash)*

```
if len(data) > 1<<16 { return }
proto.Unmarshal(data, &pb.SnapshotWithType)      -- structural filter; skip on err
Marshal → assert Validate(json1) == nil                        -- I1
Unmarshal(json1) → Marshal → assert json1 == json2             -- §11.3
budgeted() around the whole thing                              -- resource
```

Seeds: the `.pb` files `cmd/anyblockroundtrip` already writes (`original.pb` per
artifact directory, and every `.pb` under a `-keep-exports` run). If no sweep
output is at hand, `proto.Marshal` the snapshots built by `richSnapshot()` and the
`snapshotdiff/` fixtures — a dozen seeds is enough to start; the engine grows the
corpus.

Deliberately **not** in this target: `snapshotdiff.Compare`. It goes in a
companion non-fuzz test, `TestCorpusSnapshotEquivalence`, that walks the committed
`testdata/fuzz/FuzzSnapshotRoundTrip/` corpus and any real `.pb` corpus available,
asserting §11.1 (`Import(Export(S)) ≡ N(S)`) on *unmutated* inputs only. Same
comparator, controlled distribution, no manufactured false failures.

**Target 3 — `FuzzFragmentAgreement`** *(cross-surface; oracles: agreement, I2)*

```
data → treat as a single block object
  A: UnmarshalBlock(data, "", opts)
  B: Unmarshal(`{"version":2,"blocks":[` + data + `]}`, opts)
assert (A errored) == (B errored)
assert blocks equal after id normalization
```
plus the same shape for `UnmarshalFilters`/`UnmarshalSorts` against a dataview
block, and the `MarshalBlockSubtree ∘ UnmarshalBlocks` fixpoint.

Seeds: single-block and single-filter literals lifted from `fragment_test.go`,
`filters_test.go`, `datefilter_test.go`.

**Also week 1, non-fuzz:** measure the alloc-per-input-byte distribution over the
existing corpus so the amplification constants are evidence-based rather than
guessed; and add the double-run determinism check to all three targets, to be
removed once it has been green for a week.

**CI wiring:** nothing for regressions (the existing job picks up
`testdata/fuzz/`). One new scheduled workflow on `ubuntu-latest`, three steps of
`-fuzztime=20m`, `actions/cache` on `$GOCACHE/fuzz`, artifact upload on failure.
