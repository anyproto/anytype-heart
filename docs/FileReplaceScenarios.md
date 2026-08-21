# Replacing the File Behind an Existing File Object — Scenario Analysis

Date: 2026-08-06. Question: what breaks if we add "replace file" by **just swapping the file object's cid (fileId) and encryption key** — i.e. write a new `RelationKeyFileId` detail + `state.SetFileInfo{newFileId, newKeys}` and (attempt to) re-run upload. This document maps the current create → upload → backed-up → serve → offload/delete flow, then enumerates failure scenarios with verdicts.

Verdict legend:
- 🔴 **BROKEN** — the naive swap deterministically fails to function
- 🔴 **DATA LOSS** — user content can become unrecoverable
- 🟠 **RACE / STUCK** — concurrency window producing wrong state, stuck queues, or misattributed status
- 🟠 **LEAK** — quota/disk grows unboundedly
- 🟡 **NEEDS GUARD / POLICY** — works only with an explicit check or a product decision
- 🟢 **OK** — behaves acceptably as-is

---

## 1. Baseline: how the current flow works

### 1.1 Add (encrypt + DAG)
- `files.FileAdd` / `ImageAdd` (`core/files/files.go`, `images.go`) build an encrypted DAG. **fileId = CID of the root of the encrypted DAG** (`core/domain/file.go:8`). One random AES-256 key **per variant** (`getOrGenerateSymmetricKey`, files.go:453), encryption is **AES-CFB with a fixed zero IV** (files.go:407) — safety depends entirely on never reusing a key across plaintexts.
- Encryption is non-convergent (random key ⇒ new cid each add). Dedup is app-level: cross-space queries on `FileSourceChecksum` / `FileVariantChecksums` details (`core/files/queries.go:29,86`) can return an **existing** fileId+keys (`IsExisting: true`).

### 1.2 File object creation
- `fileobject.service.createInSpace` (`core/files/fileobject/service.go:431-484`): `makeInitialDetails` sets `FileId`, `FileBackupStatus=Queued`, `FileIndexingStatus=NotIndexed` (service.go:486-504); `createState.SetFileInfo{FileId, EncryptionKeys}` (service.go:448) puts (fileId, keys) into the object tree as a `ChangeSetFileInfo` change — **the sync-of-record for keys** (`core/block/editor/state/fileinfo.go`).
- The indexer flattens keys/variants into details: `FileVariantIds/Paths/Checksums/Mills/Widths/Keys/Options` + `SizeInBytes`, `FileMimeType`, `WidthInPixels`… (`InjectVariantsToDetails`, `core/files/fileobject/filemodels/filerequest.go:36`; `buildDetails`, `fileindex.go:288-317`), then stamps `FileIndexingStatus=Indexed` — **never reset anywhere in the repo**.
- On every state apply, keys are mirrored into a **global, cross-space, upsert-only** store keyed by fileId (`objectStore.AddFileKeys`, `pkg/lib/localstore/objectstore/filekeys.go:25`, wired via `detailsinject.go:167-175`).

### 1.3 Upload / backed-up marking
- `filesync` queue (`core/files/filesync`): persistent anystore collection, **item key = ObjectId** (`fileinfo.go:67`). `AddFile` is a **no-op if an entry exists in an "uploading" state — which includes `Done`** (`upload.go:49-52`, `fileinfo.go:25-33`). Entries are never deleted (`filequeue.Storage.delete` is dead code).
- Upload walk: `BlocksCheck` (availability) → `BlocksBind` (cids the node already has) → `BlockPushMany` (batched). All wire calls are scoped **(spaceId, fileId)** — the node has no concept of objectId (`rpcstore/store.go:346-416`).
- The push batcher is keyed by **fileId**, and if a batch is already open, a second caller's objectId is silently dropped (`batcher.go:52-60`).
- Status callbacks: `updateStatus` → `StatusCallback(objectId, fileId, status)` (`status.go:9-24`). The consumer that writes `FileBackupStatus` onto the object **ignores the fileId and latches at Synced forever**: `if prevStatus == Synced { return nil }` (`core/syncstatus/service.go:63-76`). No code resets `FileBackupStatus` off Synced.
- `markUploadedHook` (`core/block/editor/files.go:147-164`, HookAfterApply): any applied change carrying `FileBackupStatus=Synced` → `fileSync.MarkUploaded(objectId)` → forces the queue entry to `Done` — also fileId-blind (`upload.go:79-87`).
- Startup recovery: `ensureNotSyncedFilesAddedToQueue` re-enqueues objects with `FileBackupStatus != Synced` (`fileobject/service.go:262-286`).

