# spacev2 — Clean-room reimplementation handoff

You are reimplementing the anytype-heart **space subsystem** as the `spacev2`
package (`space/spacev2/`). This brief is your mission, ground rules, and
done-criteria. Read it fully before writing code.

## Inputs (read first, in this order)

1. **`docs/SpaceController.md`** — the design spec / north star. It is the primary
   reference: responsibilities, the any-sync/tech-space/space-view stack, the
   controller contract, the lifecycle, the outward API, the concurrency invariants
   (§9), the data/sync-plane blind spots (§10), and the reimplementation notes (§11).
2. **The existing `space/` package (v1)** — the **behavior + byte-exact-contract
   oracle**. When the doc says "must stay byte-identical" or you need an exact wire
   format / derivation / relation key, read the v1 code. Treat it as an oracle to
   match, **not** a template to copy — v1's structure has the pain points §11 lists.
3. This file.

## Operating model (do not deviate without asking)

- **Do NOT delete or edit v1 `space/`.** It stays compiling and running so the whole
  repo builds and you have a working reference + a parity target throughout.
- **Build v2 in parallel** under `space/spacev2/` (this scaffold). The package is
  currently inert (`errNotImplemented`) and **not registered** in
  `core/anytype/bootstrap.go`. Keep it that way until the parity gate passes.
- **Keep the tree compiling and tests green at every step.** Run `go build ./...`
  and the space tests continuously. No "big bang" — land it in reviewable slices.
- **Cut over last:** only after v2 passes parity, switch `core/anytype/bootstrap.go`
  to register v2 (rename `CName` to `client.space`), then remove v1 in a final,
  separate change.

## Hard contracts — preserve exactly (breaking these loses user data or breaks interop)

These are non-negotiable; a mismatch silently corrupts existing installs (see
`docs/SpaceController.md` §8, §10, §11 "Keep"):

1. **`clientspace.Space` and `space.Service` outward surfaces.** The `spacev2.Service`
   interface (service.go) mirrors v1 verbatim. Consumers (§5.2/§5.3: `block.Service`,
   `core/acl`, `crossspacesub`, indexer, chats, files, history, RPC layer) depend on
   it. Change it only with an adapter + consumer updates.
2. **Deterministic space-id derivation** — personal/tech/one-to-one ids are a pure
   function of (account signing key, space type). Existing ids cannot change.
3. **SpaceView relations = a cross-subsystem contract.** The persistent-vs-local
   (synced-vs-not) split, the status relations, `SpaceOrder`, the push-key/`IsAclShared`
   relations, and the Workspace→SpaceView mirror (`workspaceKeysToCopy`) are read by
   `crossspacesub`, `spacesyncstatus`, and `pushnotification`. Keep the keys and
   semantics; keep emitting `OnSpaceIndexOpened`.
4. **Crypto derivations, byte-exact:** account metadata (SLIP-0021
   `m/SLIP-0021/anytype/account/metadata`), push-notification keys
   (`aclobjectmanager/pushnotificationkeys.go`), and the key-value service privacy
   encoding. Cross-device/-member decryption depends on these.
5. **On-disk storage + migration.** Preserve the anystore layout and the
   badger/sqlite→anystore migration (`spacecore/storage/migrator` + `migratorfinisher`,
   `AccountMigrate`). New installs and migrated installs must both work.
6. **`SpaceLoaderListener`** (OnSpaceLoad/OnSpaceUnload), the `objectId→spaceId`
   `bindId` binding written via `source.Service.NewSource`, and the notification /
   `session.Context` side effects (§3.4, §5.3).
7. **The documented error set** (`ErrSpaceNotExists`, `ErrSpaceDeleted`,
   `ErrSpaceIsClosing`, `ErrSpaceStorageMissig`, `ErrFailedToLoad`) — or map v2 errors
   onto them.

## Invariants you must not break (docs/SpaceController.md §9)

Re-read §9 before touching lifecycle/concurrency. The load-bearing ones:

- Status writes are **`Equal()`-guarded** (breaks a re-entrant write→watcher→write loop).
- Pipeline components publish lifecycle changes **through the SpaceView**, never by
  driving their own controller (self-close deadlock otherwise).
- **Close/drain the old process before building the next**; each process instance is
  **fresh** (closed any-sync child apps can't restart).
- StateMachine: reject divergent concurrent transitions; single-writer loop.
- Don't poison/leak the in-flight build dedup map; register the dedup entry before the
  SpaceView is created.
- `techspace.Do*` locks are **non-reentrant**; spacecore ocache `TryClose` stays false;
  `techSpaceReady` is the tech-space pointer barrier; `Close` cancels ctx before setting
  `isClosing`.

## API improvements you SHOULD make (docs/SpaceController.md §11)

The internal (non-`Service`) contract is yours to improve:

- Typed `WaitLoad` on the controller (done in controller.go) instead of `Current() any`.
- Make `CanTransition` authoritative **or** drop it — but offloading is **not** terminal
  (`CancelLeave`/restore needs Offloading→Loading).
- Collapse dead `AccountStatusRemoving`; drop the deprecated marketplace/VirtualSpace
  path (GO-6259).
- Consider the unidirectional lifecycle model (write SpaceView → watcher builds
  controller) as the single path, replacing v1's dual create/join build+watcher dedup.
  This is an **open decision** (§11 candidate 1) — confirm before committing to it.

### Forward-looking goals to design for (§11)

- **First-class lazy load + pause/unload:** a real reversible `ModePaused` state (see
  controller.go), subsuming v1's defer-only lazy mode.
- **Bounded resident memory:** cap concurrently-loaded spaces, evict by LRU/idle, make
  the per-space footprint releasable.
- **Prepare for per-network spaces:** keep network identity **per-space** in the model
  (route coordinator/peer/credential resolution through a per-space handle) rather than
  the account-wide singleton. Full support needs any-sync changes; just don't bake the
  single-network assumption into the controller/spacecore contract.

## Suggested milestones (each independently buildable + testable)

1. **Skeleton + types** (this scaffold): `Service`, `SpaceController`, `Mode`, status model.
2. **spacecore/tech-space integration**: load/derive spaces, tech-space + SpaceView CRUD,
   the reactive watcher. Parity: create/load an existing account, enumerate SpaceViews.
3. **Controller + state machine + load pipeline**: bring a space to a usable
   `clientspace.Space`; `Get`/`Wait`. Parity: open objects across spaces.
4. **Join / offload / delete / remote-status / deletion controller.**
5. **Lazy load + pause/unload + memory caps** (the new capability).
6. **Migration + platform layer** (§10) wired through.
7. **Cutover**: register v2 in bootstrap, remove v1.

## Verification (required, continuous)

- `go build ./...` stays green; `go test ./space/...` (port v1 tests — they encode
  invariants the doc only summarizes; see §10) plus new tests per milestone.
- **Parity oracle checks** for the byte-exact contracts: assert v2 derives the same
  space ids, account-metadata payload, push keys, KV encoding, and SpaceView relation
  writes as v1 for the same inputs. Add these as tests, not manual checks.
- Do not claim a milestone done without the build + relevant tests passing (paste output).

## Done criteria

- v2 registered as `client.space`; v1 `space/` removed.
- All existing space tests (ported) + new tests pass; `go build ./...` green.
- No consumer changed except the bootstrap registration (or documented adapter).
- Existing accounts (new, migrated-from-badger/sqlite, joined, streamable, one-to-one)
  load, sync, and show correct status; cross-device profile/push still work.
