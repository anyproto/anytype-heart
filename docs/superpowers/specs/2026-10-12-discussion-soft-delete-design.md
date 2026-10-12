# Discussion comments: soft delete

Ticket: GO-7167. Packages: `core/block/editor/chatobject`, `core/block/editor/storestate`,
`core/block/chats/...`, `core/indexer`, `core/api` (v1 + v2), `pkg/lib/pb/model/protos`.

## Problem

Deleting a discussion comment pushes a store `Delete` op. `ChatHandler.BeforeDelete`
(`core/block/editor/chatobject/chathandler.go:190`) removes the document from the
`{chatId}chats` collection and the client receives `Event.Chat.Delete` with only the id.

- There is nothing left to render the designed "Comment deleted" placeholder from.
- Every descendant of the deleted comment disappears. Discussions *display* one level
  (post + replies), but `replyToMessageId` may point at any message in the thread: Android
  replies to the tapped message, API agents reply to the message they answer. iOS
  (`DiscussionThreadGrouper.swift:24-63`) and Android (`DiscussionViewModel.kt:383-419`)
  walk the reply chain to its root and silently drop a reply whose chain hits a missing id;
  desktop (`section.tsx:575-607`) groups by direct parent only and drops replies of a missing
  post. A hard delete therefore hides the whole subtree on every client, while the replies
  stay in storage and keep syncing.

Related existing bug: chat modify ops are applied as upserts (`chathandler.go:187`,
`storestate/state.go:286`). any-store `UpsertId` inserts `{id}` for a missing document, so a
reaction or pin applied after a hard delete re-creates a stub message with no creator, no
`read` flag (counted as unread) and no `_o.id`. Content edits are refused on a stub only by
accident (the creator check fails against the empty creator).

## Decisions

Settled with the user on 2026-10-12:

1. **Wire format: a locally materialized tombstone, nothing new on the wire.** The delete
   still travels as the existing store `Delete` op. A discussion's handler turns it into a
   tombstone instead of removing the document. Rejected alternative: pushing explicit
   `deleted`/content-clear modify ops — old hearts reject an unknown key path but apply the
   content clear (old apps would show empty messages), and they refuse the content clear when
   a moderator deletes someone else's comment (the creator check), so moderator deletes would
   never reach old apps.
2. **Scope: discussions only** (`SmartBlockTypeDiscussionObject`, i.e. the store object with
   type key `discussion`). Space chats (`chatDerived`) keep hard delete; they have no thread
   structure that needs the placeholder.
3. **No migration.** Stores where comments were already hard-deleted keep those gaps. A device
   that replays a discussion from scratch (a `reindexDiscussions` wipe, a new device) turns
   every `Delete` op in the tree into a tombstone, including pre-feature ones, so devices may
   disagree about pre-feature deletes. Accepted; where it happens it restores lost threads.
4. **Chat modify ops become update-only** (all chats, not only discussions). Fixes the stub
   bug above.

## Design

### 1. Tombstone shape

The stored document after a discussion `Delete` op is applied:

| | Fields |
|---|---|
| Kept | `id`, `creator`, `createdAt`, `replyToMessageId`, `stateId`, `synced`, `_o` (the whole order object, so the message keeps its position) |
| Set | `deleted: true`; `read: true`; `mentionRead: true`; `hasMention: false`; `modifiedAt: 0` |
| Cleared | `content` = `{message: {text: "", style: 0, marks: []}, attachments: {}, blocks: []}` (an empty object, never absent: `MarshalAnyenc` dereferences `Message`); `reactions` = `{}`; `pinned` removed; `rUnreadChIds` / `rUnreadOrdId` removed |

- `deleted` is written only on tombstones (like `pinned`, only when true) and gets a sparse
  index. Queries use `deleted == true` (`Eq`) only: since any-store v1.0.2 the planner does
  not use a sparse index for `$exists`, and a bare `NotEqual` on a sparse field returned no
  rows. "Not deleted" is expressed as a residual `Not{deleted == true}` filter, never as an
  index candidate.
