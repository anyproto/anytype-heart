# spacev2 — Greenfield orchestration reimplementation handoff

You are replacing the anytype-heart space **orchestration** layer with a new design
in `space/spacev2/`. Read this fully before writing code.

## Goal

Build a **highly effective, simple, and testable** space orchestration layer.
Simplicity and testability are the primary success metrics — not fidelity to v1.

**Every architectural decision in scope is yours.** The current `spacev2` code
(state machine, registry, unidirectional wiring from the M1–M3 work) is *provisional
scaffolding, not a specification* — redesign any of it freely, including the state
machine itself, if a simpler or more testable shape serves the goal. Do not treat v1
*or* the existing spacev2 files as a template to preserve.

Optimize for: minimal moving parts, explicit dependencies and seams that make unit
testing easy, fast startup, bounded memory, and first-class lazy-load + pause/unload.

## Scope (locked)

| GREENFIELD — redesign freely | REUSE as-is — call into, do NOT rewrite here |
|---|---|
| `space.Service` **and its API** | `clientspace.Space` (object model, cache, mandatory objects, KV) |
| the per-space controller + lifecycle/mode model | `techspace` / `SpaceView` / `AccountObject` |
| state machine (or whatever replaces it) | `spacecore` (any-sync adapter + deterministic ids) |
| watcher, registry, factory | `spaceinfo` (status structs / relation mapping) |
| lazy-load + **pause/unload** + memory caps | storage + on-disk **migration** |
| load / offload / join orchestration | the byte-exact crypto helpers (account metadata, push keys, KV encoding) |
| deletion-controller **driver** (poll + offload) | any-sync ACL client, coordinator client |

You are rewriting *how spaces are brought up, tracked, and torn down*. You are **not**
rewriting the object model, the tech-space metadata layer, the any-sync adapter, the
status structs, or storage/migration — you call into them.

## The one hard boundary: compatibility

Because you **reuse** the layers that own the on-wire / on-disk / crypto formats,
network + data compatibility is largely *inherited*. Keep it that way:

- **Drive the reused layers with the same semantics:** create SpaceViews via `techspace`
  with the same relations/statuses; derive ids via `spacecore`; produce spaces via
  `clientspace.BuildSpace`. Existing accounts (new / migrated-from-badger-sqlite /
  joined / streamable / one-to-one) must keep loading and syncing across devices.
- **Reuse the exact derivation helpers — do not reimplement the algorithms:** space-id
  derivation (`spacecore`), account metadata (`domain.DeriveAccountMetadata`, SLIP-0021),
  push-notification keys, KV encoding. Call them; don't re-derive.
- Requirement level: **wire/network/crypto = byte-exact** (other devices depend on it);
  **local orchestration state = entirely yours** (how you track controllers in memory,
  ready-futures vs maps, etc.). Local on-disk format may change only via the reused
  migration path — but since storage is reused, you likely don't touch it at all.

## Full architectural freedom (within scope)

Explicit, so there is no ambiguity: **nothing about v1's or the current spacev2's
orchestration is prescriptive.**

- The state machine (`statemachine.go`), the mode set, the registry design, the
  unidirectional-vs-dual-path lifecycle, the controller interface, and the `Service` API
  shape are all yours. Choose the simplest correct, testable design.
- Prefer explicit dependencies + small interfaces + deterministic test seams over any
  pattern inherited from v1.
- `docs/SpaceController.md` **§9 is a hazard list, not invariants** — it records the
  concurrency problems v1 hit (re-entrant status-write storms, load/offload storage
  races, process restart-safety, dedup-map poisoning, tech-space lock reentrancy). Avoid
  these *hazards*; solve them however your architecture prefers. Do not port v1's
  specific mechanisms just because §9 describes them.
- `docs/SpaceController.md` **§11 recommendations are ideas, not requirements.**

## Inputs (read in this order)

1. **`docs/SpaceController.md`** — reference: what the subsystem does + the external
   contracts. Read §9 as hazards, §11 as ideas.
