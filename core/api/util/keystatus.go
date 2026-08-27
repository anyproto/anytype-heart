package util

// keystatus.go carries the authenticated credential's description and the
// legacy-key deprecation signal through the HTTP layer. Both route groups
// emit the signal headers; the /v2 whoami body repeats the same values —
// agents read bodies, not headers — so the constants live here, in the one
// package both sides already share, and the header and the body cannot
// drift apart.

import (
	"context"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ApiVersion is the API version reported by the Anytype-Version response
// header and the whoami body's api.version — one constant, two mirrors.
const ApiVersion = "2025-11-08"

// The legacy-key deprecation signal (design spec P1 §6). Deliberately NOT
// RFC 9745 Deprecation/Sunset: those headers require a Date value and are
// scoped to the RESOURCE in the response — emitting them on /v1 would
// declare /v1 deprecated, the opposite of the grandfathering promise.
// Anytype-Key-Status names the CREDENTIAL, the thing that is actually
// legacy, and is sent on every authenticated response — a client never has
// to treat absence as meaningful.
const (
	KeyStatusHeader = "Anytype-Key-Status"
	KeyStatusLegacy = "legacy"
	KeyStatusScoped = "scoped"

	// NoticeHeader carries one short single-line ASCII sentence a client can
	// print verbatim (npm's notice pattern). It never interpolates user data.
	NoticeHeader = "Anytype-Notice"
	// LegacyKeyNotice is the sentence itself — also embedded in the whoami
	// body for legacy keys. Text is API surface; tests pin it. Emitted only
	// for nil-grant keys of JsonAPI scope: a grant is only ever valid on
	// JsonAPI scope (wallet.ValidateAppLinkGrant), so a Limited or Full
	// credential cannot follow the re-issue advice and never gets it.
	LegacyKeyNotice = "This API key is a legacy unscoped key with access to every space. It will keep working. Re-issue it as a scoped key in Settings > API Keys."

	// KeyDeprecationLink is the Link header value pointing at the key
	// policy. rel="deprecation" WITHOUT a Deprecation header is RFC 9745
	// §3.1's own worked example for "here is the policy, no date committed".
	// The target is the developer portal's live authentication guide (its
	// Settings > API Keys walkthrough); move it to the dedicated key-scoping
	// section when that page ships (design spec P1 §7) — a policy link that
	// 404s would invert the signal's whole point.
	KeyDeprecationLink = `<https://developers.anytype.io/docs/guides/get-started/authentication>; rel="deprecation"; type="text/html"`
)

// KeyStatus names the credential kind for the signal: legacy for nil-grant
// keys, scoped otherwise. Grant PRESENCE decides, never key-string format —
// a legacy-format key can be granted in place and a new-format key can be
// unscoped.
func KeyStatus(grant *ApiGrant) string {
	if grant == nil {
		return KeyStatusLegacy
	}
	return KeyStatusScoped
}

// ApiKeyInfo describes the authenticated CREDENTIAL (never the person) for
// introspection: the app link attributes WalletCreateSession resolved at
// session mint. Zero timestamps mean unknown (CreatedAt) / never (ExpiresAt).
type ApiKeyInfo struct {
	Id        string // the app link's hash — the id ListApps shows
	Name      string
	CreatedAt int64
	ExpiresAt int64
	Scope     model.AccountAuthLocalApiScope
}

// apiKeyInfoCtxKey is the private carrier type for the credential
// description on the request context.
type apiKeyInfoCtxKey struct{}

// CtxWithApiKeyInfo stores the authenticated credential's description on the
// request context, next to the grant (CtxWithApiGrant) — whoami reads both
// from the same carriers the enforcement path populates.
func CtxWithApiKeyInfo(ctx context.Context, info ApiKeyInfo) context.Context {
	return context.WithValue(ctx, apiKeyInfoCtxKey{}, info)
}

// ApiKeyInfoFromCtx returns the request's credential description; ok is
// false when the request never passed ensureAuthenticated.
func ApiKeyInfoFromCtx(ctx context.Context) (ApiKeyInfo, bool) {
	info, ok := ctx.Value(apiKeyInfoCtxKey{}).(ApiKeyInfo)
	return info, ok
}
