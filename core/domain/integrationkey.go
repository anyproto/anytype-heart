package domain

// integrationkey.go holds the integration-key primitive shared by the API
// DELETE provenance rule (core/api/APIV2_OBJECT_DELETE.md §5/§11.2-3) and,
// later, integration attribution (docs/IntegrationAttribution.md): a
// normalized slug of the paired app's name, stamped by heart into the change
// payloads it authors (pb.Change.IntegrationKey) from the authenticated
// session — never accepted from a request body.
//
// The slug's unforgeability does not come from the value — it comes from the
// signature and ACL validation on the change that carries it (§6). The value
// is deliberately human-meaningful and stable across key re-issue under the
// same app name (§8: the rotation story), which is why it is a slug and not
// a hash.

import (
	"context"
	"strings"
)

// integrationKeyMaxLen caps the slug (§5).
const integrationKeyMaxLen = 64

// IntegrationKeyFromAppName derives the integration key from an app link's
// AppName: lowercase, [a-z0-9_-] kept, every other run collapsed to a single
// '-', trimmed of edge dashes, capped at 64. Empty in ⇒ empty out (a key
// with no recorded name creates unprovenanced objects, §5). The same
// function later derives the attribution feature's per-space integration
// unique key ("integration-<slug>"), so the two features cannot diverge on
// what a given app name maps to.
func IntegrationKeyFromAppName(appName string) string {
	lower := strings.ToLower(appName)
	var b strings.Builder
	b.Grow(len(lower))
	pendingSep := false
	for _, r := range lower {
		legal := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !legal {
			pendingSep = true
			continue
		}
		if pendingSep && b.Len() > 0 {
			b.WriteByte('-')
		}
		pendingSep = false
		b.WriteRune(r)
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > integrationKeyMaxLen {
		slug = strings.Trim(slug[:integrationKeyMaxLen], "-")
	}
	return slug
}

// ctxKeyIntegrationKey carries the session's integration key on a request
// context. It lives in domain (not core/api/util) because the consumer is
// the object-creation pipeline (smartblock init → the creating change) and
// the installers are transport middlewares: today the shared HTTP auth
// middleware (both API versions), later attribution's gRPC interceptor —
// one neutral carrier, several installers.
type ctxKeyIntegrationKey struct{}

// CtxWithIntegrationKey installs the session's integration key on ctx.
// An empty key installs nothing: absence and empty must stay one state
// (no stamp).
func CtxWithIntegrationKey(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyIntegrationKey{}, key)
}

// IntegrationKeyFromCtx returns the integration key riding ctx, or "" —
// nil-safe, because the creation pipeline sees nil ctx in tests and from
// internal callers.
func IntegrationKeyFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	key, _ := ctx.Value(ctxKeyIntegrationKey{}).(string)
	return key
}
