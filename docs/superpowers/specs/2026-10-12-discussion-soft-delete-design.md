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
| `Delete`, live message | Hard delete, as today | Permission check as today (author, or Owner/Admin at the change's ACL head); rewrite to the tombstone with `coll.UpdateId` on the transaction context (a write failure is wrapped in `ErrCritical`, like `applyDelete` wraps `DeleteId` failures, so the apply aborts and the replay watermark does not move past the change); `subscription.SoftDelete` (section 3); queue the full-text delete (a queue failure is logged, as today); return `DeleteModeKeep` |
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

The guarantee is about the logical message (`deleted`, content, reactions, pinned, creator,
reply target, position). Stored metadata already differs per device (`stateId`, `synced`, the
read flags), and the `_o` sub-orders a tombstone keeps can differ by apply order: an edit
applied before the delete leaves its `_o.content`, one applied after is dropped. Nothing reads
`_o` sub-orders of a tombstone (the full-text iterator skips tombstones), so tests compare the
logical message, not raw documents.

The causal premise holds for changes this heart produces, because the RPC guards (section 4)
never push a modify or delete for a message that is not stored locally. A change from an older
writer that names a message its author never had (possible today through API v1, which pushes
a delete for any id) is not causally after the create; it is a no-op on a device that applies
it before the create and effective on one that applies it after. That divergence exists today
with hard delete and is not addressed here.

Replies: a reply that was written concurrently with the delete always applies (refusing a
create at apply time would diverge). It counts as a mention of the deleted comment's author,
because `IsCurrentUserMentioned` finds the tombstone's `creator`, whatever the order of the
reply and the delete (with hard delete it depended on the order). New local replies to a
tombstone are refused by the RPC (section 4).

This holds for replies that causally follow their parent's create and are applied in a later
transaction than the parent (see "Accepted limitations" for replies replayed together with
their parent). A reply whose author never had the parent (possible through the API, which
does not check that `reply_to` exists) can be applied before the parent on one device and
after it on another, and is a mention on only the second; that exists today and is not
addressed here.

### 3. Counters, events, last-message date

**Unread counters.**

- The tombstone's `read=true`, `mentionRead=true`, `hasMention=false` keep it out of
  `LoadChatState`, `GetOldestOrderId`, `GetUnreadMessageIdsInRange` and `GetAllUnreadMessages`
  with no query change.
- `readMessagesModifier` skips `deleted` documents when setting `read=false`, so
  `ChatUnreadMessages` (mark-unread) cannot resurrect one. `readMentionsModifier` already
  skips `hasMention=false`.
- The tombstone path adjusts the in-memory state at once, from the document it replaces:
  `Messages.Counter--` if it was unread, `Mentions.Counter--` if it was an unread mention,
  `messageCount--`. Without this, a read callback that flushes before the post-commit reload
  (seen heads that arrived before the changes, `markReadMessages` → `Flush(false)`) would
  publish a count still inflated by the deleted message.
- It also sets `needReloadState`, as `Delete` does today. After the commit `Flush(true)`
  reloads `ChatState` (both counters, oldest order ids, `lastStateId`,
  `unreadReactionOrderId`) and the message count from the repository, which corrects the
  oldest-unread order ids and anything the deltas missed; the parent's unread counter worker
  then runs as usual. Deleting an unread comment on one device clears the badge on the others
  once the `Delete` op syncs, because each builds the tombstone itself.

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

One exception, from event coalescing in subscription windows: a message created and deleted
before the window flushes (both in one apply batch) reaches the window as a single
`Event.Chat.Add` carrying the tombstone, because an entry's Add takes priority over its Update
(`state.go` events buffer). A client that never saw the message still learns of it, already
deleted. Observers are called per change and do not coalesce: they get Added (live message)
then Updated (tombstone).

Window eviction must not lose the tombstone. When a window is full, an add of a newer message
evicts the oldest entry (`messagesState.applyAddMessage` → `RemoveFront`), and its pending
events are dropped with it. Today a hard delete survives this because deletes are buffered
separately (`deleteIds`), but a tombstone is an Update on the entry: one batch with "delete A,
create C" in a full window would evict A before the flush, and the client would keep A's
original content. Eviction therefore moves an evicted entry's pending events other than Add
into `outOfWindowEvents`, which `appendEventsTo` already flushes (an unflushed Add is dropped as
today: the client never had the message and it is now outside its window). This also stops the
same loss for pending edits, reactions and pins, which exists today.

Space chats keep `Event.Chat.Delete`. `GetPrevOrderId` keeps seeing tombstones, which is
right because every window holds them (section 5): add events position new messages relative
to entries clients hold.

**Last-message date.** `subscriptionManager.GetLastMessage` (feeds `lastMessageDate`) skips
tombstones: it walks the window back from its end, and its repository fallback reads only
live messages. Deleting the newest comment falls back to the previous one, as today. If every
comment is deleted, `lastMessageDate` keeps its old value, as today. Message previews
(`SubscribeToMessagePreviews`) cover only `chatDerived` chats; push notifications are sent
only from a local `AddMessage`. Neither changes.

### 4. RPC guards

Local guards for new operations, in `storeObject`, under the object lock, before anything is
pushed. New exported sentinels in `chatobject`, alongside `ErrModifyForeignMessage`:
`ErrMessageDeleted = errors.New("message is deleted")`,
`ErrReplyToDeletedMessage = errors.New("can't reply to a deleted message")` and
`ErrMessageNotFound = errors.New("message not found")`. The RPC layer maps all three to
`BAD_INPUT`.

The guards apply to every chat, not only discussions, because the target must exist locally
before a change naming it is pushed. With update-only modifies, an edit, reaction or pin for a
message that is not stored would otherwise push a change that applies as a no-op and report
success; and a change naming a message its author never had is not causally after the
message's create (section 2).

| RPC | Target missing locally | Target is a tombstone |
|---|---|---|
| `ChatEditMessageContent` | `ErrMessageNotFound` | `ErrMessageDeleted` |
| `ChatToggleMessageReaction` | `ErrMessageNotFound` | `ErrMessageDeleted` |
| `ChatPinMessages` (`SetMessagePinned`) | `ErrMessageNotFound` | `ErrMessageDeleted` |
| `ChatDeleteMessage` | Success, nothing pushed (today a change is pushed) | Success, nothing pushed (Android and desktop offer a second delete) |
| `ChatAddMessage` with `replyToMessageId` | No check, as today | `ErrReplyToDeletedMessage` |

- `ChatPinMessages` takes several ids and today pushes one change per id. It checks every
  target first and pushes nothing if any is refused, so a refusal never follows a partial pin.
- The reply check runs before `PushStoreChange`, so a refused reply sends no push notification
  and touches no attachment's `createdInContext`.
- `ChatGetMessages`, `ChatGetMessagesByIds` and `ChatSubscribeLastMessages` return tombstones in
  place, `deleted: true`.
- `ChatDeleteMessage`'s file cleanup is unchanged: it reads the message's attachments and link
  targets before deleting and runs the object GC on them (`core/block/chats/service.go`
  `DeleteMessage`), so a discussion comment's own files are removed as today while its row
  stays. A repeat delete reads an empty tombstone and cleans nothing.

The guards are a backstop for clients and the API; the apply-time rules in section 2 are what
make remote changes safe.

### 5. Full-text index and search

- The tombstone path queues `AddChatMessageDeleteToIndexQueue`, as `BeforeDelete` does today.
- `IterateMessagesForIndexing` skips `deleted` documents, which covers a full backfill and the
  re-index `BeforeModify` queues for a modify the guard then drops. The indexer applies deletes
  after the streamed upserts of a batch, so a tombstone cannot come back.
- `ChatSearch`: hydrating full-text hits drops tombstones (as a stale hit for a hard-deleted
  message is dropped today). Browse mode (no query) reads only live messages; the creator
  index must still serve the creator query (pinned by the repository's index tests).

Which reads include tombstones is a per-caller decision, not a property of the repository:

| Read | Tombstones | Callers |
|---|---|---|
| `GetLastMessages` | Included (unchanged) | subscription windows (`chatsubscription/service.go`, hence the per-chat SSE stream via `chatsubadapter.go`), the space-stream backfill (`spacechathub.go`) |
| `GetMessages`, `GetMessagesByIds`, `GetPrevOrderId` | Included (unchanged) | client and API reads, add-event positioning |
| New live-only latest read (`GetLastLiveMessages`) | Excluded | `ChatSearch` browse without creators, `subscriptionManager.GetLastMessage` fallback |
| `GetLastMessagesByCreators` | Excluded (its only caller is `ChatSearch` browse) | `ChatSearch` browse with creators |

A window or backfill that dropped tombstones would lose the thread anchor of replies to a
deleted comment, and a later add would name an `afterOrderId` the client never received.

### 6. API

**v2.**

- `v2model.ChatMessage` gains `Deleted bool \`json:"deleted,omitempty"\``, present only on
  tombstones. The exact served tombstone is `id`, `state_id`, `order`, `author_id`, `at`,
  `author` and `reply_to` under their existing omission rules (`author` only when the
  participant name resolves), `"text": ""` (the member is always serialized, as today) and
  `"deleted": true`; no `edited_at`, `blocks_text`, `attachments`, `reactions`, `reacted_by`
  or `pinned`.
- `get_chat_messages` serves tombstones in place. Both descriptions of the count change:
  `ChatMessagesResponse.MessageCount`'s comment ("a deleted one is gone from it") and the
  `get_chat_messages` handler's description of `message_count` (`core/api/v2/handler/chat.go`)
  now say a deleted space-chat message is gone, while a deleted discussion comment stays in
  `messages` with `deleted: true` and is not counted. `lifetime_message_count` is unchanged.
  The full wrapper's golden tool list (`core/api/wrapper/full/testdata/tools_list.golden.json`)
  carries that description and is regenerated with it.
- `edit_chat_message`, `toggle_chat_reaction`, `delete_chat_message` on a tombstone: **404**
  "message … was deleted", from the existing existence check `getChatMessageProto`, before
  the RPC and on dry runs. Delete keeps today's 404 for an already-deleted message.
- `add_chat_message` whose `reply_to` is a tombstone: **400**, an issue on `reply_to`,
  checked before the dry run. A `reply_to` that does not exist keeps today's behavior.
- A delete can land between those prechecks and the RPC. `v2ChatRpcError` therefore matches
  the new sentinels by description BEFORE its generic `BAD_INPUT` → 400 branch, the way it
  already matches the foreign-message sentinels: `ErrMessageDeleted` and `ErrMessageNotFound`
  → 404, `ErrReplyToDeletedMessage` → 400 with the issue on `reply_to`. A delete that races
  another delete succeeds (the RPC is idempotent).
- `delete_chat_message`'s dry-run warnings about attachments and link targets are unchanged.
- All these statuses are already declared on those ops; the response inventory
  (`openapi_responses_test.go`, 377 pairs) does not change. `make openapi` regenerates the
  document; the new and changed descriptions fit the served-prose budget tests.
- Both SSE streams report a deletion of a message they already announced as
  `message_updated` with `deleted: true`. They differ when the create and the delete arrive in
  one batch (section 3): the per-chat stream, fed by a subscription window, emits one
  `message_added` carrying the tombstone; the space stream, fed by observers, emits
  `message_added` with the live message, then `message_updated` with the tombstone. Opening a
  stream (window snapshot, space-stream backfill) announces existing tombstones as
  `message_added` with `deleted: true`.

**v1.** `core/api/model.ChatMessage` gains `deleted` (`omitempty`), set by its converter and
its event conversion. Add and edit already map `BAD_INPUT` to 400; the reaction path collapses
every RPC error into `ErrFailedToggleReaction` (HTTP 500), so it gains the same `BAD_INPUT` →
400 mapping. A v1 delete of a missing or deleted message still returns 200 (now without
pushing a change). Nothing else in v1 changes.

## Compatibility

Checked 2026-10-12 against desktop (`anytype-ts` origin/develop d965ea2), iOS
(`anytype-swift` origin/develop 301f619677) and Android (`anytype-kotlin` origin/main).

- **The new deletion path sends old apps nothing new.** Their bundled heart applies the same
  `Delete` op as a hard delete, exactly as today (including today's hidden subtrees). One
  existing exception stays: an old heart applies modifies as upserts, so a reaction or pin
  made while the comment was live and received after its delete re-creates a contentless
  stub there, as it does today. The update-only fix protects new hearts only.
- **Downgrade on the same profile is not covered** (decided 2026-10-12: document, do not bump
  `ForceReindexDiscussionsCounter`). The guarantee above holds for devices whose stores were
  built by the heart that reads them. If a profile a new heart already wrote tombstones into is
  reopened by an older heart, the tombstones stay (the `Delete` ops are behind the replay
  watermark and are not re-applied) and the older heart shows them as comments with author and
  date and no body, until something rebuilds that discussion's store.
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

## Accepted limitations

All three exist today; this change inherits them and does not fix them.

- **Side effects before commit.** Handler hooks run before the apply transaction commits:
  subscription windows and observers see the change, and the full-text queue (another
  database) is written. If the transaction then rolls back (for example, any-sync fails to
  persist the change after it was applied to the store), a window keeps showing the tombstone
  and the message is missing from search although the store still has it live. Reopening the
  chat rebuilds the window; a later edit or a reindex restores search. Commit-scoped
  subscription events would be their own project.
- **A create skipped on a full-text queue failure** (found during review, file separately):
  `BeforeCreate` returns a plain error when `AddChatMessageToIndexQueue` fails
  (`chathandler.go:145`), and `applyChangeSet` logs and skips non-critical errors, so the
  message is not stored on that device while the replay watermark moves past it. A later
  modify or delete of that message finds nothing. The tombstone path does not repeat this: its
  queue failure is only logged.
- **Reply mentions lost when a reply is applied together with its parent** (found during
  review, file separately; decided 2026-10-12 to keep it out of this change).
  `IsCurrentUserMentioned` looks the parent up through `repository.GetMessagesByIds`, which
  opens its own read transaction (any-store `db.ReadTx` takes a separate read connection) and
  cannot see documents written earlier in the same apply transaction. A full replay (a new
  device, a discussion reindex) or a sync batch carrying both therefore stores
  `hasMention=false` on a reply to the user's message, while a device that had the parent
  committed earlier marks it. The reply still counts as an unread message. The fix is a getter
  that reads through the apply transaction's context (`coll.FindId(ctx, id)`).

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
  - a concurrent reply to a deleted comment still counts as a mention of its author, both when
    the reply is applied before the delete and after it (parent committed earlier);
  - a tombstone write failure aborts the apply (`ErrCritical`): the store keeps the live
    message and the replay watermark does not advance.
- Counters:
  - deleting an unread message, an unread mention, and an unread reply-to-me from another
    author clears the counters; mark-unread skips tombstones; `messageCount` excludes them;
    unread reactions on a deleted comment are cleared from the state;
  - seen heads that arrive before the changes, then one batch with create A, create B, delete
    A: no flushed state ever reports A as unread or counted.
- Repository: `CountMessages`, `IterateMessagesForIndexing` and the live-only reads exclude
  tombstones, `GetLastMessages` includes them, with index-usage checks.
- Subscription:
  - a discussion delete emits `Event.Chat.Update` (not `Delete`) and observers get
    `ChatChangeMessageUpdated`; out-of-window too;
  - create and delete in one batch emit one Add carrying the tombstone in a window, and Added
    then Updated to observers;
  - in a full window, "delete A, create C" in one batch still delivers A's tombstone (through
    `outOfWindowEvents`), and so does a pending edit of A;
  - reopening a subscription after deleting post P keeps P (tombstone) and its reply R; an
    all-tombstone window; an add after a newest tombstone names it as `afterOrderId`;
  - `GetLastMessage` skips tombstones in the window and in the fallback.
- RPC guards: edit, reaction, pin and reply on a tombstone refused; edit, reaction and pin on a
  missing message refused; delete of a missing or deleted message pushes nothing; a pin batch
  with a tombstone first, middle or last pushes nothing; a refused reply sends no push.
- Indexer: a tombstone is not re-added by a backfill.
- `ChatSearch` (service level): a stale full-text hit whose message is now a tombstone is
  dropped with its highlight; browse without creators and browse with creators skip tombstones
  and return older live messages when the newest rows are tombstones.
- API v2: the exact served tombstone (REST and both SSE streams, author resolved and not), and
  the two streams' event sequences for a create and delete in one batch; 404 on edit, reaction and
  delete of a tombstone, including dry runs and the race through `v2ChatRpcError`; 400 with the
  `reply_to` issue, including the race; delete warnings unchanged.
- API v1: the `deleted` field; reaction on a tombstone → 400.
