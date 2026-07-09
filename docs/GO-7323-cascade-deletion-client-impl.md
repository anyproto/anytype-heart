# GO-7323 — Cascade Deletion → user-confirmed object archival (desktop client brief)

**Audience:** anytype-ts (desktop) implementer.
**Type:** high-level idea + protocol contract. Code-level investigation (component reuse, exact wiring points) is for the implementing agent.

## Background

When a user creates an object **inside** another object, the backend records the parent in the
`createdInContext` relation. Previously, archiving/deleting that parent (or removing the link to
a created object) **implicitly archived** the whole nested subtree — objects and files alike.
User feedback: silently archiving nested *objects* is surprising.

New behavior (backend already shipped on `go-7323-cascade-deletion-orphan-events`):

- **Nothing is archived, restored, or deleted automatically.** Every orphan — nested objects at any
  depth, and files at any depth, including direct children — is reported in a **new event** listing
  the ids. The client decides whether to block on a confirmation popup or just inform the user, who
  can clean up later via `ObjectCleanupSuggestions`.
- The sole exception is **chat attachments**: deleting a chat message permanently deletes the files
  that message introduced, regardless of who uploaded them. Those files have no other owner and there
  is no session in which to ask.

## Behavior matrix

The backend no longer archives, restores, or deletes anything on its own. Every orphan — objects and
files, at every depth — is reported in the `CleanupSuggestion` event. **The client decides** whether
to block on a confirmation popup or merely inform the user, who can clean up later via
`ObjectCleanupSuggestions`.

| Trigger | Files (direct children) | Other orphans (nested objects + deep files) |
|---|---|---|
| Archive object A | **`CleanupSuggestion` event** → client decides | **`CleanupSuggestion` event** → client decides |
| Delete object A | **`CleanupSuggestion` event** (broadcast) | **`CleanupSuggestion` event** (broadcast) |
| Remove link A→X | **`CleanupSuggestion` event** | **`CleanupSuggestion` event** |
| Unarchive / undo | nothing | nothing (restored manually from the Bin) |
| Delete chat message | attachments permanently deleted | — |

> **Breaking for clients that handle `ObjectAutoArchive` / `ObjectAutoRestore`:** these events are
> **no longer emitted**. They remain defined in the proto, and no error is returned — they simply
> never arrive. Any UI that depended on them (e.g. an "N files moved to Bin" toast) must move to
> `CleanupSuggestion`, whose candidate list now contains file objects on paths where it previously
> carried only non-file objects. Clients already resolve each candidate's layout for display, so no
> parsing change is needed — the list is just longer.
>
> Files auto-archived by earlier builds stay in the Bin. Unarchiving their parent no longer pulls
> them back out; the user restores them by hand.

## Protocol changes (the contract)

### 1. New event — `Object.CleanupSuggestion`

`pb/protos/events.proto`, registered in the `Event.Message` oneof as `objectCleanupSuggestion` (field **146**):

```proto
message CleanupSuggestion {
  repeated string objectIds = 1;  // orphan ids (objects any level + files level >= 2) created within contextId
  string contextId = 2;           // the object that was archived / deleted / had a link removed
  Trigger trigger = 3;
  enum Trigger { archive = 0; delete = 1; linkRemoval = 2; }
}
```

- `objectIds` — the candidates to offer the user. May mix objects and (deep) files; the client can
  resolve each id's layout itself for display.
