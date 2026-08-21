# Extending the bundled relations list from JSON files at runtime — Design

**Issue:** none — internal escape hatch, commit as `GO-0000`
**Branch:** `go-0000-extra-bundled-relations` (proposed)
**Date:** 2026-08-17

## Goal

Let an internal build add relations to `pkg/lib/bundle`'s bundled relation set at process
start, by pointing an environment variable at one or more JSON files that use the exact
same schema as `pkg/lib/bundle/relations.json`.

The extras are **additive only**: a key that already exists in the generated bundle is never
replaced.

This exists so an internal deployment or a debugging session can teach the middleware about a
relation without regenerating `relation.gen.go` and shipping a new binary. It is deliberately
undocumented — no user-facing docs, no mention in the API surface.

## Non-goals

- **Installing the extras as objects in spaces.** `RelationChecksum` stays a generated
  compile-time constant, so `core/indexer/reindex.go:117` never observes a change and no
  bundled relation objects are created or reindexed for the extras. They live in the
  in-memory bundle only.
- **Extending bundled types, layouts, or the system/internal relation lists.** Only the
  `relations` map is extendable. `TypeChecksum`, `SystemRelations`, `RequiredInternalRelations`,
  and `Layouts` are untouched.
- **Generator changes.** `pkg/lib/bundle/generator` is not modified. The extras never enter
  `relations.json` and never affect the checksum.
- **A removal or override mechanism.** There is no way to delete or shadow a generated relation.

## What "in-memory only" buys you

`relations` is a single unexported package map (`pkg/lib/bundle/relation.gen.go:215`,
`map[domain.RelationKey]*model.Relation`) and every accessor in the package reads it:
`GetRelation`, `PickRelation`, `MustGetRelation`, `MustGetRelationLink`, `GetRelationFormat`,
`HasRelation`, `ListRelationsUrls`. Merging into that one map is therefore enough for every
consumer in the repo to see the extras, including:

- format resolution in `core/relationutils/formatfetcher`
- the "is this key bundled?" branch in the import pipeline, `objectcreator`, `smartblock`,
  `participant`, and `core/api/service/property.go`
- the `_br…` bundled-relation source (`core/block/source/sourceimpl/bundledrelation.go`,
  which lists ids via `bundle.ListRelationsUrls`)

Two consequences follow from staying in memory, and both are accepted:

1. **No generated Go constant.** An extra relation is reachable only by string key. Compiled
   code cannot refer to `RelationKeyFoo`, so extras are usable from data-driven paths (API,
   import, queries, details) and nowhere else.
2. **Devices disagree.** A device without the env var answers `HasRelation` differently for the
   same key, and will treat a relation object carrying that key as an ordinary space relation
   rather than a bundled one. Acceptable for an internal switch; it is the main reason this is
   not a shipped feature.

## Behavior contract

| `ANYTYPE_EXTRA_RELATIONS` | Behavior |
|---|---|
| unset or empty | Zero work, zero behavior change. No file I/O, no allocation beyond an empty split. |
| set, all entries valid | Each relation is added to `relations` before any derived list is computed. |
| set, key collides with a generated relation | Generated relation wins, extra is skipped, one warning logged per collision. |
| set, anything else invalid | `panic` during package init with a message naming the env var, the file, and the offending entry. |

Fail-fast is deliberate: a typo in an internal-only env var should stop the process loudly
rather than produce a half-populated bundle whose symptoms surface hours later as a missing
relation format.

**Collision is the one exception to fail-fast.** It logs and skips instead of erroring, because a
future release that adds a relation with a key an existing deployment already configured would
otherwise turn into a startup crash on that deployment. Skipping keeps the generated bundle
authoritative and the process alive; the warning is how the operator learns the extra is now
redundant.

## JSON schema

Identical to `pkg/lib/bundle/relations.json` — a top-level array of objects. This is a hard
requirement, not a convenience: it means an entry can be copy-pasted into `relations.json` and
regenerated when it graduates from an experiment to a real bundled relation.

```json
[
  {
    "key": "internalTraceId",
    "name": "Internal trace ID",
    "format": "shorttext",
    "source": "details",
    "description": "Correlates an object with an internal trace",
    "maxCount": 1,
    "hidden": true,
    "readonly": true
  }
]
```

Fields, with the same meaning and defaults the generator gives them:

