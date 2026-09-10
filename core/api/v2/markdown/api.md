Anytype API v2 exposes an object as one editable **AnyBlock JSON document**.
A single `GET` reads its properties and content; a single `PATCH` applies a
batch of edits. It is designed for agents, scripts, and integrations that
need to work with whole documents.

> **Pre-release:** API v2 can change without a new API version. Use
> [API v1](https://developers.anytype.io/docs/reference) for production
> integrations that need a stable contract.

## Connect and authenticate

The API runs locally with Anytype, at `http://127.0.0.1:31009` by default.
Both API majors share the same process, port, and API keys. v2 routes start
with `/v2/` and do not require the `Anytype-Version` request header.

To obtain a key through Desktop pairing:

1. Send `POST /v2/auth/challenges` with an `app_name`.
2. The user approves spaces and permissions in Anytype Desktop, then receives
   a four-digit code.
3. Send `POST /v2/auth/api_keys` with the `challenge_id` and `code`. The
   response contains an `api_key` and the approved grant.

These two pairing endpoints require no existing key. For authenticated
requests, send the key in the `Authorization` header:

```bash
curl http://127.0.0.1:31009/v2/auth/whoami \
  -H "Authorization: Bearer $api_key"
```

Set `api_key` to your key before running the example. The `whoami` response
describes which spaces and permissions it grants.

The bearer credential is an opaque Anytype API key. It is not a JWT, and it
is never read from a query or body parameter. An unknown, revoked, or expired
key returns `401`.

## Objects, properties, and blocks

- **Spaces** contain objects. Most routes are scoped to one space.
- **Objects** combine typed `properties` with document content in `blocks`.
- **Types** describe an object's shape and are addressed by a type key, such
  as `page` or `task`.
- **Queries** show a live selection of objects. **Collections** hold a
  hand-curated list.
- **Chats** store messages separately from document blocks.

Blocks form a flat array in document order, with `indent` describing the
hierarchy. Inline text formatting uses Markdown. The title and description
live in the `name` and `description` properties.

| Task | Request |
| --- | --- |
| Find available spaces | `GET /v2/spaces` |
| Search within a space | `POST /v2/spaces/{space_id}/search` |
| Search across spaces | `POST /v2/search` |
| Read a document | `GET /v2/spaces/{space_id}/objects/{object_id}` |
| Create an object | `POST /v2/spaces/{space_id}/objects` |
| Edit a document | `PATCH /v2/spaces/{space_id}/objects/{object_id}` |

## Names and identifiers

Names owned by v2 use `snake_case`: path and query parameters, request and
response fields, and `PATCH` operation names. Content inside an AnyBlock
document retains the format's own field vocabulary.

### Property and type keys

Reads use stable keys such as `due_date` by default. A key is minted when
the property or type is created and stays the same after a rename. Use keys
in scripts, configuration, and integrations.

Use `?keys=name` to read display names such as `Due date` instead. This is
useful when showing users their own vocabulary.

Inputs accept keys, display names, and forgiving spelling variants. For
example, `due_date`, `dueDate`, `due-date`, and `Due date` can address the same
property. An ambiguous spelling returns `400` with the candidates. An
unknown spelling never creates a property implicitly.

Select options are addressed by their names. Object references remain full
object ids and are included inline.

### Block and view ids

Default object reads shorten machine-minted block ids to document-local
suffixes. Use ids exactly as the read returns them when editing. Routes and
operations that address a block or view accept a full id or a unique suffix;
an ambiguous suffix is refused with a list of candidates.

Use `?ids=full` for the export shape, with full ids throughout. This is the
shape to store for a backup or use when cloning a document.

### Space references

A space is normally served with a short reference: the last six characters
of the first half of its full `<cid>.<replicationKey>` id. Space-scoped routes
accept either spelling. Resolution tries an exact id first, then a unique
suffix among the spaces the key can access.

An ambiguous reference returns `400` with the candidates. Spaces whose
suffixes collide are served with full ids.

A short reference is only unique among the spaces currently visible to the
key. Joining another space can make it ambiguous. Use `?ids=full` whenever a
space reference will be stored outside the API.

## Read and search efficiently

Object reads return the editable document by default. For a smaller read:

- `?outline=true` returns the block structure with shortened text.
- `?include=properties` or `?include=blocks` selects part of the object.
- `?block={block_id}` returns one subtree, marked as a partial document.
- `?format=md` returns a read-only Markdown rendering.

Partial documents and Markdown renderings cannot be sent back as whole
document writes. Use `PATCH` operations to edit the object.

List and search rows are compact: `id`, `name`, `type`, and the properties
you request. They do not embed a type object. Request the fields you need
instead of reading every object separately.

Lists use `offset` and `limit`, with 25 rows by default. Responses include
`total` and `has_more`, plus a narrowing hint when truncated. Chat messages
use an order-id cursor instead.

## Edit with a batch of operations

Send an `ops` array to the object's `PATCH` endpoint. For example, this body
changes the title and appends a paragraph:

```json
{
  "ops": [
    {
      "op": "set_properties",
      "set": { "name": "Meeting notes" }
    },
    {
      "op": "insert_blocks",
      "markdown": "Next step: review the proposal."
    }
  ]
}
```

The batch is atomic: an invalid operation rejects the whole request.
Discover the accepted fields for each operation at
`GET /v2/schemas/ops/{op}`.

### Concurrency, retries, and dry runs

| Control | Behavior |
| --- | --- |
| `ETag` / `If-Match` | Object reads return an `etag` in the body and an `ETag` header. Send it back in `If-Match` to check for concurrent changes. A stale value returns `409` with the current etag. Without the header, the last write wins. |
| `Idempotency-Key` | Resource mutations replay the stored response when retried with the same key and request. `POST /v2/validate` also accepts a key. Search requests do not. |
| `?dry_run=true` | Resource mutations validate the request and report the outcome without committing. A dry run returns `200` where the committed create would return `201`. Each operation documents any limits to its dry-run result. |

Chats use order ids and `last_state_id` for concurrency, and have no etag.

Resource bodies reject unknown fields with `400` and name the offending
field. Document bodies are capped at 10 MiB; structured bodies at 1 MiB.

Deleting an object, type, or property archives it in Bin, where the Anytype
app can restore it. Object deletion is limited by creator provenance; see
the [delete operation](https://developers.anytype.io/docs/reference/v2/delete-object)
for its requirements. Deleting a chat message, including any attachment
cleanup that follows, is not an archive.

## Errors and warnings

Errors from v2 handlers have this structure:

```json
{
  "status": 400,
  "code": "...",
  "message": "...",
  "issues": [
    { "path": "...", "message": "...", "hint": "..." }
  ]
}
```

All four top-level fields are always present. `issues` is an empty array
when there is no path to name. Issues identify the failing input and can
describe the allowed values or how to repair the request.

Some refusals occur before v2 handlers run:

- Pairing, authentication, key-scope checks, request-origin checks, and the
  shared write rate limit use the older shared error envelope.
- An unmatched route or an unhandled panic returns no error envelope.

A read does not fail because it encounters content its representation
cannot express. Such content is reported in `warnings` beside the result.

## Stream chat messages

`GET /v2/spaces/{space_id}/chats/{chat_id}/messages/stream` opens a
Server-Sent Events connection. It starts with the last `limit` messages as
`message_added` events, then delivers live changes:

- `message_added`, `message_updated`, and `message_deleted`
- `reactions_updated`, `state_updated`, and `pinned_updated`

Idle connections stay open with comment lines that clients ignore.

Each addition carries the chat state id as its event id. An id-only frame
after the opening window carries the highest such id. Send the last id back
as `Last-Event-ID` to resume a dropped connection against the same node.

**Only additions replay.** Edits, deletions, pins, and reactions do not
change that state id. Re-read messages to refresh changes made while the
connection was down.

A `resync_required` event means the retained messages could not establish
coverage of the gap since your last event id. Content outside the following
window is unverified. The absence of this event does not guarantee a
complete history: out-of-order additions or reindexing can create gaps the
stream cannot detect. Re-read messages when you need certainty.

## Download files and icons

Use `GET /v2/spaces/{space_id}/files/{file_id}/content` with a file id or the
`icon_image` value from a space or member. The response contains bytes with
the matching `Content-Type`.

Downloads support byte ranges and conditional requests with `ETag` or
`Last-Modified`. File preconditions use standard status codes, including
`304` and `412`. Access is checked on every request.

## Discover schemas at runtime

| Request | Result |
| --- | --- |
| `GET /v2/schemas` | Available schema kinds |
| `GET /v2/schemas/{kind}` | The schema for one kind |
| `GET /v2/schemas/ops/{op}` | The schema for one edit operation |

The schemas support strict constrained decoding. Read them before writing
requests, and use `POST /v2/validate` to check an AnyBlock document.
