# Spec: FT queue generations (B1)

## Problem

`FtQueueMarkAsIndexed` replaces the whole queue document with `{id, spaceId, seq}`. Anything
that happened to the entry between `ListIdsFromFullTextQueue` and the mark — a chat message
deletion (`del`), a new message order id (`ord`), or a plain re-enqueue after an edit — is
silently erased and the entry is marked as indexed. The processing window is a whole batch
(up to 1000 objects, seconds to minutes), so this is a real, reproducible loss
(`TestFtQueueMarkAsIndexedPreservesConcurrentUpdates`). Recovery only happens at the
once-per-session consistency check, which does not cover message-level diffs at all.

There is no way to fix this with the current schema: a pending entry looks identical whether it
was enqueued before or after the indexer captured the object state.

## Design

Add a generation stamp to every queue entry:

- `gen` (uint64): set to `GenerateFTQueueCounter()` on EVERY enqueue-type mutation:
  `AddToIndexQueue(WithCounter)`, `AddChatMessageToIndexQueue`, `AddChatMessageDeleteToIndexQueue`,
  `EnqueueAllForFulltextIndexing`, `FtQueueReconcileWithSeq`'s reset.
  - The current `anyencutil.Equal` short-circuit ("entry unchanged → skip write") goes away for
    pending entries: bumping `gen` is the point. Entries are tiny; the extra write is acceptable.
  - The existing dedupe semantics for chat `ord` (keep the smaller order id) stay; only `gen`
    always moves forward.

- `ListIdsFromFullTextQueue` returns `gen` per entry (`domain.FullTextQueuedObject.Gen`).

- `FtQueueMarkAsIndexed(refs, seq)` takes the listed objects (id + gen), and per entry runs a
  `ModifyFunc` that:
  - if `v.gen != ref.gen` → returns unmodified (the entry was re-dirtied mid-batch; it stays
    pending with its `ord`/`del` intact);
  - else sets ONLY `seq` (no document replacement), preserving all other fields.

- Indexer plumbing: `processQueuedObjects` already receives `[]domain.FullTextQueuedObject`;
  `succeedIds` changes from `[]domain.FullID` to carrying the listed `gen` (signature change of
  `domain.FullTextProcessFunc` and `FtQueueMarkAsIndexed`).

## Why gen, not field-preserving merge only

Preserving `ord`/`del` in the ModifyFunc fixes the chat-field loss but still marks the entry as
indexed (`seq` set) even when it was re-dirtied — the re-enqueue is what must survive. The
generation comparison handles every mutation uniformly, including plain object re-enqueues.

## Compatibility

Existing entries have no `gen` (reads as 0). First post-upgrade listing returns gen 0; a mark
with gen 0 matches only entries that were not re-dirtied (any new enqueue writes gen > 0) —
exactly the intended semantics. No migration needed.

## Effects on opstamp reconciliation

Unchanged. `seq` semantics stay the same; `FtQueueReconcileWithSeq` keeps comparing `seq` with
the tantivy commit opstamp. A skipped mark just leaves `seq = 0` (pending), which the loop picks
up on the next tick.

## Tests

- Repro test (both sub-tests) turns green.
- Mark with stale gen leaves `ord`/`del`/pending intact; mark with current gen sets seq and
  preserves remaining fields.
- Mixed batch: some entries re-dirtied, some not — only the latter get marked.
