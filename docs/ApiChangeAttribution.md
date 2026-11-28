# API change attribution

Every change heart writes while serving a local HTTP API request (v1 or v2, app-key session) records the app's RAW name. Nothing else is recorded: no integration object, slug or icon. `docs/IntegrationAttribution.md` (an older, unmerged spec) is superseded.

## Decided (not open)

- **Trust.** No proof or extra signature: the account key signs the change and our own heart asserts the name, ONLY from the authenticated session (`core/api/server/middleware.go`), never from a request body.
- **Raw, never normalized**, bounded at key issuance. The stamp site does not re-check.
- **Scope.** HTTP API requests only. gRPC calls and background work are never stamped.
- **Cost.** One string on the change; empty is 0 bytes. No extra change is written.

## Where the name is visible

| Place | Field | Filled from |
|---|---|---|
| Tree change | `pb.Change.integrationName` (10, also `ChangeNoSnapshot`) | `smartblock.Apply` → `state.TakeIntegrationName` |
| Store change (chat, discussion) | `pb.StoreChange.integrationName` (10) | `store.PushStoreChange`, from its ctx |
| Object detail | `lastModifiedVia` (bundled, derived, read-only, short text) | the change `lastModifiedBy` is read from |
| Chat message | `model.ChatMessage.integrationName` (19) | the CREATING store change (`ChatHandler.BeforeCreate`) |
| Version history | `Rpc.History.Version.integrationName` (7) | each change's payload |

`StoreChange` shares `Change`'s field numbers, because history and the debug reader decode every payload as `Change`; the rule is in `changes.proto`.

## The carry

One rule: **the applied state carries the request**. `Apply` asks it (`TakeIntegrationName`). It answers from the applied state and the edit states it was layered on, the nearest layer first; within one layer the explicit stamp (2) wins over the session's name (1). The carries:

1. the state's `session.Context` — `mw.newContext(cctx)` copies the name off the request context, so every `NewStateCtx(ctx)` edit is stamped with no code at the site;
2. the explicit per-state stamp, for a writer that holds the request `context.Context` but builds `NewState()`: `st.StampFromCtx(ctx)`;
3. `session.AttributionOnly(ctx)`, for a path that takes a `session.Context` but must keep broadcasting its events as with `nil`. `NewStateCtx` turns it into (2). It is `nil` outside the API.

The document state is never consulted and `NewState()` never inherits, so a desktop edit a moment later carries nothing. Store pushes read their `ctx`; an apply on a store-backed object hands its name on (`detailscomponent.go`, `accountobject.go`, `virtualwidget.go`). A hook that writes other objects in the request reads `ApplyInfo.IntegrationName` (type → objects layout sync).

## Route inventory

"session", "stamp", "attr" = carries 1, 2, 3; "ctx" = store push.

| Route (v1 / v2) | Entry point | Stamped |
|---|---|---|
| POST objects (v1) | `mw.ObjectCreate`/`ObjectCreateBookmark`, then `ObjectRelationAddFeatured`, `BlockCreate`, `BlockPaste` | creation (Init) + session |
| POST objects, templates, queries, collections (v2) | `creator.CreateObjectFromSnapshot` | creation |
| PATCH object (v1) | `mw.ObjectSetDetails`, `ObjectSetObjectType`, `BlockListDelete`, `BlockPaste` | session |
| PATCH object (v2): properties, blocks, items, set_type, favorite | `mutator.MutateObject`, `mw.ObjectListSetIsFavorite` | stamp |
| DELETE object, POST restore, DELETE type/property/tag/file | `mw.ObjectSetIsArchived` → Bin collection | stamp |
| DELETE object `?permanent` (v2), DELETE file `?skip_bin=true` (v1; without it the file is only archived) | `mw.ObjectListDelete` | the Bin unlink, the uninstall mark, the default-template reset and the homepage reset: stamp. The Home unlink: NO (below). The tree deletion is a space-settings record, not a `pb.Change`: no field |
| POST/PATCH types, properties, tags | `mw.ObjectCreate*`, `mw.ObjectSetDetails`, `mutator.MutateObject` | creation / session / stamp; layout sync via the hook |
| POST/DELETE list objects (v1) | `mw.ObjectCollectionAdd/Remove` | session |
| POST files | `mw.FileUpload` | creation |
| POST/PATCH/DELETE widgets (v2) | `widgets.*` adapter | stamp (personal scope: ctx) |
| POST discussion (v2) | `mw.ObjectAddDiscussion` | creation, ctx, stamp on the parent |
| POST chats; POST/PATCH/DELETE message; reactions | `mw.ObjectCreate`, `mw.Chat*` | creation → ctx; ctx |
| POST spaces | `mw.WorkspaceCreate`, `WorkspaceSetInfo` | workspace details: attr. Bootstrap: NO (below) |
| PATCH space | `mw.WorkspaceSetInfo` | attr |
| chat read / read_all / reactions read | local read state, plus seen heads written to the tech-space key-value store | no change envelope to stamp |
| chat status, search, validate, auth | no write | nothing to stamp |
| members (v1, route disabled) | ACL records | out of scope |

