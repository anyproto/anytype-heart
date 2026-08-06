# ImportV2 — Reusing the User's Existing Types (opt-in)

Status: design, not implemented. Depends on the always-mint work
(`2026-08-05-importv2-planner-always-mint-design.md`), which this relaxes rather than replaces.

## 1. Why

Always-mint is the right default for an empty or near-empty space: the import brings its own
types, shaped by the source, and touches nothing the user built. It is the wrong default for
an *established* space. A user who already runs a Tasks type with their own properties, and
who imports a Notion task tracker, currently gets a second, parallel task type beside the
one they use. Nothing is broken; the space is now saying the same thing twice.

The ask is to let the user point the importer at their existing type system and have the model
map onto it. Off by default — with it off, behaviour is exactly the always-mint design.

## 2. Goals

- The user can opt in to mapping imported containers onto types that already exist.
- The model is *steered*, never trusted: a target it names must exist and be offered.
- With the flag off, byte-identical to today.
- A container mapped onto a pre-existing type keeps its collection (§5).

**Non-goals.** Reusing existing *properties* beyond the bundled allowlist (a follow-up — the
same evidence and validation machinery, but a much larger candidate set). Merging imported
objects into existing ones. Changing the always-mint default.

## 3. Wire

`AIParams` gains one field:

```proto
message AIParams {
    Rpc.AI.ProviderConfig config = 1;
    bool includeContentSamples = 2;
    bool reuseExistingTypes = 3;   // default false = always mint
}
```

Default-false keeps every existing client on today's behaviour with no change.

## 4. Candidate selection

The space's types cannot all go in the prompt: a mature space has dozens to hundreds, each
with properties, and the plan call already consumes 76-80s of a 90s budget with truncation a
live risk (always-mint design §6). Selection is therefore part of the design, not an
implementation detail.

Offer a type as a candidate when **all** hold:

- it is not bundled (bundled types are excluded by §3.3 of the always-mint design and that
  does not change);
- it is not archived or deleted;
- it has at least one object, or was authored by the user — a type that exists but is unused
  and import-created is noise;
- the candidate set is capped (proposed: 40, most-used first by object count), and the cap is
  reported in the run log when it truncates, per the no-silent-caps rule.

Each candidate is rendered as its key, name, and its recommended properties with formats —
the same `objectType` vocabulary the container evidence already uses, so the model sees
candidates and sources in one shape.

**Origin matters for the offer.** A type a previous import created is a candidate on the same
footing as a user-authored one; the distinction that matters is §5's, about what happens to
the collection.

## 5. Application rules

A container may name a candidate type instead of defining one. Then:

- **The container keeps its collection.** The single-database replacement (always-mint §3.4)
  applies only to types the plan *mints*. A pre-existing type already has objects, so "all
  objects of this type" is not "this database's rows" — the premise of the replacement fails,
  and the collection is the only thing that records which rows came from this database. This
  also resolves, by construction, the review finding where a minted type silently dedup-matched
  a user's same-named type and stranded the database's membership.
- **The type is never rewritten.** Not its name, layout, icon, or recommended relations.
  `persist.go:247-256` already refuses to rewrite a type the user authored; this design extends
  that intent to *every* reused type, including import-created ones, because the user may have
  edited it since. The import may only add objects of that type.
- **Property remaps still scope by type** (always-mint §3.2). A container on candidate type `T`
  scopes its custom property keys to `T`, so its properties join that type's rather than
  forming a parallel set — which is the entire point of opting in.

## 6. Trust boundary

`Sanitize` stays a pure function. Candidates are passed **in** as data — a set of allowed
type keys, exactly as `AllowedBundledTargets` is passed for relations — so the sanitizer
gains no dependency on the space and stays unit-testable:

```go
Sanitize(plan Plan, schemas []ContainerSchema, allowedTypes map[domain.TypeKey]TypeCandidate, report func(Issue)) Plan
```

A container naming a type that is neither plan-defined nor in `allowedTypes` is dropped with
`llmPlanEntryDropped`, as an unknown type is today. This closes the obvious attack: the model
cannot name an arbitrary existing type — only one the selection step chose to offer.

`scopeMap` treats a candidate key as a real type for scoping (it is one), so the §3.2
"validated scope" rule extends to candidates without change.

## 7. The risk this adds, stated plainly

`persist` protects the type *document*, but nothing protects the user's type from acquiring
objects that do not belong to it. If the model maps a Notion "Reading list" onto the user's
"Task" type, every row becomes a Task in their space, and undoing it means retyping each
object by hand. This is the one direction with no guard, and it is inherent to the feature —
which is why it is opt-in, why candidates are capped and curated, and why the import report
must list every container→existing-type mapping prominently rather than as an Info line among
hundreds.

It also reintroduces a judgment call the always-mint design deliberately removed for
stability (§1: 13 vs 4 containers typed on identical evidence at temperature 0). Expect the
mapping to vary between runs; combined with §5 the blast radius is bounded — a wrong mapping
mistypes objects but never rewrites a type or destroys a collection.

## 8. Testing

- **Archetype corpus** (always-mint §5) gains candidate-bearing cases: an archetype whose
  container clearly matches an offered type, one that clearly does not, and one where two
  offered types are plausible. Invariants: an obvious match is taken; a poor match mints
  instead; an unoffered key is never emitted.
- **Sanitize** unit tests: unoffered type key dropped; candidate key accepted; candidate used
  as a property scope; a candidate that is also a bundled key rejected.
- **Notion** scripted tests: a container on a candidate type keeps its collection; the
  candidate type object is not re-emitted or rewritten; rows carry the candidate's key.
- **Flag off**: the whole existing suite must pass unchanged, which is the parity guarantee.

## 9. Open questions

- Should candidates include types from *other* spaces the user has? Assumed no.
- Should a reused type's recommended relations be *extended* with the database's properties
  (the always-mint §3.4 backfill)? That edits the user's type, so the default is no — but then
  imported properties on a reused type are unlisted, the same gap §3.4 fixed for minted types.
  Needs a decision; the honest options are "leave unlisted" and "ask the user".
- Is `reuseExistingTypes` the right granularity, or should it be a three-way (never / suggest /
  always)? Proposed: start binary, widen only if asked for.
