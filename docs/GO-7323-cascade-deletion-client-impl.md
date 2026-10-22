# GO-7323 — Cascade Deletion → user-confirmed object archival (desktop client brief)

**Audience:** anytype-ts (desktop) implementer.
**Type:** high-level idea + protocol contract. Code-level investigation (component reuse, exact wiring points) is for the implementing agent.

## Background

When a user creates an object **inside** another object, the backend records the parent in the
`createdInContext` relation. Previously, archiving/deleting that parent (or removing the link to
a created object) **implicitly archived** the whole nested subtree — objects and files alike.
User feedback: silently archiving nested *objects* is surprising.

> **Status:** the backend is merged to `develop` (PR #3201). Regenerate protos from `develop`.
> Nothing has been *released* to users, so the protocol can still be changed cheaply — raise
> objections now.

New behavior:

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
  repeated string objectIds = 1;  // every orphan created within contextId: objects and files, any depth
  string contextId = 2;           // the object that was archived / deleted / had a link removed
  Trigger trigger = 3;
  enum Trigger { archive = 0; delete = 1; linkRemoval = 2; }
}
```

- `objectIds` — the candidates to offer the user. Mixes objects and files at any depth; the client
  resolves each id's layout itself for display.
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

Semantics: when `true`, the backend performs a **pure archive** of exactly the given ids and skips
orphan detection entirely — no `CleanupSuggestion` event. The client uses this on the **confirmation
call** so that archiving the user-chosen objects does not trigger another round of detection, which
would re-open the popup in an infinite loop. Default `false` runs detection and emits the event.

### 3. Retired — `ObjectAutoArchive` / `ObjectAutoRestore`

Still defined in the proto, never emitted. The backend performs no automatic archival or restoration,
so there is nothing for them to announce. They are kept rather than removed because the decision is
gated behind a single backend constant (`objectgc.autoArchiveOrphanFiles`) and may be revisited after
user feedback; removing the messages would be a breaking protocol change for a reversible experiment.

Clients should stop relying on them. See the note in the behavior matrix above.

### 4. New RPCs — the on-demand cleanup list

The event above is the *reactive* half. These two are the on-demand half, in
`pb/protos/commands.proto` and registered in `service.proto`:

```proto
message CleanupSuggestions {
  message Request {
    string spaceId = 1;         // required; empty => BAD_INPUT
    repeated string keys = 2;   // relation keys to project; empty => a default set
  }
  message Response {
    Error error = 1;
    repeated Item items = 2;

    message Item {
      google.protobuf.Struct details = 1;  // only the requested keys
      bool isRoot = 2;                     // createdInContext parent is outside the orphan set
      Reason reason = 3;                   // set on roots only; `none` for descendants
      enum Reason { none = 0; contextArchived = 1; contextDeleted = 2; contextUnlinked = 3; }
    }
    message Error {
      Code code = 1; string description = 2;
      enum Code { NULL = 0; UNKNOWN_ERROR = 1; BAD_INPUT = 2; }
    }
  }
}

message CleanupSuggestionIgnore {
  message Request {
    repeated string objectIds = 1;
    bool ignored = 2;
  }
  message Response {
    Error error = 1;
    message Error {
      Code code = 1; string description = 2;
      enum Code { NULL = 0; UNKNOWN_ERROR = 1; BAD_INPUT = 2; }
    }
  }
}
```

**`ObjectCleanupSuggestions`** returns every orphan in the space as a **forest**, sorted by id.
Reconstruct the tree by joining each item's `details.createdInContext` to its parent's `details.id`;
items with `isRoot = true` are the roots. `reason` explains *why* a root is orphaned and is only
meaningful on roots — descendants inherit their root's reason and carry `none`.

**Chat content is never suggested.** Files and objects created in a live chat carry
`createdInContext = <chat object>` but can never acquire a backlink (chat messages live in anystore,
not in block state), so they would otherwise look exactly like unlinked orphans. Objects whose
creation context is a live chat — or any other non-GC-eligible layout — are excluded. Once the chat
itself is deleted its attachments *are* genuinely orphaned, and they appear with
`reason = contextDeleted`.

`keys` selects which relations come back in `details`. Pass what you need to render (e.g. `name`,
`iconEmoji`, `iconImage`, `snippet`). `id`, `createdInContext`, and `resolvedLayout` are **always**
included regardless — you cannot render the forest without them. Passing no keys yields the default
set: `name`, `type`, `creator`, `createdDate`, `snippet`, `iconEmoji`, `iconImage` (plus the three
forced keys). Passing any key replaces that default entirely — the forced three are still added.

**`ObjectCleanupSuggestionIgnore`** permanently excludes objects from cleanup suggestions — both this
list and the `CleanupSuggestion` event — by setting the hidden `createdInContextIgnored` relation.
Ignoring an object also drops its descendants from the list, because the ignored object stays alive
and still references them. Reversible: pass `ignored = false`.

The write syncs across devices but deliberately does **not** bump `lastModifiedDate`, so ignoring an
object will not reorder it in "recently modified" views.

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

1. **Regenerate proto bindings** from branch `go-7323-cascade-deletion-orphan-events` (events +
   commands + service) — picks up the new event field, `skipCascade`, and the two new RPCs.
2. **Handle the new event**: add `CleanupSuggestion` to the event mapper + dispatcher; on receipt,
   surface `{ objectIds, contextId, trigger }` per the blocking-vs-informative decision.
3. **Build the popup**: reuse the Bin tree view (per above) with checkboxes + confirm.
4. **Thread `skipCascade`** through the archive command wrapper and call it with `true` from the
   popup's confirm handler.
5. **Retire the `ObjectAutoArchive` / `ObjectAutoRestore` handlers.** They will never fire again. Any
   "files moved to Bin" toast should be dropped, or re-driven from the user's own confirmation in
   step 4 — the files now appear in the `CleanupSuggestion` candidate list like any other orphan.
6. **Wire `ObjectCleanupSuggestions`** for the on-demand list (space settings / a "clean up" entry
   point), rendering the forest via `isRoot` + `createdInContext`, and grouping roots by `reason`.
7. **Wire `ObjectCleanupSuggestionIgnore`** as a per-item "don't suggest this again" action, and
   remember it is reversible so a settings screen can un-ignore.

## Open questions for the client team

- **Delete is broadcast to all sessions** (no session context at delete time) → the confirmation
  popup would appear on every device of the user, not just the initiating one. Confirm acceptable, or
  decide on session-targeted delivery.
- **Batch archive**: archiving multiple objects where one is an ancestor of another can produce the
  same candidate id under two `contextId`s (two `CleanupSuggestion` messages). The client should
  **dedup** ids across messages when populating the popup.
- **Blocking vs informative popup** is the client's call, and the backend does not care. A blocking
  popup on every archive may be intrusive; a passive notice plus the on-demand list may let orphans
  pile up. Worth deciding before the first release rather than after.
- **Naming is settled but unshipped.** `CleanupSuggestion` / `skipCascade` / `ObjectCleanupSuggestions`
  / `ObjectCleanupSuggestionIgnore` are final on the branch, and nothing has been released, so a
  rename is still cheap. Say so now if you'd prefer different names.
