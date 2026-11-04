> Ready-to-paste PR description for branch `GO-3132-expose-discussion-id` → `develop`.
> Follows the org-wide template in anyproto/.github (.github/pull_request_template.md).

- [x] I understand that contributing to this repository will require me to agree with the [Contributor License Agreement](https://github.com/anyproto/.github/blob/main/docs/CLA.md)

## Description

Objects with an inline discussion (the chat rendered at the bottom of an object page) store the discussion's object id in their `discussionId` detail, set in `ObjectAddDiscussion` (`core/block/service.go`). The Local HTTP API never surfaced it, so discussions were unreachable through the API:

- discussions are derived store objects and are not indexed, so they are invisible to search;
- `GET /v1/spaces/{space_id}/chats` only returns `chat_derived` space-level chats;
- calling the chat message endpoints with the parent object id fails with `500 failed to get chat messages`.

The chat endpoints themselves already work for discussions — the API service passes `chatId` straight through to `ChatGetMessages`/`ChatAddMessage` as `ChatObjectId`, and discussions are chat store objects (the desktop client reads discussion messages through the same RPCs). The only missing piece was the id.

This PR adds `discussion_id` to the API `Object` and `ObjectWithBody` models, populated directly from object details in `getObjectFromStruct` and `getObjectWithBlocksFromStruct` (bypassing the property cache, which does not include the hidden `discussionId` relation). With it, `GET /v1/spaces/{space_id}/chats/{discussion_id}/messages` and the other chat endpoints work on discussions with no further changes.

**v2 note:** no change needed — the v2 API has no fixed Object schema; its AnyBlock documents and `?fields=` lists already serve detail-sourced hidden relations such as `discussionId` (the codec only strips `derived`/`local`-sourced keys).

**Follow-up commit — expose chat message blocks:** while verifying discussions end to end we hit the second half of the gap: desktop clients compose messages from blocks (`text`, `link`, `embed`, `editor_quote`, `message_quote`) and leave the flat message part empty, so `ChatGetMessages` returned empty text for every message written from a client. The second commit maps the message block oneof to a `blocks` array on the API `ChatMessage` model — the same representation the store and push notifications already use — so discussion messages written in any client are readable through the API:

```json
"blocks": [
  { "type": "text", "text": "hello bot!", "style": "paragraph" },
  { "type": "message_quote", "message_id": "msg-0", "participant_id": "…", "text": "quoted" }
]
```

The flat `content` part is untouched and still carries API-originated messages; without the blocks mapping, the chat endpoints on discussions read back empty for human-written messages.

**Type of PR** (check all applicable):

- [x] 🚀 Feature
- [ ] 🐛 Bug Fix 🕷️
- [ ] 📃 Documentation Update 📖
- [ ] 🎨 Style 🎨
- [ ] 🧠 Code Refactor 🤯
- [ ] 🚀 Performance Improvements 🚀
- [x] ✅ Test 🧪
- [ ] 📦 Build 📦
- [ ] 🖥️ CI 🖥️

## Related Tickets & Documents

- Closes the API-side gap reported in #3132 (the shipped chat endpoints cover space-level `chat_derived` chats only; object discussions remained inaccessible)
- External demand: anyproto/anytype-mcp#115

## Mobile & Desktop Screenshots/Recordings

N/A — API-only change, no visual changes. Example response field added:

```json
{
  "object": "object",
  "id": "bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ",
  "name": "My object",
  "discussion_id": "bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"
}
```

## Added tests?

- [x] 👍 yes
- [ ] 🙅 no, because they aren't needed
- [ ] 🙋 no, because I need help

Extended `TestObjectService_ListObjects` / `TestObjectService_GetObject` with a `discussionId` detail; added `TestObjectService_DiscussionIdSerialization` covering presence in JSON and `omitempty` behavior when absent. Added `TestChatMessageFromProto_*` covering block-composed messages (`blocks` mapping incl. message quotes and links), flat API-originated messages, and the nil case.

## Added to documentation?

- [ ] 📜 README.md
- [ ] 📜 [tech-docs](https://github.com/anyproto/tech-docs)
- [x] 🙅 no documentation needed

OpenAPI specs (`core/api/docs/v1/openapi.{json,yaml}`) are regenerated in this PR; the field carries its own schema description.

## Post-deployment tasks

- [ ] 📝 Yes — re-verify end to end against a released build with anytype-mcp: `GET /objects/{id}` returns `discussion_id` → `GET /chats/{discussion_id}/messages` returns the thread → `POST /chats/{discussion_id}/messages` shows up live in the desktop client's Discussion section.
