// Package apiv2 registers and serves the Anytype local API v2.
//
// doc.go carries nothing but the OpenAPI general-info block for the v2
// document. `make openapi` runs swag twice — once over core/api with
// core/api/v2 excluded (the v1 document, general info in core/api/service.go)
// and once over core/api/v2 with this file as -g. Keeping the two blocks in
// two packages is why a v2 reader never scrolls past a v1 endpoint.
//
//	@title							Anytype API v2
//	@version						2025-11-08
//	@description					The agent-oriented Anytype local API. Objects are AnyBlock JSON documents (pkg/lib/anyblockjson/SPEC.md) rather than block trees, so one read returns a whole editable document and one PATCH edits it.
//	@description					One vocabulary throughout (C2): snake_case everywhere this API names something of its own — path and query params, request and response fields, and the PATCH op names alike. Things are addressed by name, never by id: property keys, option names, and `type` as a type key. Inside an object's `blocks` and `properties` you are reading the AnyBlock format's own vocabulary, which passes through unchanged. Responses are compact JSON (C3); object refs are always full inline, and the default object read relabels machine-minted block ids to short doc-local suffixes — `?ids=full` returns the export shape with full ids everywhere — the backup shape, and the read to clone from (C4); list and search rows carry id, name, type and the properties you asked for, and never embed type objects (C5).
//	@description					Errors share one shape (C6): {status, code, message, issues:[{path, message, hint}]}, path-addressed and naming the allowed values, so a failed call tells an agent how to repair it. Reads never fail on unknown content — anything a representation cannot express is reported in `warnings` (C11).
//	@description					Every object read returns an `etag` derived from the tree heads plus an ETag header; mutations take `If-Match` and are advisory by default (C7). Every mutation honours `Idempotency-Key` (C8) and `?dry_run=true` (C9). Every list surface is paginated with limit=25 by default and steers you when it truncates (C10).
//	@description					Schemas are discoverable at runtime and strict-mode-compatible for constrained decoding (C12, C13): GET /v2/schemas lists the kinds, GET /v2/schemas/{kind} returns one, and GET /v2/schemas/ops/{op} returns the schema of a single PATCH op.
//	@description					Spaces are served by a SHORT reference — the last six characters of the space id's CID half — and every route that takes a space accepts either that short reference or the full `<cid>.<replicationKey>` id. Resolution is exact id first, then a unique suffix, within the spaces the credential can see; an ambiguous reference is a 400 listing the candidates, and two spaces whose tails collide are both served in full. The full id keeps working everywhere.
//	@description					A short reference is an addressing convenience, not a stable identifier: it is unique only against the spaces the credential can currently see, so joining a space with a colliding tail retires it. `?ids=full` — the same parameter that asks an object read for the export shape — makes every space id in the response the full one instead: use it whenever a reference is going to be stored outside this API (a config file, a script, a log line, another API).
//	@tag.name						Auth
//	@tag.description				What the calling key may do — ask this before discovering limits through 403s.
//	@tag.name						Spaces
//	@tag.description				The containers everything else lives in. Nearly every other route is scoped to one.
//	@tag.name						Objects
//	@tag.description				Read and write whole AnyBlock documents: one GET returns an editable document, one PATCH edits it.
//	@tag.name						Search
//	@tag.description				Find objects by query, filter and sort — within a space or across all of them.
//	@tag.name						Types
//	@tag.description				An object's shape: the properties it recommends and the views it opens with.
//	@tag.name						Properties
//	@tag.description				The typed key-value fields objects carry, and the option vocabularies select fields draw from.
//	@tag.name						Lists
//	@tag.description				Sets (a live query over a type) and collections (a hand-curated list), with their views.
//	@tag.name						Chat
//	@tag.description				Messages, reactions and read state. Chats store messages outside blocks, paged by order-id cursors.
//	@tag.name						Members
//	@tag.description				Who is in a space, and which of them you are.
//	@tag.name						Files
//	@tag.description				Upload bytes and get the id that file blocks and chat attachments reference.
//	@tag.name						Templates
//	@tag.description				Starting documents for a type.
//	@tag.name						Schemas
//	@tag.description				The format itself: what a valid document looks like, what each PATCH op accepts, and a validator to check one against them. Read these before writing.
//	@termsOfService					https://anytype.io/terms_of_use
//	@contact.name					Anytype Support
//	@contact.url					https://anytype.io/contact
//	@contact.email					support@anytype.io
//	@license.name					Any Source Available License 1.0
//	@license.url					https://github.com/anyproto/anytype-api/blob/main/LICENSE.md
//	@host							http://127.0.0.1:31009
//	@securitydefinitions.bearerauth	BearerAuth
//	@externalDocs.description		OpenAPI
//	@externalDocs.url				https://swagger.io/resources/open-api/
//
// The version above is deliberately the same date as v1's: it is the value of
// the `Anytype-Version` response header (server.ApiVersion), which one gin
// engine sets for both route groups (C1). Bump all three together or none.
// (Nothing below the annotation block may start a line with an at-sign —
// swag reads every comment group in this file and would take it as an
// attribute.)
package apiv2
