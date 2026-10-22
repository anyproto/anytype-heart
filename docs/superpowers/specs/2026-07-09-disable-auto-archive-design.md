# Disable auto-archival and auto-restore of orphan files — Design

**Issue:** GO-7323 (continues the cascade-deletion / cleanup-suggestion work)
**Branch:** `go-7323-cascade-deletion-orphan-events`
**Date:** 2026-07-09

## Goal

Stop the backend from mutating objects on its own when a context is archived, deleted, or unlinked.
Every orphan — objects and files, at every level — is reported to the client as a
`CleanupSuggestion` candidate. The client decides whether to prompt in place (blocking) or merely
inform the user, who can clean up later via the on-demand `ObjectCleanupSuggestions` RPC.

Auto-archival of *objects* was already disabled: they only ever reach `OrphanCandidates.Candidates`.
This change extends the same treatment to files, which are the last thing the backend still archives
without asking.

The archival code is **retained behind a compile-time switch**, not deleted, so it can be restored
if user feedback calls for it.

## Non-goal

Chat attachment cleanup. Deleting a chat message still permanently deletes the files that message
introduced. That path is not context-driven archival; it is garbage collection for a resource with
no other owner, and it runs with no session context, so no confirmation event could ever be shown.

## Behavior contract

`OrphanCandidates.Files` — the "act on these without asking" list — becomes empty on every
context-driven path. `OrphanCandidates.Candidates` — the "ask the user" list — absorbs the files.

| Trigger | Before | After |
|---|---|---|
| Archive a page | level-1 orphan files auto-archived | files → `Candidates` |
| Delete a page | level-1 orphan files auto-archived | files → `Candidates` |
| Remove a link from a page | orphan file auto-archived | file → `Candidates` |
| Unarchive a page | archived child files auto-restored | nothing happens |
| Re-add a link (undo) | archived file auto-restored | nothing happens |
| Delete a chat message | own file deleted; another's file archived | **all** orphan files deleted |

## Architecture

### The switch

A single package-level constant in `core/block/objectgc/objectgc.go`:

```go
// autoArchiveOrphanFiles gates context-driven auto-archival and auto-restore of orphan files.
// Disabled: orphan files are reported as CleanupSuggestion candidates and the client decides
// whether to prompt in place or let the user clean up later. Retained so this can be re-enabled
// if user feedback calls for it.
const autoArchiveOrphanFiles = false
```

A constant rather than a config field: `objectgc` has no config dependency, `config.Config` is
local-only and not remotely togglable, and flipping a constant in a patch release is no slower than
flipping a local config value. The constant keeps the gated code compiled and type-checked.

### Gated sites

Four, all within `objectgc`:

1. **`collectOrphanedObjects`** — gate the level-1-file branch:
   `if autoArchiveOrphanFiles && current == objectId && isFile`. With the switch off, every orphan
   file joins `Candidates` regardless of level; with it on, the old level-1 behavior returns. The
   BFS itself is unchanged: files are still never descended into.

   (Gating rather than deleting the condition is what keeps the switch reversible — this is the site
   where it would be easiest to accidentally make the change one-way.)

2. **`ArchiveOrphansOnLinksRemoval`** — the `skipBin` branch still deletes (see *Chats* below). The
   else-branch appends to `Candidates` instead of `Files`, and the trailing
   `SetListIsArchivedNoGC(res.Files, true)` is guarded.

3. **`restoreObjectsOnUnarchive`** — early return. `collectOrphanedForRestore` remains, uncalled.

4. **`RestoreOrphansOnLinksAdded`** — early return, placed *before* its
   `SetListIsArchivedNoGC(toRestore, false)`.

### No caller changes

`delete.go:50`, `set_details.go:231`, and `smartblock.go:1672` already guard on `len(res.Files) > 0`
(or append an empty slice, which is a no-op). Once `Files` is always empty they do nothing, and the
`ObjectAutoArchive` / `ObjectAutoRestore` events stop firing on their own. The entire disable is
contained in one file.

This is the property that makes the switch cheap to flip back.

## Chats path