2. **v1 `space/`** — the behavior oracle for (a) the *call contracts* of the reused
   layers and (b) the byte-exact crypto/derivation helpers to reuse. Match behavior at
   those seams; ignore its internal structure.
3. This file.

## Outward API is greenfield

Redesign `space.Service` into a cleaner API. Because `clientspace.Space` is reused, it
stays the returned space object, so most consumers only touch the `Get`/`Wait`/`Create`/
`Join`/`Delete`-style entry points. Migrate consumers as part of cutover; a temporary
adapter that satisfies today's callers is acceptable during the transition.

## Operating model

- Build in parallel under `space/spacev2/`; keep v1 `space/` compiling and the whole
  tree green + tests passing at every step. No big-bang.
- **Cut over last:** register v2 as `client.space`, migrate consumers, then remove the
  v1 orchestration — staged, reviewable changes. The reused layers stay put.

## Verification (required, continuous)

- `go build ./...` stays green; `go test ./space/...` (+ new tests) per slice.
- **Compat tests** guarding the inherited boundary: assert v2 produces the same space
  ids, account-metadata payload, push keys, KV encoding, and SpaceView relation writes
  as v1 for identical inputs. (Reusing the helpers should make these pass by
  construction — the tests exist to catch accidental divergence.)
- Do not claim a milestone done without pasted build + test output.

## Done criteria

- v2 registered as `client.space`; v1 orchestration removed; the reused layers
  (`techspace`, `clientspace`, `spacecore`, `spaceinfo`, storage, migration) untouched.
- Demonstrably **simpler and better-tested** than v1: fewer moving parts, clear unit
  tests for the lifecycle (create / load / join / offload / delete / lazy / pause).
- All space tests pass; existing accounts load, sync, and show correct status;
  cross-device profile + push still work.

## Progress log (current state)

- **GO-7348**, branch `go-7348-spacecontroller-refactor`.
- The earlier M1–M3 code was stashed (`spacev2-agent-wip`); the current
  implementation is a fresh build on the **per-space reconciler** architecture —
  see `DESIGN.md` (normative) for the model and how it answers §9.
- Implemented and unit-tested (all `-race`, stress-run):
  - `state.go` / `controller.go` / `registry.go` — the lifecycle engine: pure
    `decide(status, wanted)`, one reconcile goroutine per space, input/decision
    seq freshness, controller-owned retry, first-class pause/unload.
  - `backends.go` / `presetloader.go` — production Backend over the reused
    layers; post-load domain components (aclobjectmanager & co) hosted with a
    preset-loader shim, so push keys / participants / notifications /
    OnSpaceLoad/Unload stay v1 code paths.
  - `bootstrap.go` / `watcher.go` / `service.go` / `api.go` — tech-space
    resolution decision tree (old-account fallbacks), SpaceView watcher,
    account bootstrap (new account = tech space + one *created* first space;
    the derived personal space is only for existing/old accounts), full verb
    surface (Get/Wait/Create/CreateOneToOne/Join/InviteJoin/CancelLeave/
    Delete/AddStreamable/Preload/SyncAllSpaceHeads/...), lazy mode as
    wanted=false, deletioncontroller.SpaceManager satisfied.
- Known deliberate deviations from v1 (all improvements, documented in code):
  marketplace reindex errors are not swallowed; Join always moves an existing
  view to Joining; AddStreamable is idempotent; offload tolerates missing
  storage; no waiting-map / dedupqueue anywhere.
- **Remaining: cutover (last step).** Register v2 as `client.space`
  (CName switch — v2 currently runs as `client.spacev2`), define the outward
  Service interface consumers compile against (or a temporary adapter in
  package `space`), migrate consumers + error values, remove v1 orchestration,
  and run integration/compat verification (existing accounts of all vintages
  load and sync; cross-device profile + push). Note: the coordinator-status
  kick is a single `UpdateCoordinatorStatus` on Run (v1 kicked per shareable
  controller Start); the deletion driver enumerates via `AllSpaceIds()` from
  the registry, which is populated by the watcher before the kick fires.