### 1.4 Serving
- **The byte-serving path never reads `RelationKeyFileId`.** `GetFile`/`GetImage` (`core/block/editor/fileobject/fileobject.go:41-55`) rebuild readers from the flattened `FileVariant*` details (`GetFileInfosFromDetails`), and the gateway goes through the same path. `RelationKeyFileId` is only read by sync/offload/dedup/GC logic.

### 1.5 Offload / delete / GC
- `fileoffloader.fileOffload` (`offloader.go:94-110`): gate is **per-object** `FileBackupStatus == Synced` read from details; removes the DAG of the object's **current** fileId. Cross-object protection exists only cross-**space** (`offloadFileSafe`), not within a space.
- Object delete → `DeleteFileData` (`fileobject/service.go:704-742`): live **cross-space query** "any other object with this fileId?" — the only refcount mechanism; non-transactional (TOCTOU).
- `filesync.DeleteFile` supersedes any state unconditionally → `FilesDelete(spaceId, fileId)` RPC (`delete.go:17-105`).
- Reconciler (`core/files/reconciler/reconciler.go`): startup sweep deletes node-side fileIds referenced by no object (`reconcileRemoteStorage`, :187-243, keyed set `deletedFiles`), plus a per-apply `FileObjectHook` that re-enqueues upload when a live Synced object references a swept fileId — but that re-enqueue goes through `AddFile`, which no-ops on `Done` entries (pre-existing bug, see P1 below).
- `handleLimitReached` fires a **speculative, un-refcounted** `FilesDelete(spaceId, fileId)` whenever quota is hit (`upload.go:581-588`).

### 1.6 fileId sharing is normal
At least 10 paths make N objects reference one fileId: import reuse (`CreateFromImport`), `ObjectDuplicate` (copies fileInfo verbatim, `state.go:1367`), same-space block duplicate/paste, cross-space paste (`CreateFromImport` with same fileId), upload-time checksum dedup, node-side bind dedup, invite/identity icon payloads embedding raw (fileId, keys) out-of-band (`fileacl/service.go:57-101`). The DB index on fileId is deliberately non-unique (`spaceindex/store.go:315-319`).

### 1.7 History / undo
- History restore rebuilds `state.fileInfo` by replaying `ChangeSetFileInfo` changes — restoring an old (fileId, keys) onto the same object works today (`core/history/history.go:533-599`, `smartblock.ResetToVersion`).
- Ctrl+Z undo does **not** track `fileInfo` (`undo.Action` has no FileInfo field; `state.go:791-827` propagates fileInfo to parent but never into the undo action).
- Legacy-migrated file objects have their **object id derived from fileId** (`migration.go:107-120` → `DeriveObjectIdWithAccountSignature`).

---

## 2. The naive replace under analysis

On device A, for an existing file object O (currently `oldFileId`, status Synced):
1. `files.FileAdd(newContent)` → `newFileId`, `newKeys` (local DAG populated).
2. One Apply on O: `SetDetail(FileId, newFileId)` + `state.SetFileInfo{newFileId, newKeys}`.
3. `filesync.AddFile(O, newFileId)`.

Everything below assumes this minimal version unless stated.

---

## 3. Scenarios

### Group A — Deterministic breakage (naive swap does not work at all)

**A1. The new file is never uploaded.**
`AddFile` finds O's queue entry in `FileStateDone` (from the original sync) and returns without touching it (`upload.go:49-52` + `IsUploadingState` includes `Done`). The new fileId never reaches the node. The startup backstop (`ensureNotSyncedFilesAddedToQueue`) also can't help: it filters `FileBackupStatus != Synced`, and A3 keeps the status Synced; even if status were reset, its `AddFile` call hits the same `Done` guard.
**Verdict: 🔴 BROKEN.** filesync needs a first-class supersede path (e.g. `AddFile` overwrites when `req.FileId != stored.FileId`, or an explicit `ReplaceFile(objectId, oldFileId, newFileId)` transition).

