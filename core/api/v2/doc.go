// Package apiv2 registers and serves the Anytype local API v2.
//
// doc.go carries nothing but the OpenAPI general-info block for the v2
// document; its Introduction lives in markdown/api.md.
// `make openapi` runs swag twice — once over core/api with
// core/api/v2 excluded (the v1 document, general info in core/api/service.go)
// and once over core/api/v2 with this file as -g. Keeping the two blocks in
// two packages is why a v2 reader never scrolls past a v1 endpoint.
//
//	@title		Anytype API v2
//	@version	2025-11-08
//	@description.markdown
//	@tag.name						Auth
//	@tag.description				Obtain an API key with Desktop approval, then inspect which spaces and permissions it grants.
//	@tag.name						Spaces
//	@tag.description				The containers everything else lives in. Nearly every other route is scoped to one.
//	@tag.name						Objects
//	@tag.description				Read and write whole AnyBlock documents: one GET returns an editable document, one PATCH edits it.
//	@tag.name						Search
//	@tag.description				Find objects by query, filter and sort, within one space or across all of them.
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
//	@tag.description				Upload files and download file or icon content.
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