`ArchiveOrphansOnLinksRemoval(..., skipBin=true, ...)` currently degrades to *archive* when the file
was uploaded by another participant (`objectgc.go:193-199`, a `MyParticipantId` comparison). That
fallback is removed: `skipBin=true` now deletes unconditionally.

Rationale: only admins can moderate a chat, so a moderator deleting a message should remove its
attachments regardless of who uploaded them. Consistency beats the partial protection, which in any
case only moved the file to the bin.

This is a **permanent** change, orthogonal to the kill switch. Re-enabling `autoArchiveOrphanFiles`
does not resurrect the creator check.

Consequence: `ParticipantProvider` (interface, struct field, and `app.MustComponent` wiring in
`Init`) loses its only caller and is removed, along with `mockParticipantProvider` in the test
fixture. A wired-but-unused `app.MustComponent` is a trap for the next reader; restoring it is a
three-line diff.

## Protocol

No proto change.

`ObjectAutoArchive` and `ObjectAutoRestore` remain defined, and `objectgc.FilterExplicitIds` keeps
handling them. They simply stop being emitted. Removing them would be a breaking protocol change for
a switch we may reverse.

**Client-visible:** clients that handle `ObjectAutoArchive` will never receive it again. This is
silent — no error, no empty event — so the client team must be told explicitly. Conversely, the
`CleanupSuggestion` candidate list now contains file objects on paths where it previously contained
only non-file objects; clients already resolve each id's layout for display, so this needs no
client change, but the popup will list more items than before.

`ListOrphans` / `ObjectCleanupSuggestions` are untouched. They never mutated anything, and already
include files.

## Data migration

None, and one consequence worth stating: files auto-archived by earlier builds stay in the bin.
With auto-restore off, unarchiving their parent no longer pulls them back out, so a user who relied
on that round-trip must now restore them manually from the bin. This is a one-way effect on existing
data. It is not destructive — nothing is deleted — and it is reversed by flipping the constant.

## Error handling

Unchanged. The gated branches are the only removals; every existing error path (`fmt.Errorf(...: %w)`
wrapping, the `log.Warnf` non-fatal handling in `delete.go`) stays as-is. `deleteObject` failures on
the chats path continue to be logged and skipped rather than aborting the batch.

## Testing

The invariant worth locking in is *the archiver is never called on a context-driven path*. That is
what a regression would violate, and it is stronger than asserting an empty `Files` slice.

1. **New invariant tests** — after `CheckObjectsOnObjectArchived` (both directions) and
   `ArchiveOrphansOnLinksRemoval(skipBin=false)`, assert `fx.archiver.archivedIds` and
   `fx.archiver.unarchivedIds` are both empty.
2. **Reclassification** — the ~35 existing assertions in `objectgc_test.go` that expect a file in
   `res.Files` now expect it in `res.Candidates`. Level-1 and level-2 files become
   indistinguishable, so the tests that exist purely to prove that distinction collapse into one.
3. **Restore direction** — `RestoreOrphansOnLinksAdded` and the unarchive direction of
   `CheckObjectsOnObjectArchived` return nothing and call nothing.
4. **Chats** — a file created by *another* participant with `skipBin=true` is now deleted, not
   archived. This is the one test whose expectation inverts rather than moves; assert against the
   `objectDeleter` mock.
5. **Re-enable path** — no test may depend on `autoArchiveOrphanFiles` being `false` in a way that
   silently passes when it is flipped to `true`. Tests assert on the *observed* behavior of the
   current constant; flipping it is expected to fail the new invariant tests, which is correct and
   is how we would notice an accidental flip.

`detailservice` and `smartblock` tests use `ObjectGC` stubs and are unaffected.

## Rejected alternatives

- **Delete the archival code.** Cleanest end state, but the whole point is to watch user feedback
  and possibly reverse. Rejected.
- **Runtime config flag.** Would allow toggling without a rebuild, but requires plumbing a config
  dependency into `objectgc` for a value that is local-only anyway. Overkill for a decision we
  expect to make once.
- **Leave `res.Files` populated and ignore it at the call sites.** Spreads the disable across four
  files instead of one, and leaves a populated field that means nothing. Rejected.
