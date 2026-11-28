package domain

// integrationname.go holds the integration-name primitive shared by the API
// DELETE provenance rule (core/api/APIV2_OBJECT_DELETE.md §5/§11.2-3) and
// API change attribution (docs/ApiChangeAttribution.md): the RAW app name of
// the paired API key, stamped by heart into EVERY change it authors while
// serving a local-API request — tree changes (pb.Change.IntegrationName) and
// store changes (pb.StoreChange.IntegrationName) — from the authenticated
// session, never accepted from a request body, and never normalized.
//
// The name on the change is all there is: no integration object, no slug, no
// icon. An earlier spec (docs/IntegrationAttribution.md, never merged) had
// those; it is superseded.
//
// The value used to be a normalized slug (lowercase, [a-z0-9_-], collapsed,
// capped). That was dropped deliberately: normalization is many-to-one
// ("Claude/Desktop", "CLAUDE DESKTOP" and "Claude Desktop" all collapsed to
// one principal, so a key paired under one visibly different name could
// archive another's output) and lossy (a Cyrillic/CJK/emoji-only name
// normalized to "", silently creating permanently unprovenanced objects).
// Normalization bought tolerance — re-pairing as "claude desktop" still
// matched "Claude Desktop" — but for an AUTHORIZATION comparison tolerance
// is a liability; exact match is the point. The record's unforgeability
// never came from the value anyway — it comes from the signature and ACL
// validation on the change that carries it (§6). No slug is needed
// anywhere — the former IntegrationKeyFromAppName helper is deleted rather
// than left as a rival spelling authority.

import (
	"context"
)

// MaxIntegrationNameLen bounds the raw app name in bytes, enforced where
// names are minted (CreateApp and the challenge flow — reject, never
// truncate: truncation is normalization again, two names differing only in
// their tail would collapse to one principal). The name rides every
// change an API request authors and appears in debug exports, so it must be
// bounded;
// 128 bytes is double the old slug cap and comfortably covers every real
// app name. The stamp site deliberately does NOT re-check: a legacy key
// minted before the bound keeps working, and the recorded value always
// equals the session's name exactly.
const MaxIntegrationNameLen = 128

// ctxKeyIntegrationName carries the session's raw app name on a request
// context. It lives in domain (not core/api/util) because the consumers are
// the editor (smartblock init and apply, the store push) and the one
// installer is a transport middleware: the shared HTTP auth middleware, for
// both API versions. The apps' gRPC calls install nothing and are never
// stamped. Inside heart the name also rides a derived context where an apply
// hands its attribution to a store push (CtxWithIntegrationName on a
// component context) — never on a context that outlives the request's work.
type ctxKeyIntegrationName struct{}

// CtxWithIntegrationName installs the session's raw app name on ctx.
// An empty name installs nothing: absence and empty must stay one state
// (no stamp).
func CtxWithIntegrationName(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyIntegrationName{}, name)
}

// CtxWithoutIntegrationName returns ctx with the app name taken off — for
// work a request triggers but did not ask for (a GC cascade), which must not
// be attributed to the app. Everything else ctx carries stays.
func CtxWithoutIntegrationName(ctx context.Context) context.Context {
	if IntegrationNameFromCtx(ctx) == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyIntegrationName{}, "")
}

// IntegrationNameFromCtx returns the raw app name riding ctx, or "" —
// nil-safe, because the creation pipeline sees nil ctx in tests and from
// internal callers.
func IntegrationNameFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	name, _ := ctx.Value(ctxKeyIntegrationName{}).(string)
	return name
}