| Field | Type | Required | Notes |
|---|---|---|---|
| `key` | string | yes | Must match `^[a-zA-Z_][a-zA-Z0-9_]*$` (the leading underscore permits `_score`-style keys). |
| `name` | string | yes | Non-empty. |
| `format` | string | yes | A key of `model.RelationFormat_value` (`longtext`, `shorttext`, `number`, `status`, `tag`, `date`, `file`, `checkbox`, `url`, `email`, `phone`, `emoji`, `object`, `relations`, `map`). |
| `source` | string | yes | A key of `model.RelationDataSource_value` (`details`, `derived`, `account`, `local`). |
| `description` | string | no | Defaults to empty. |
| `maxCount` | int | no | Defaults to 0 (unlimited). Must fit in an `int32` and be non-negative. |
| `hidden` | bool | no | Defaults to false. |
| `readonly` | bool | no | Maps to `ReadOnly`. |
| `objectTypes` | []string | no | Bare type keys; each is prefixed with `bundle.TypePrefix` (`_ot`), exactly as the generator does. An entry that already carries the `_ot` prefix is rejected rather than double-prefixed, as is an empty entry. |
| `revision` | int | no | Carried through. |
| `includeTime` | bool | no | Carried through. |

Two fields are fixed by the loader and not configurable, matching the generator: `Id` is
`addr.BundledRelationURLPrefix + key` (`_br…`) and `Scope` is `model.Relation_type`.
`ReadOnlyRelation` is always `true` — a bundled relation's own definition is never editable.

## Architecture

### New file: `pkg/lib/bundle/extra.go`

A pure loader plus a thin env-var wrapper. The loader takes its destination map as an argument
so it can be tested without touching the environment or package state.

```go
package bundle

const envExtraRelations = "ANYTYPE_EXTRA_RELATIONS"

var log = logging.Logger("bundle")

var relationKeyRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// extraRelationsPaths splits the env var. Extracted from init so it can be tested directly.
func extraRelationsPaths() []string {
	return filepath.SplitList(os.Getenv(envExtraRelations))
}

// relationJSON mirrors the schema of relations.json consumed by ./generator, so an entry can
// move between the two files unchanged.
type relationJSON struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Format      string   `json:"format"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	MaxCount    int      `json:"maxCount"`
	Hidden      bool     `json:"hidden"`
	Readonly    bool     `json:"readonly"`
	ObjectTypes []string `json:"objectTypes"`
	Revision    int      `json:"revision"`
	IncludeTime bool     `json:"includeTime"` // nolint: tagliatelle
}

// loadExtraRelations merges relations declared in the given JSON files into dst. Keys already
// present in dst are skipped: the generated bundle always wins. Any other problem is an error —
// the caller is expected to treat a misconfigured env var as fatal.
func loadExtraRelations(paths []string, dst map[domain.RelationKey]*model.Relation) error