- `modifiedAt` is zeroed because all three clients label any message with `modifiedAt` set as
  "(edited)" (desktop `post.tsx:39`, iOS `DiscussionMessageBuilder.swift:158`, Android
  `ChatViewModel.kt:622`).
- `read`/`mentionRead`/`hasMention` make the tombstone invisible to every unread query
  without touching those queries (section 3).

Proto: `model.ChatMessage` gains `bool deleted = 18;` with a comment saying only discussions
produce it and what a tombstone keeps. `chatmodel` gains `DeletedKey = "deleted"`; marshal
writes it only when true, unmarshal reads it.

A tombstone can only come from a `Delete` op: `BeforeCreate` forces `Deleted = false`, and
`storeObject.AddMessage` strips the key from the client-supplied message like it strips
`read`/`synced`.

### 2. Storestate and apply rules

`storestate` gains `DeleteModeKeep`: the handler already rewrote the document inside the
apply transaction; `applyDelete` leaves it in place. (The existing `DeleteModeMark` does not
fit: it replaces the document with `{id, _d}` and overwrites `_o`.)

`ChatHandler` learns whether it serves a discussion (set from `storeObject.typeKey` in
`Init`). Rules, identical for local and remote changes:

| Incoming op | Space chat | Discussion |
|---|---|---|
| `Delete`, live message | Hard delete, as today | Permission check as today (author, or Owner/Admin at the change's ACL head); rewrite to the tombstone with `coll.UpdateId` on the transaction context; `subscription.SoftDelete` (section 3); queue the full-text delete; return `DeleteModeKeep` |
| `Delete`, tombstone | n/a | No-op: no permission check, no event, no index work; `DeleteModeKeep` |
| `Delete`, missing document | No-op, as today | No-op, as today |
| `Modify` (content, reactions, pinned) on a tombstone | n/a | Dropped in `UpgradeKeyModifier` (`return v, false, nil`) before the inner modifier runs |
| `Modify` on a missing document | Dropped (`BeforeModify` returns `ModifyModeUpdate`; `applyModify` already ignores `ErrDocNotFound`) | Same |
| `Create` replayed over an existing tombstone | n/a | `ErrIgnore`, as today (`BeforeCreate` finds the doc) |

Why this converges: a message's create is always applied before its own modifies and its
delete (the delete and every modify causally descend from the create, and both full replay
and the incremental `GetAfterAddSeq` walk iterate in `orderId` order). A modify concurrent
with the delete may be applied before or after it: before, the tombstone overwrites its
effect; after, the guard drops it. Delete wins either way, on every device.

Replies: a reply that was written concurrently with the delete always applies (refusing a
create at apply time would diverge). It counts as a mention of the deleted comment's author,
because `IsCurrentUserMentioned` finds the tombstone's `creator` — on every device, whatever
the apply order (with hard delete it depended on the order). New local replies to a tombstone
are refused by the RPC (section 4).

### 3. Counters, events, last-message date

**Unread counters.**

- The tombstone's `read=true`, `mentionRead=true`, `hasMention=false` keep it out of
  `LoadChatState`, `GetOldestOrderId`, `GetUnreadMessageIdsInRange` and `GetAllUnreadMessages`
  with no query change.
- `readMessagesModifier` skips `deleted` documents when setting `read=false`, so
  `ChatUnreadMessages` (mark-unread) cannot resurrect one. `readMentionsModifier` already
  skips `hasMention=false`.
- The tombstone path sets `needReloadState`, as `Delete` does today. After the commit `Flush`
  reloads `ChatState` (both counters, oldest order ids, `lastStateId`,
  `unreadReactionOrderId`) and the message count from the repository; the parent's unread
  counter worker then runs as usual. Deleting an unread comment on one device clears the
  badge on the others once the `Delete` op syncs, because each builds the tombstone itself.

**`messageCount`** (`Event.Chat.UpdateMessageCount`, documented as non-deleted messages):
`CountMessages` = count(all) − count(`deleted == true`), the second through the sparse index.
`CountMessagesLifetime` (the history collection behind API `message_count`) is unchanged.

**Events.** The subscription manager gets `SoftDelete(msg *chatmodel.Message)`:

- applies `applyUpdate` to every window, so clients receive `Event.Chat.Update` carrying the
  full tombstone and the message stays at its position;
- notifies observers with `ChatChangeMessageUpdated` and the tombstone (not
  `ChatChangeMessageDeleted`), so the API space stream (`core/api/spacechathub.go`, which
  covers discussions) and the per-chat stream both report an updated message with
  `deleted: true`, the same shape reads return;
- sets `needReloadState` (state and count events follow after the commit).

Space chats keep `Event.Chat.Delete`. `GetPrevOrderId` keeps seeing tombstones, which is
right: add events position new messages relative to entries clients hold.

**Last-message date.** `subscriptionManager.GetLastMessage` (feeds `lastMessageDate`) skips
tombstones: it walks the window back from its end, and its repository fallback reads only
live messages. Deleting the newest comment falls back to the previous one, as today. If every
comment is deleted, `lastMessageDate` keeps its old value, as today. Message previews
(`SubscribeToMessagePreviews`) cover only `chatDerived` chats; push notifications are sent
only from a local `AddMessage`. Neither changes.

### 4. RPC guards

Local guards for new operations, in `storeObject`, under the object lock. New exported
sentinels in `chatobject`, alongside `ErrModifyForeignMessage`:
`ErrMessageDeleted = errors.New("message is deleted")` and
`ErrReplyToDeletedMessage = errors.New("can't reply to a deleted message")`. The RPC layer
maps both to `BAD_INPUT`.

| RPC on a tombstone | Result |
|---|---|
| `ChatEditMessageContent` | `ErrMessageDeleted` |
| `ChatToggleMessageReaction` | `ErrMessageDeleted` |
| `ChatPinMessages` (`SetMessagePinned`) | `ErrMessageDeleted` |
| `ChatAddMessage` whose `replyToMessageId` is a tombstone | `ErrReplyToDeletedMessage`; a target that does not exist keeps today's behavior (no check) |
| `ChatDeleteMessage` | Success without pushing a change (Android and desktop offer a second delete) |
| `ChatGetMessages`, `ChatGetMessagesByIds`, `ChatSubscribeLastMessages` | Return tombstones in place, `deleted: true` |

The guards are a backstop for clients and the API; the apply-time rules in section 2 are what
make remote changes safe.

### 5. Full-text index and search

- The tombstone path queues `AddChatMessageDeleteToIndexQueue`, as `BeforeDelete` does today.
- `IterateMessagesForIndexing` skips `deleted` documents, which covers a full backfill and the
  re-index `BeforeModify` queues for a modify the guard then drops. The indexer applies deletes
  after the streamed upserts of a batch, so a tombstone cannot come back.
- `ChatSearch`: hydrating full-text hits drops tombstones (as a stale hit for a hard-deleted
  message is dropped today). Browse mode (no query; `GetLastMessages` /
  `GetLastMessagesByCreators`) reads only live messages; the creator index must still serve
  the creator query (pinned by the repository's index tests).

### 6. API

**v2.**

- `v2model.ChatMessage` gains `Deleted bool \`json:"deleted,omitempty"\`` (present only on
  tombstones; a tombstone has no text, blocks, attachments, reactions or `edited_at`).
- `get_chat_messages` serves tombstones in place. `ChatMessagesResponse.MessageCount`'s
  comment changes from "a deleted one is gone from it" to: a deleted space-chat message is
  gone; a deleted discussion comment stays in `messages` with `deleted: true` and is not
  counted. `lifetime_message_count` is unchanged.
- `edit_chat_message`, `toggle_chat_reaction`, `delete_chat_message` on a tombstone: **404**
  "message … was deleted", from the existing existence check `getChatMessageProto`, before
  the RPC and on dry runs. Delete keeps today's 404 for an already-deleted message.
- `add_chat_message` whose `reply_to` is a tombstone: **400**, an issue on `reply_to`,
  checked before the dry run. A `reply_to` that does not exist keeps today's behavior.
- All these statuses are already declared on those ops; the response inventory
  (`openapi_responses_test.go`, 377 pairs) does not change. `make openapi` regenerates the
  document; the `deleted` description fits the served-prose budget.
- Both SSE streams report a deletion as `message_updated` with `deleted: true` (section 3).

**v1.** `core/api/model.ChatMessage` gains `deleted` (`omitempty`), set by its converter and
its event conversion. Mutations on a tombstone hit the heart refusals and v1's usual error
mapping. Nothing else in v1 changes.

## Compatibility

Checked 2026-10-12 against desktop (`anytype-ts` origin/develop d965ea2), iOS
(`anytype-swift` origin/develop 301f619677) and Android (`anytype-kotlin` origin/main).

- **Old apps never see an empty message.** Their bundled heart applies the same `Delete` op as
  a hard delete, exactly as today (including today's hidden subtrees).
- **New heart, UI not yet updated** — the only case a tombstone reaches an unaware UI. All
  three render a discussion tombstone as a comment with avatar, author and date and no body;
  none crash. Replies stay attached under it.
  - All three still offer react and reply on it (desktop also edit); the heart refuses them.
  - Desktop removes your own deleted comment from its store immediately (`post.tsx:439`,
    `reply.tsx:299-307`), so the deleter sees the post and all its replies vanish until a
    reload while everyone else sees the tombstone.
  - Desktop routes `chatUpdate` for comments by `replyToMessageId` and copies only
    `{content, attachments, reactions}` (`dispatcher.ts:1312-1340`); it will not pick up
    `deleted` without a change.

## Out of scope

- Migration of comments hard-deleted before this change (decision 3).
- Client work: placeholder rendering, hiding react/reply/edit on a tombstone, desktop no longer
  removing the deleter's comment locally and following the reply chain to its root
  (iOS IOS-5944; desktop and Android tickets to file).
- The MCP repository regenerating its tool fixture from the new OpenAPI document.
- Space-chat soft delete.

## Testing

Test-first, in the existing `chatobject` fixture, which applies changes to the store as any
author.

- Handler:
  - the tombstone's exact stored shape;
  - author delete and Owner/Admin delete produce it; a writer cannot delete someone else's
    comment;
  - a second delete is a no-op (no events, no FT queue);
  - edit, reaction and pin applied both before and after the delete converge to the tombstone;
  - a full replay of create + delete builds the tombstone;
  - space chats still hard-delete;
  - a reaction applied after a space-chat hard delete creates no document (upsert regression);
  - a concurrent reply to a deleted comment still counts as a mention of its author.
- Counters: deleting an unread message, an unread mention, and an unread reply-to-me from
  another author clears the counters; mark-unread skips tombstones; `messageCount` excludes
  them; unread reactions on a deleted comment are cleared from the state.
- Repository: `CountMessages`, `IterateMessagesForIndexing` and the live-message reads exclude
  tombstones, with index-usage checks.
- Subscription: a discussion delete emits `Event.Chat.Update` (not `Delete`) and observers get
  `ChatChangeMessageUpdated`; `GetLastMessage` skips tombstones in the window and in the
  fallback.
- RPC guards: edit, reaction, pin and reply refused; repeat delete pushes nothing.
- Indexer: a tombstone is not re-added by a backfill.
- API v2: the read shape; 404 on edit/reaction/delete of a tombstone; 400 on `reply_to` a
  tombstone, including dry runs.