**A2. The object keeps serving the old bytes.**
Readers/gateway serve from the `FileVariant*` details mirror, which is computed once at indexing and never refreshed; `FileIndexingStatus=Indexed` is never reset, so the 60s indexing provider will never pick O up again; the `File.Init` re-derive branch only fires when `FileVariantIds` is empty (`editor/files.go:123-141`). Result: sync/offload/GC think O is `newFileId` while every read returns `oldFileId`'s content.
**Verdict: 🔴 BROKEN.** The swap must atomically regenerate `FileVariant*` + `SizeInBytes`/`FileMimeType`/dimensions/`Name` (or reset `FileIndexingStatus`+clear variants and force reindex — but see C2 for why synchronous regeneration in the same Apply is strongly preferred).

**A3. Backup status lies: object shows Synced forever.**
`FileBackupStatus` is latched: the only writer refuses any transition away from Synced (`syncstatus/service.go:71`), and `Queued` is only ever written at creation. The UI reports the replaced file as backed up while nothing has been uploaded; downstream consumers (offloader gate B5, downloader C4, startup re-enqueue A1) all read this lie.
**Verdict: 🔴 BROKEN.** The swap Apply must itself reset `FileBackupStatus` (e.g. Queued), and the latch consumer must not be the only path allowed to write the detail.

**A4. Stale derived metadata.**
Subsumed by A2 but user-visible on its own: size, mime, dimensions, name, and the object's `file` simple-block all describe the old content until re-derivation.
**Verdict: 🟡** (fixed by the A2 requirement).

**A5. Other devices never re-download the new content for offline use.**
The eager downloader subscribes on `FileAvailableOffline == false && FileBackupStatus == Synced` (`filedownloader/downloader.go:111-135`). A device that had downloaded the old file has `FileAvailableOffline=true`; the swap doesn't reset it, so the new content is never eagerly fetched (lazy on-read fetch still works).
**Verdict: 🟠 NEEDS GUARD** — reset `FileAvailableOffline` in the swap Apply.

### Group B — Races on the replacing device

**B1. Replace while the original (or previous) upload is still in flight.**
All completion/error callbacks resolve by **objectId only**. Sequence: old upload's `BlockPushMany` completes after the swap → `updateUploadedCids(objectId, oldCids)` / `updateStatus(objectId, Synced)` act on the record that now describes the new file. Confirmed: the Synced status write lands on the replaced object (`syncstatus` ignores the fileId param). Plausible (needs verification): a late completion against a fresh record whose `CidsToUpload` plan is still empty can mark it `Done` outright (`batchuploader.go:59-125`), permanently skipping the new upload.
**Verdict: 🟠 RACE.** Every filesync callback and status write must compare the callback's fileId against the record's/object's current fileId before acting.

**B2. `MarkUploaded` force-completes the new upload.**
`markUploadedHook` fires on applies carrying `FileBackupStatus=Synced` and forces the queue entry to `Done`, fileId-blind. Two triggers: (a) locally — if the naive swap leaves status Synced (A3), a subsequent unrelated apply can kill the pending upload; (b) cross-device — a replicated change from a device whose latched Synced detail merges after the swap change. Either way the new file is marked backed-up without ever being pushed.
**Verdict: 🟠 RACE.** `MarkUploaded` must verify the fileId it is confirming.

**B3. Batcher misattribution when two objects share an in-flight fileId.**
If the replacement content dedup-hits an existing fileId that another object is concurrently uploading, the fileId-keyed batch keeps the first caller's objectId and silently drops the second (`batcher.go:52-60`); the second object's queue entry can stay `Uploading` with a non-empty plan forever.
**Verdict: 🟠 RACE / STUCK** (latent today, made reachable by replace + dedup).

**B4. Late error callbacks resurrect superseded entries.**
`addToLimitedQueue` / `addToRetryUploadingQueue` don't check the record's current state (`batchuploader.go:32-57,80-90`). If the replace flow does `DeleteFile(old)` → `AddFile(new)`, a late error from the old file's push can flip the new record to `Limited`/`PendingUpload` based on the old file's failure — wrong quota state or a gratuitous retry against the wrong plan. (Pre-existing gap; replace makes the delete-then-add sequence routine.)
**Verdict: 🟠 RACE** — same fix as B1: fileId-guard the callbacks.

