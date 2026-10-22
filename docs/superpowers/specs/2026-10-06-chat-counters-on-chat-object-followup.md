# Follow-up: unread counters on the space chat object, and `is_main` in v2

Status: approved to build, 2026-10-06 (ruling: every chat without a parent writes to itself; discussions keep writing to their parent). Comes after the space chat stream and the search
stream (`2026-10-06-apiv2-space-chat-stream-design.md`, `2026-10-06-apiv2-object-stream.md`).

## 1. Why

Today only discussions store unread counters as object details, and they store them on the
discussion's **parent** (`chatobject.go` `writeUnreadCountersToParent`, from GO-7168, 22 April
2026). `triggerParentUnreadUpdate` returns early unless the chat's type key is `discussion`, so the
space chat (`chatDerived`, the "General" chat) has no stored counters; clients read its state
through the chat RPC and message previews. Checked in a real store: the General chat has `name`,
`lastMessageDate`, `isMainChat` and no `unreadMessageCount` / `unreadMentionCount`; the workspace
object and the space view carry only `chatId`.

Consequences of the gap:
- `unread_mention_count > 0` in a search or the search stream finds discussion parents but never a
  space chat.
- The space chat's counters can only be read through the chat state manager.

## 2. Decisions taken (conversation 2026-10-06)

- The space chat writes the two local details **onto the chat object itself**, not onto its parent
  (the workspace object).
- Discussions keep writing to their parent; a discussion does not also write to itself.
- Ruling (the owner did not answer): the space chat AND every other `chatDerived` chat write to
  themselves.

## 3. Change

1. `chatobject`: `triggerParentUnreadUpdate` / `writeUnreadCountersToParent` handle the space chat
   case: when the chat is the space chat (`s.Id() == Space().DerivedIDs().SpaceChat`), write
   `unreadMessageCount` and `unreadMentionCount` as local details on the chat object (own state, no
   parent lookup). Reuse the coalescing worker, so writes stay serialized and converge to the
   freshest values. Other `chatDerived` objects: decide in the plan whether they also write to
   themselves (probably yes, same mechanism: any chat without a parent writes to itself).
2. Tests, including the GO-7378 regression class: counters must not inflate after a reindex or an
   object reopen; the worker must not start for a chat whose object is deleted; a space chat's own
   write must not push a change to other members (local details only).
3. v2: add `is_main` (boolean, always present, C2) to the chat row, filled from the `isMainChat`
   detail in `chatRowOf`; it appears on `list_chats` rows and on the stream's `chat_added`. A
   missing detail serves `false`.
4. Search/stream vocabulary: no change needed once the counters are details (the
   `v2BundledQueryKeys` allowlist already accepts them), but the spec text that says the counters
   are "discussion-parent counters" becomes "chat or discussion-parent counters".
5. Docs: `APIV2.md`, `v2/SKILL.md`, the Q3 note in `APIV2_SURFACES.md`.

## 4. Main chat identification (reference)

- Property: `isMainChat` (checkbox, hidden, read-only, derived). Set to `true` in chatobject init
  when the chat id equals the space's derived space-chat id; the same block forces the name to
  "General". Absent on every other chat.
- Cross-check: the workspace object and the space view carry `chatId` = the main chat's id.
- Not exposed in v2 today; item 3 above exposes it.

## 5. Risks

- Counter writes on the space chat object add local detail changes on a hot object. Coalescing keeps
  them at most one write in flight.
- Local details only: they do not sync, so each device holds its own account's counters, as for
  discussion parents.