Counts: 48 routes produce tree or store changes (23 v1, 25 v2); all 48 stamp the change(s) the request itself asks for, with three caveats: the tree deletion and the Home unlink of a permanent delete (2 routes), and the space bootstrap (1). 13 POST routes write nothing to a tree.

Fixed on the way: the two v1 list write handlers passed the gin context, which loses every session carrier (`core/api/handler/list.go`).

### Deliberately NOT stamped (heart's own work, even when a route triggers it)

- Load-time migrations: the init apply of an EXISTING object.
- Space bootstrap on POST spaces: mandatory objects, bundled installs, the use-case import, starting widgets (loader goroutine, `context.Background()`).
- Detached work: bookmark fetch and its images (v1), async upload completion after a paste, file sync/indexing status, GC archival, the workspace → space-view mirror, bundled template clones of a new type.
- **Inline, and still heart's own**: bundled template clones made when `set_type` or a create installs a bundled type, and the use-case import on POST spaces, run inside the request but on `context.Background()` on purpose. They must never count as app-created for DELETE ownership.
- A GC cascade: archiving a page stamps the Bin change of the ids the request named, not of the orphaned files the GC adds.
- **Accepted:** the Home unlink of an object a permanent delete removes, pages and files alike. It is develop's in-memory cleanup (`closeAndMarkDeleted`), which the deletion worker also runs for the same object and can reach first; it stays heart's own, unstamped, as on develop.

Unchanged: a bundled type/relation an API request INSTALLS is created with the request ctx and names the app. Those kinds are off the DELETE allowlist.

## DELETE ownership is unchanged

`?permanent=true` reads the FIRST non-root change's name (`objectprovenanceadapter.go`). Stamped edits cannot change the answer, because that change is always a creating change:

- A child of the root can only be pushed by an `Apply` on a root-only tree. `Apply` refuses that (`ErrApplyOnEmptyTreeDisallowed`) unless `AllowApplyWithEmptyTree`, which only `InitObject` passes, only for `IsNewObject`. An edit is therefore always a descendant of a creating apply, and tree order puts ancestors first.
- A creating apply takes its name from `InitContext.Ctx` as before; it has no session context, so the new carries add nothing to it.
- Derived trees (FileObject is the one on the allowlist): two devices may each push a creating change under one root. Both are creating changes; which is first was already decided by tree order.
- Imports and templates create through the same Init path. Chats and discussions are refused before provenance is read.

Tests: `TestSmartBlock_AnEditIsNeverATreesFirstChange`, `TestCreatorProvenance_EditsNeverOwn`, `TestDeleteObjectPermanently`.

## lastModifiedVia: it goes wherever lastModifiedBy goes

The rule is one sentence: **the app always comes from the same source as the author.** Which change counts as the latest, and when the index overrides the tree, is develop's behavior, unchanged; attribution inherits it, flaws included (below), and adds none.

- Set from change X: the app is X's name, REMOVED when X has none or no change was picked. For the local apply that is the very spot develop sets the author in — on the document, right before the push, with no event of its own.
- Restored from the index row on open (the row's author overrides the tree's, as always): the app is taken from that same row, set or removed, into exactly the states the author goes into — the edit state always, the document only when no pending local details hold the row back.
- Left alone: both are.
- **A bare write of the author clears the app.** `lastModifiedBy` has always been writable (a details RPC, a block bound to it); whatever sets or removes it in a state removes the app from that state, and a derivation sets the app again right after the author. So a written author never keeps the previous author's app.
- **Not writable** (`bundle.IsDerivedAttribution`): API v2 `set_properties`, API v1 (set and null), the editor's details setter and `ObjectListModifyDetailValues` refuse the key by name, no-op writes included; the state's generic setters (`SetDetail`, `SetLocalDetail`, `SetDetails` and the `Add*` ones) skip it, so a write of an object's whole details — which carries the key along — cannot put the previous author's app back next to a new author, whatever order its map is visited in; a block binding to the key does not project into it (its other binding still does); `Apply` makes a value no derivation put on the applied state the document's, comparing the key's presence and typed value (a value that is not a name is no app: removed) (`EnforceLastModifiedVia`).
- **Creation does not set it** (the creating change is not a user change, as for `lastModifiedBy`).
- **Store-backed objects (chats) never show it**: not after a push, not from the index on open, not in history-built details. Their details live in a store whose replay derives no author. Their changes are stamped all the same.
- **Kept with the author in a deletion snapshot** and in the default deletion-audit projection; **removed from an anonymized debug export's index row**.
- **Placed exactly like `lastModifiedBy`**: required internal + system relation, read-only, not hidden.