**B5. Offload during the replace window destroys the only copy of the new file.**
With status still Synced (A3), `FileOffload`/`FileSpaceOffload`/`FileListOffload` pass the `Synced` gate (`offloader.go:105-107`) and remove the DAG of the object's **current** fileId — the new one, whose blocks exist nowhere else yet. The pending upload then fails local reads, gets parked in `MissingBlocks` (10min→24h backoff), and the content is gone from Anytype entirely.
**Verdict: 🔴 DATA LOSS.** Requires status reset in the same Apply (A3) **and** ideally an offloader check against the filesync queue state, since a status-reset race window remains between Apply and detail visibility.

**B6. Refcount TOCTOU on old-file cleanup.**
Any replace flow that deletes the old fileId must use the `DeleteFileData`-style cross-space query first; that query-then-delete is non-transactional, and a concurrent import/paste/duplicate referencing oldFileId in the window loses its data on the node. Pre-existing race, but replace multiplies delete frequency.
**Verdict: 🟡 NEEDS GUARD** (accept the small window, or serialize deletes through a queue that re-checks at execution time).

**B7. Crash between the swap Apply and the queue operations.**
The Apply is durable in the tree; `DeleteFile(old)` / `AddFile(new)` are separate anystore writes. A crash in between leaves state and queue disagreeing. Recovery must be derivable from state at startup — which today it is not (A1+A3 block `ensureNotSyncedFilesAddedToQueue`).
**Verdict: 🟠** — fix ordering (Apply first, queue ops idempotent from state) plus the A1/A3 fixes make the startup pass a real backstop.

### Group C — Multi-device / offline

**C1. Replace while offline; other devices online.**
The swap change syncs through the space tree as soon as A reconnects, typically before the blocks finish uploading. Other devices see the new fileId with content not yet fetchable; the gateway's `WaitAvailable` + retry semantics cover this the same way they cover a freshly created file. Degradation is bounded by how long A stays offline — identical to today's create-offline UX.
**Verdict: 🟢 OK** (assuming Group A fixes; show "syncing" status honestly per A3).

**C2. Concurrent replace on two devices.**
`SetFileInfo` is whole-struct replace and details are per-key LWW, so if each device's swap is **one atomic Apply** (fileId + fileInfo + all variant details + statuses in a single change), convergence picks one side wholesale — correct. But if metadata regeneration is **async** (separate later change, as the current indexer pipeline would do), changes interleave across devices: final state can be device B's fileId with device A's variant keys/checksums — the serving path then decrypts the wrong DAG with the wrong keys (garbage or stale bytes), and if the losing side never re-indexes, the mixed state is stable.
**Verdict: 🟠 RACE.** Hard requirement: the entire swap (fileId, keys, variants, derived details, status resets) is one Apply/one change. The loser's uploaded fileId becomes an orphan on the node — cleaned by the reconciler sweep eventually (see D1).

**C3. Replace racing object deletion from another device.**
Delete wins: `DeleteFileData` refcounts and deletes the fileId it reads at delete time; the other fileId of the pair (old or new depending on merge order) is left bound with no object → orphan until the reconciler sweep.
**Verdict: 🟡** — acceptable, relies on the sweep (D1).

**C4. Version-history restore of a pre-replace version.**
Replaying `ChangeSetFileInfo` restores oldFileId+oldKeys onto the object — mechanically works, and the global key store still has the keys (upsert-only). But: (a) if replace deleted oldFileId from the node, the restored file is remotely unfetchable; (b) the restored state carries `FileBackupStatus=Synced`, so `markUploadedHook` re-latches `Done` (B2); (c) the reconciler's rebind for swept fileIds no-ops (P1). Net: restore shows a Synced file that no device can download.
**Verdict: 🟠 POLICY + FIXES.** Decide: keep old fileIds bound for history (quota cost) vs. accept that history restore of file content dies with replace; either way fix P1 and B2.

**C5. Ctrl+Z undo of a replace.**
`undo.Action` doesn't capture `fileInfo`: undo reverts the FileId detail and variant details but not `state.fileInfo` → the two representations diverge (sync/GC follow the detail, key-mirroring follows fileInfo). Depending on which side later wins, this yields wrong keys in the global store or GC against the wrong fileId.
**Verdict: 🟡 NEEDS GUARD** — either extend `undo.Action` with FileInfo or make replace non-undoable (undo = replace back, going through the same full flow).

### Group D — File node, quota, cleanup