- `contextId` — the object the user acted on (use it to phrase the prompt, e.g. "Objects created in *Page X*").
- `trigger` — lets the client tailor copy (archive vs delete vs link removal).
- The event is emitted **only when there are candidates** (non-empty list).
- **Delivery:** for **archive** and **link removal** it rides the initiating session (the command
  response / that session's stream). For **delete** there is no session context, so it is
  **broadcast to the space** (all sessions). See Open Questions.

### 2. New request field — `skipCascade`

Added to **both** archive RPC requests:

- `Rpc.Object.SetIsArchived.Request` → `bool skipCascade = 3;`
- `Rpc.Object.ListSetIsArchived.Request` → `bool skipCascade = 3;`

Semantics: when `true`, the backend performs a **pure archive** of exactly the given ids — **no
cascade at all** (no file auto-archive, no `CleanupSuggestion` event). The client uses this on the
**confirmation calls** so that archiving the user-chosen objects does **not** trigger another round
of orphan detection (which would re-open the popup — an infinite loop). Default `false` preserves
the cascade behavior for normal user-initiated archive/delete.

### 3. Retired — `ObjectAutoArchive` / `ObjectAutoRestore`

Still defined in the proto, never emitted. The backend performs no automatic archival or restoration,
so there is nothing for them to announce. They are kept rather than removed because the decision is
gated behind a single backend constant (`objectgc.autoArchiveOrphanFiles`) and may be revisited after
user feedback; removing the messages would be a breaking protocol change for a reversible experiment.

Clients should stop relying on them. See the note in the behavior matrix above.

### 4. Phase-2 RPCs

The event above is the *reactive* half of cleanup suggestions. Phase 2 adds the on-demand half:

- `ObjectCleanupSuggestions(spaceId, keys)` — returns every orphan in the space as a forest: one
  item per orphan, with `isRoot` marking the forest roots and `reason` (`contextArchived`,
  `contextDeleted`, `contextUnlinked`) set on roots only. `keys` selects which relations come back in
  `details`; `id`, `createdInContext` and `resolvedLayout` are always included.
- `ObjectCleanupSuggestionIgnore(objectIds, ignored)` — permanently excludes objects from cleanup
  suggestions *and* from automatic context-driven archival, by setting the `createdInContextIgnored`
  relation. Reversible by passing `ignored=false`.

## Client flow

1. User archives / deletes an object, or removes a link (normal call, `skipCascade=false`).
2. Backend mutates nothing and sends `CleanupSuggestion` with every orphan id — objects and files.
3. Client receives `CleanupSuggestion` and chooses its UX: a **blocking confirmation popup**, or a
   non-blocking notice, leaving the user to clean up later via `ObjectCleanupSuggestions`.
4. User ticks the ones to archive and confirms (or cancels — nothing happens to unticked objects).
5. Client archives the chosen ids with **`ObjectListSetIsArchived(selectedIds, isArchived=true, skipCascade=true)`**.
   The `skipCascade=true` is essential — it prevents the re-prompt loop.
6. Anything the user left unticked stays active. Because the *entire* transitive subtree was already
   listed in step 2, there is no second-level prompt — one popup covers the whole impact.

## UX recommendation — reuse the Bin's tree view

The Bin already displays archived objects as an **expandable tree nested by `createdInContext`** —
which is exactly the relation that defines "orphans created inside" the acted-on object. So the
candidate set in `CleanupSuggestion` forms the **same tree shape** the Bin already knows how to render.

**Recommendation:** reuse the Bin's existing tree component inside the new popup rather than building
a new list, to avoid duplication. High-level fit (implementing agent to confirm specifics):

- The tree is the natural representation — users see parent→child structure and understand what
  archiving a parent implies.
- Add **checkboxes** to choose which objects to archive. Selecting a parent should **cascade-select
  its subtree** (archiving a parent implies its descendants), with the ability to deselect children.
- The popup confirms the selection and issues the `skipCascade=true` archive call.

**Reuse caveat to verify:** the Bin's tree subscribes to "all archived objects" (`isArchived == true`).
The popup instead needs to render a **specific provided id set** (the event's `objectIds`, which are
*not* archived yet). The implementing agent should check whether the tree component can be
parametrized by an injected id list / alternative data source; if selection is already controlled by
the parent (recommended pattern), embedding it in a popup should be a contained change rather than a
fork. If reuse turns out to require a large refactor, fall back to the existing selectable-object-list
popup pattern — but the tree is strongly preferred for clarity.

## Client work checklist (high level)

1. **Regenerate proto bindings** from the updated `.proto` (events + commands) — picks up the new
   event field and `skipCascade`.
2. **Handle the new event**: add `CleanupSuggestion` to the event mapper + dispatcher; on receipt, open
   the confirmation popup with `{ objectIds, contextId, trigger }`.
3. **Build the popup**: reuse the Bin tree view (per above) with checkboxes + confirm.
4. **Thread `skipCascade`** through the archive command wrapper and call it with `true` from the
   popup's confirm handler.
5. **Retire the `ObjectAutoArchive` / `ObjectAutoRestore` handlers.** They will never fire again. Any
   "files moved to Bin" toast should be dropped, or re-driven from the user's own confirmation in
   step 4 — the files now appear in the `CleanupSuggestion` candidate list like any other orphan.

## Open questions for the client team

- **Delete is broadcast to all sessions** (no session context at delete time) → the confirmation
  popup would appear on every device of the user, not just the initiating one. Confirm acceptable, or
  decide on session-targeted delivery.
- **Batch archive**: archiving multiple objects where one is an ancestor of another can produce the
  same candidate id under two `contextId`s (two `CleanupSuggestion` messages). The client should
  **dedup** ids across messages when populating the popup.
- **Event/flag naming** (`CleanupSuggestion`, `skipCascade`) is still provisional on the backend — flag
  if you'd prefer different names before this merges.
```