// toModel validates one entry and converts it the way the generator does.
func (r relationJSON) toModel() (*model.Relation, error)
```

`loadExtraRelations` walks the paths in order and keeps a `seen map[domain.RelationKey]string`
of key → declaring file, so the same key declared twice across the extra files is reported as an
error naming both files rather than resolved by file order.

Error messages wrap with context per the repo convention, e.g.
`fmt.Errorf("parse extra relations file %s: %w", path, err)` and
`fmt.Errorf("extra relation %q in %s: %w", key, path, err)`.

`logging.Logger` is safe to use here: `pkg/lib/logging` imports only `pkg/lib/initialparams`,
which imports only `pb` — no cycle back into `bundle`.

### Call site: `pkg/lib/bundle/init.go`

The merge happens as the first statement of the package's existing `init()` (`init.go:71`):

```go
func init() {
	if paths := extraRelationsPaths(); len(paths) > 0 {
		if err := loadExtraRelations(paths, relations); err != nil {
			panic(fmt.Errorf("%s: %w", envExtraRelations, err))
		}
	}
	for _, r := range relations {
		// existing LocalRelationsKeys / DerivedRelationsKeys classification
	}
	// ...
}
```

Ordering is guaranteed, and this is why the merge goes *here* rather than in a new `init()` in
`extra.go`:

- Go initializes all package-level variables — including the `relations` map literal in
  `relation.gen.go` — before running any `init()` function, so the map is fully populated when
  the merge runs.
- Package `bundle` has **exactly one** `init()` (verified: `init.go:71` is the only one), so
  there is no file-ordering question to reason about. A second `init()` in a new file would
  order by filename (`extra.go` < `init.go`), which happens to work today but would silently
  break if either file were renamed.
- No package-level var initializer in `bundle` reads `relations` — only that `init()` does — so
  merging at its top is enough for `LocalRelationsKeys`, `DerivedRelationsKeys`, and
  `LocalAndDerivedRelationKeys` to classify extras by their `source` like any other relation.

`filepath.SplitList` is the separator choice: `:` on unix, `;` on Windows, which is the only
split that survives Windows drive letters. `filepath.SplitList("")` returns an empty slice, so
the unset case needs no special handling beyond the length check.

### Panic vs. returned error

`init()` can only fail by panicking. That is acceptable here because the panic is unreachable
unless the env var is set — a stock build cannot hit it — and because the alternative (a lazily
loaded, error-returning initializer called from bootstrap) would leave every `bundle` read that
happens before bootstrap seeing a different relation set than reads after it. A single
deterministic set, established before any consumer runs, is worth the panic.

## Testing

`pkg/lib/bundle/extra_test.go`, table-driven against `t.TempDir()`, calling
`loadExtraRelations` directly with a fresh `map[domain.RelationKey]*model.Relation` — no env
var manipulation, no reliance on package init.

| Case | Expectation |
|---|---|
| valid single relation | added to dst; `want` is a fully populated `*model.Relation` including `Id: "_br…"`, `Scope: Relation_type`, `ReadOnlyRelation: true`, mapped format and data source |
| `objectTypes` set | each entry prefixed with `_ot` |
| optional fields omitted | zero values, no error |
| key already in dst | dst entry unchanged (pointer-identical), no error |
| same key in two files | error naming both files |
| unknown `format` | error naming the field and value |
| unknown `source` | error naming the field and value |
| empty key / key with invalid chars (`my-key`, `my key`, `1key`) | error |
| `_score`-style key | accepted |
| empty name | error |
| negative or out-of-`int32` `maxCount` | error |
| empty or already `_ot`-prefixed object type | error |
| malformed JSON | error mentioning the path |
| missing file | error mentioning the path |
| two files, both valid | both merged |
| empty path list | no-op, no error |

The "stock build changes nothing" claim needs no test of its own: with the env var unset the
loader is never called, and the existing package tests plus `RelationChecksum` already pin the
generated bundle.

### The init wiring

Package init runs once per process, so the wiring is tested by re-executing the test binary as a
subprocess with `ANYTYPE_EXTRA_RELATIONS` set (`TestExtraRelationsInit` spawns
`TestExtraRelationsInitChild`, which skips unless it sees the marker env var). Two cases:

- **valid file** — the child asserts against real package state: `GetRelation` returns the extra
  with `Id: "_brextraInitTestRelation"`, `HasRelation` is true, and `LocalRelationsKeys` contains
  the key. That last assertion is the point of the subprocess: it proves the merge ran *before*
  `init` derived the key lists, which no in-process test can show.
- **invalid file** — the child process exits non-zero and its output names both the env var and
  the offending value, proving the panic path reports enough to diagnose the typo.

`extraRelationsPaths` gets its own two cases (unset yields nothing; a joined pair splits on
`os.PathListSeparator`) via `t.Setenv`.

## Files touched

| File | Change |
|---|---|
| `pkg/lib/bundle/extra.go` | new — env var name, `relationJSON`, `toModel`, `loadExtraRelations`, `extraRelationsPaths`, logger (127 lines) |
| `pkg/lib/bundle/extra_test.go` | new — table-driven loader tests plus the subprocess init tests (336 lines) |
| `pkg/lib/bundle/init.go` | 8 lines at the top of the existing `init()`; no new imports (`fmt` was already there) |

No changes to `core/indexer/reindex.go`, the generator, or any generated file.

## Rejected alternatives

**Fold the extras into the reindex checksum.** Make `RelationChecksum` a var computed from the
embedded JSON plus the extras, so `core/indexer/reindex.go:117` fires and the relations become
real objects in every space. Rejected: flipping the env var then churns a full bundled-relation
reindex on every account, and removing an extra later leaves orphaned relation objects with no
cleanup path. In-memory extras have no persistent footprint to clean up.

**Merge at generation time** — have the generator read the extra files. Rejected: it requires a
rebuild, which is the entire thing this avoids.

**Load through the config service** instead of reading the env var in `bundle`. Rejected:
`bundle` is a leaf package with no service dependencies, and consumers read `relations` long
before the app's config service is wired. Reading `os.Getenv` at init keeps the package a leaf
and matches the existing convention (`ANYTYPE_LOG_LEVEL`, `ANYTYPE_DISABLE_FT_INDEXER`,
`ANYTYPE_PARENT_INDEX_DEBUG` are all read this way, several in package-level vars).

**Hard-error on collision.** Consistent with fail-fast, but turns a future release that adds the
same key into a startup crash on an already-configured deployment. See the behavior contract.