**D1. Old fileId stays bound: quota leak.**
The object isn't deleted, so nothing calls `DeleteFileData`; oldFileId's cids stay bound to the space, double-counting quota. The reconciler sweep would eventually catch it (no object references oldFileId anymore) — but it runs at startup only, uses the empty-objectId sentinel, and marks the fileId in `deletedFiles`, which then poisons C4/D6.
**Verdict: 🟠 LEAK / POLICY.** Replace needs an explicit, refcount-checked deletion of the old fileId (reusing the `DeleteFileData` query), with the C4 history policy decided consciously rather than by sweep side-effect.

**D2. Ordering under quota pressure.**
Upload-new-then-delete-old needs transient headroom for both files; if the space is near its limit the new upload lands in `Limited` while old still occupies quota — replace appears hung. Delete-old-first frees quota but creates a window where **neither** version exists on the node; a crash/offline in that window leaves other devices unable to fetch anything, and if A then dies, the old content is unrecoverable remotely.
**Verdict: 🟡 POLICY.** Recommended: upload-first, delete-old only after new reaches Done; surface Limited explicitly; never delete-first.

**D3. Speculative delete on quota hit can nuke a shared fileId.**
`handleLimitReached` fires `FilesDelete(spaceId, fileId)` "just in case", un-refcounted. If the replace's new content dedup-shares its fileId with another already-Synced object in the space, hitting the limit deletes that object's remote data too.
**Verdict: 🟠** — pre-existing landmine; replace makes limit-hits on shared fileIds likelier. The speculative delete should run the same other-referents query.

**D4. Old fileId shared with another object.**
Duplicated objects, same-space pastes, and import reuse all share fileIds (§1.6). Replace-on-A + unconditional delete-old breaks object B remotely: B still shows Synced, its local blocks may later be offloaded, and then B's content is gone for every device.
**Verdict: 🔴 → 🟡 with guard.** The old-file delete must be conditional on the cross-space referents query (exactly `DeleteFileData` semantics, excluding O itself).

**D5. Replace resolving to an existing fileId (dedup), including itself.**
`FileAdd` may return `IsExisting` with a fileId already owned by another object (fine — but the delete guard D4 and batcher race B3 become live), or the object's **own current** fileId (replace-with-identical-content). The naive `DeleteFile(old) → AddFile(new)` with old==new deletes the file from the node and then re-uploads it — transient remote absence for all devices.
**Verdict: 🟡 NEEDS GUARD** — short-circuit no-op when newFileId == oldFileId; also note local quota allocations are objectId-keyed (`nodeusage.go:91-118`) so dual references double-count transiently (cosmetic).

**D6. Replacing TO a previously swept fileId.**
If the target fileId is in the reconciler's `deletedFiles` set (it was orphaned once), `FileObjectHook` will try to rebind — via `AddFile`, which no-ops on `Done` (P1). Combined with D1's sweep, a replace-back-to-old-content (or C4 restore) can leave the object permanently absent from the node while marked Synced.
**Verdict: 🟠** — fixed by P1 + fileId-aware `AddFile`.

### Group E — Ecosystem and product semantics

**E1. Invites / identity profiles embed raw (fileId, keys) out-of-band.**
Space icons and profile pictures get their fileId+keys copied into invite payloads and identity broadcasts (`fileacl`, `inviteservice`, `identity`). An in-place replace of such a file object doesn't update those payloads; once the old fileId is GC'd, cached invites/identity icons break. Today's flows sidestep this because "changing" an icon swaps the **object id** in a relation.
**Verdict: 🟡 NEEDS GUARD** — regenerate invite/identity payloads on replace of referenced icon objects, or exclude icon objects from in-place replace.

**E2. Dedup-index poisoning from half-updated objects.**
The dedup queries answer from object details. An object carrying newFileId with old `FileVariantChecksums` (the A2 state) makes a future upload of the **old** content resolve to the **new** fileId — that new object then displays the wrong file.
**Verdict: 🟠** — eliminated by the atomic-Apply requirement (C2); worth a regression test.

**E3. Legacy migrated file objects: object id is derived from fileId.**
For migration-created objects, `objectId = derive(fileId)` (`migration.go:107-120`). A swap breaks that cryptographic binding: any code or future migration that re-derives the id from a fileId (late-migrating device, re-import of the old fileId) resolves to this object and assumes it still holds the old content.
**Verdict: 🟡 NEEDS GUARD** — refuse in-place replace on derived-id objects (detectable via the unique key), or explicitly audit every derivation site first.

