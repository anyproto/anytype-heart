# GO-7323 — Cascade Deletion → user-confirmed object archival (desktop client brief)

**Audience:** anytype-ts (desktop) implementer.
**Type:** high-level idea + protocol contract. Code-level investigation (component reuse, exact wiring points) is for the implementing agent.

## Background

When a user creates an object **inside** another object, the backend records the parent in the
`createdInContext` relation. Previously, archiving/deleting that parent (or removing the link to
a created object) **implicitly archived** the whole nested subtree — objects and files alike.
User feedback: silently archiving nested *objects* is surprising.

New behavior (backend already shipped on `go-7323-cascade-deletion-orphan-events`):

- **Files** that are direct children of the acted-on object are still **auto-archived** — no change,
  still reported via the existing `ObjectAutoArchive` event.
- **Everything else orphaned** (nested objects at any depth, plus files deeper than level 1) is **no
  longer archived automatically**. Instead the backend emits a **new event** listing those object
  IDs so the client can show a **confirmation popup** and let the user pick what to archive.

## Behavior matrix

| Trigger | Files (direct children) | Other orphans (nested objects + deep files) |
|---|---|---|
| Archive object A | auto-archived + `ObjectAutoArchive` | **`OrphansDetected` event** → popup |
| Delete object A | auto-archived | **`OrphansDetected` event** (broadcast) → popup |
| Remove link A→X | auto-archived (if X is a file) | **`OrphansDetected` event** → popup |
| Unarchive / undo | auto-restored | nothing (objects restored manually from the Bin) |

## Protocol changes (the contract)

### 1. New event — `Object.OrphansDetected`

`pb/protos/events.proto`, registered in the `Event.Message` oneof as `objectOrphansDetected` (field **146**):

```proto
message OrphansDetected {
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
cascade at all** (no file auto-archive, no `OrphansDetected` event). The client uses this on the
**confirmation calls** so that archiving the user-chosen objects does **not** trigger another round
of orphan detection (which would re-open the popup — an infinite loop). Default `false` preserves
the cascade behavior for normal user-initiated archive/delete.

### 3. Unchanged

`ObjectAutoArchive` / `ObjectAutoRestore` keep their current meaning (files). No change to how the
client handles them.

## Client flow

1. User archives / deletes an object, or removes a link (normal call, `skipCascade=false`).
2. Backend auto-archives direct-child files (`ObjectAutoArchive`, as today) **and** sends
   `OrphansDetected` with the candidate object ids.
3. Client receives `OrphansDetected` → opens the **confirmation popup** listing those objects.
4. User ticks the ones to archive and confirms (or cancels — nothing happens to unticked objects).
5. Client archives the chosen ids with **`ObjectListSetIsArchived(selectedIds, isArchived=true, skipCascade=true)`**.
   The `skipCascade=true` is essential — it prevents the re-prompt loop.
6. Anything the user left unticked stays active. Because the *entire* transitive subtree was already
   listed in step 2, there is no second-level prompt — one popup covers the whole impact.

## UX recommendation — reuse the Bin's tree view

The Bin already displays archived objects as an **expandable tree nested by `createdInContext`** —
which is exactly the relation that defines "orphans created inside" the acted-on object. So the
candidate set in `OrphansDetected` forms the **same tree shape** the Bin already knows how to render.

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
2. **Handle the new event**: add `OrphansDetected` to the event mapper + dispatcher; on receipt, open
   the confirmation popup with `{ objectIds, contextId, trigger }`.
3. **Build the popup**: reuse the Bin tree view (per above) with checkboxes + confirm.
4. **Thread `skipCascade`** through the archive command wrapper and call it with `true` from the
   popup's confirm handler.
5. Files keep their existing `ObjectAutoArchive` toast/handling — no change.

## Open questions for the client team

- **Delete is broadcast to all sessions** (no session context at delete time) → the confirmation
  popup would appear on every device of the user, not just the initiating one. Confirm acceptable, or
  decide on session-targeted delivery.
- **Batch archive**: archiving multiple objects where one is an ancestor of another can produce the
  same candidate id under two `contextId`s (two `OrphansDetected` messages). The client should
  **dedup** ids across messages when populating the popup.
- **Event/flag naming** (`OrphansDetected`, `skipCascade`) is still provisional on the backend — flag
  if you'd prefer different names before this merges.
```