API v2 serves it as `last_modified_via` in `properties`. Follow-up outside heart: any-block's own vocabulary does not know the key.

### Same as lastModifiedBy, pre-existing, not fixed here (separate ticket)

Everything below already holds for `lastModifiedBy`/`lastModifiedDate` on develop; the app shares it, and is never wrong apart from them.

- **A failed push leaves the new pair in the live document** (it is set before the push), until reopen.
- **An open object learns of a new author or app the way it does on develop**: no event of their own; through the index and the next ObjectShow.
- **Snapshot boundary.** A build starts at the tree's root snapshot: an earlier, later-stamped edit behind it is not seen, and a build with no user change writes a participant id of an empty identity.
- **Slow clocks.** The latest change is the greatest timestamp (first met on ties), while the local apply always takes the pair: the two disagree until reopen.
- **A text block bound to it shows nothing**, like a block bound to any local or derived property: a binding is initialized from the synced details only. Accepted — it also keeps the app out of block text, block-create changes and the snippet.
- **Writable `lastModifiedBy` / `lastModifiedDate`.** A details write or an import can set them; `lastModifiedVia` alone is refused.
- **Incremental vs full replay**, and a stale index row overriding a newer tree on open.
- **A history restore** merges the restored version's author and date into the document until its own push sets them.

## Chat

Every API-authored `StoreChange` is stamped. The message field is creation-only: `BeforeCreate` sets it from the change envelope and re-marshals the document, which also deletes a key forged in a peer's payload; no modify path reads the envelope, and a modify of the key is refused. `AddMessage` strips the key from the payload it signs.

No clearing of rows stored before the field. A modify can only touch `reactions`, `content` (author only) and `pinned`, so a key in a pre-upgrade row can only come from the message's own create JSON, which its author signed: the author's claim about their own message, the same claim a modified client can make after the upgrade through `StoreChange.integrationName`. Accepted cosmetic leftover: such a row shows its claimed app on a device that stored it then, and nothing after a crdt.db rebuild.

Accepted limitation: a message an OLD client received before upgrading keeps no `integrationName` afterwards — replay skips stored messages. No backfill.

## History, debug export

One row per change. Rows group by time gap AND attribution, so a group never mixes an API change with an app change, or two apps (`versionAttribution`). A several-heads version has a synthetic id (hash of the SORTED heads); `Versions` — and `Show` of a tree that ends in several heads, when nothing listed it — records the heads and the one change the row is described by, under a lock, and `Show` answers with that same row, so the returned id round-trips to both branches. A historical view carries the object's own details once, under the object's id, in `Show` and in `DiffVersions` alike: a self-linked page does not get its current index row appended. A version can lack the `lastModifiedVia` relation link the live document has (a version stored before the upgrade, or one without an app; a snapshot written since may carry the link with no value, as snapshots store every relation link the state holds). On a restore, that difference pushes nothing: the removal of a local or derived relation link (`lastModifiedVia`, `isFavorite`, …) is filtered out of the change content. A restore whose content differs — a changed paragraph, say — pushes, as on develop, and makes the restorer the last editor. An anonymized debug export clears the name on tree and store changes.

## Task B (authoring device) — parked

Device attribution is parked; its current design keeps device proofs in the tech-space key-value store and adds no field to changes, so no change field is reserved for it. If it resumes, `objecthandler.LastModifiedInfo`, `storestate.ChangeAttribution` and `history.versionAttribution` are the structs a new attribute joins.

## Out of scope, and untested

Out: ACL records, tree deletion records, any-block's vocabulary, client UI, the creating app in HTTP chat DTOs and push payloads. No dedicated test (one stamp call each, on a carry tested elsewhere): the type/homepage side changes of a delete, the discussion's parent detail, bundled reinstall, workspace details on POST spaces, the paste upload block, the layout-sync option. `ObjectFactory.InitObject` itself is not driven by a test; its flag set (`InitApplyFlags`) is.

Pre-existing and left for separate tickets: the delete cleanup (`closeAndMarkDeleted`) passes the object's cached `isFavorite` back to the favorite setter, so a stale `true` can re-add the Home link of an object being deleted; a failed store commit can leave a permanent replay hole through the add-seq watermark (GO-7621); exclusive pagination of history repeats a several-heads synthetic version (GO-7622); synthetic version ids are held in memory without bound and lost on restart (GO-7623). Known limitation: a space that already has a custom property keyed `last_modified_via` collides with the provisioned relation in API v1/v2.