**E4. fileId→objectId lookups get ambiguous / import resurrection.**
`GetObjectDetailsByFileId` picks `records[0]` arbitrarily among multiple matches. After a replace, an import that carries the old fileId no longer finds O, so it creates a fresh object referencing oldFileId (keys still in the global store) — re-referencing data that replace may have just deleted/swept from the node; recovery depends on the broken rebind chain (P1).
**Verdict: 🟡** — with P1 fixed and D1's refcounted delete, this degrades to "import re-uploads the old content", which is acceptable.

**E5. References see silently mutated content.**
Chat attachments, mentions, and file blocks all reference the **objectId**; every past message/embed now renders the new content. That's the point of the feature, but it rewrites the meaning of historical messages ("the file I sent you" changes after the fact).
**Verdict: PRODUCT DECISION** — consider surfacing modification (e.g. `lastModified`/badge), and decide whether chat-attached files are replaceable.

**E6. Key hygiene is a hard constraint on the API surface.**
AES-CFB with a fixed zero IV means key reuse across different plaintexts is catastrophic (keystream reuse → plaintext recovery). A replace API must always mint fresh random keys for new content and must never accept "reuse the object's existing key" as an optimization. The import path's `CustomEncryptionKeys` pass-through must not be reachable from replace with an old key + new content.
**Verdict: 🔴 SECURITY CONSTRAINT** (on design, not a race).

---

## 4. Pre-existing bugs surfaced by this analysis (independent of the feature)

- **P1. Reconciler rebind is a no-op**: `FileObjectHook` → `rebindHandler` → `AddFile`, but the object's queue entry is `Done` and `AddFile` ignores it (`upload.go:49-52`). The repair path for "node lost my synced file" appears dead. (Static trace; verify with a test.)
- **P2. `onBatchUploadError` resurrects deleted queue entries** (no state check; `batchuploader.go:32-57,80-90`).
- **P3. Batcher drops the second objectId for a shared in-flight fileId** (`batcher.go:52-60`).
- **P4. Speculative quota delete is un-refcounted** (`upload.go:581-588`) — can delete a fileId another Synced object references.
- **P5. `filequeue.Storage.delete` is dead code** — the filesync queue collection grows monotonically forever.
- **P6. `DeleteFileData` refcount is TOCTOU** (query-then-delete, no serialization).

---

## 5. Minimal requirements for a correct replace

1. **filesync first-class supersede**: `AddFile` must overwrite the entry when the requested fileId differs (or add `ReplaceFile(objectId, oldFileId, newFileId)`); `MarkUploaded` and every batcher/status callback must compare fileIds before acting (fixes A1, B1, B2, B4, D6).
2. **One atomic Apply** containing: `SetFileInfo{new}`, `FileId` detail, regenerated `FileVariant*` + derived details + file blocks, `FileBackupStatus→Queued`, `FileIndexingStatus` consistent, `FileAvailableOffline→false` (fixes A2–A5, C2, E2). Synchronous metadata injection (the DAG is local at this point) — do not reuse the async indexer for the swap.
3. **Allow status downgrade**: the syncstatus Synced latch must permit the transition when the object's fileId changed (fixes A3; closes B5's main window).
4. **Refcount-checked, deferred old-file deletion**: after new reaches `Done`, run the `DeleteFileData` cross-space query for oldFileId (excluding O) and only then `DeleteFile`; never delete-first (fixes D1, D2, D4; policy for C4 decided explicitly).
5. **No-op when newFileId == oldFileId**; handle `IsExisting` dedup results (D5).
6. **Guard rails**: refuse replace on legacy derived-id objects (E3); refresh or exempt invite/identity icon objects (E1); fresh random keys always (E6); undo either extended to fileInfo or routed back through replace (C5).
7. **Fix P1** regardless — replace increases traffic through the rebind path.

## 6. Sources

Mechanism map assembled 2026-08-06 from four parallel code surveys over `core/files` (files, fileobject, filesync, filestorage, fileoffloader, filedownloader, reconciler, fileacl, fileuploader), `core/block/editor` (state/fileinfo, smartblock hooks, file blocks, clipboard/basic duplication), `core/syncstatus`, `core/history`, `pkg/lib/localstore/objectstore`, and `any-sync@v0.12.16` `commonfile/fileproto`. Line numbers reflect the develop worktree on that date.
