package util

// grant.go carries an app key's space grant through the HTTP layer. The
// grant record itself lives in the wallet (core/wallet.AppLinkGrant, sealed
// into the app-link file); WalletCreateSession surfaces it as a proto
// message, and this package holds the plain form both route groups and the
// v2 service read — the request context.Context is the carrier, like the
// app name above.

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Grant permission levels. The vocabulary matches the wallet's persisted
// perms values (core/wallet: AppLinkPermsRead / AppLinkPermsReadWrite).
const (
	GrantPermsRead      = "read"
	GrantPermsReadWrite = "readwrite"
)

// ApiGrant is the HTTP layer's view of an app key's space grant. A nil
// *ApiGrant means an unscoped/legacy key — enforcement passes it through
// unchanged. A non-nil grant constrains every request to Spaces × Perms.
type ApiGrant struct {
	Spaces []string `json:"spaces"`
	Perms  string   `json:"perms"` // GrantPermsRead | GrantPermsReadWrite
}

// ApiGrantFromProto converts the WalletCreateSession grant. An unrecognized
// permission value maps to read, never to readwrite: an enum this binary
// does not know must not widen into write access.
func ApiGrantFromProto(grant *model.AccountAuthAppGrant) *ApiGrant {
	if grant == nil {
		return nil
	}
	perms := GrantPermsRead
	if grant.Perm == model.AccountAuthAppGrant_ReadWrite {
		perms = GrantPermsReadWrite
	}
	return &ApiGrant{
		Spaces: append([]string(nil), grant.SpaceIds...),
		Perms:  perms,
	}
}

// AllowsSpace reports whether the grant covers spaceId. Nil means
// unscoped/legacy and callers branch on that BEFORE calling — a nil
// receiver here answers false, so a caller that forgets the branch fails
// closed instead of open. An EMPTY Spaces list also denies every space:
// empty must be impossible (persist-time validation rejects it) and, if
// ever encountered, must NEVER be read as "all spaces" — the loop's
// vacuous false is load-bearing.
func (g *ApiGrant) AllowsSpace(spaceId string) bool {
	if g == nil || spaceId == "" {
		return false
	}
	for _, granted := range g.Spaces {
		if granted == spaceId {
			return true
		}
	}
	return false
}

// CanWrite reports whether the grant permits write-classified routes. Only
// the exact readwrite value passes — an empty or unknown Perms is read at
// most (fail closed).
func (g *ApiGrant) CanWrite() bool {
	return g != nil && g.Perms == GrantPermsReadWrite
}

// Describe renders the grant for 403 messages: error-guided self-correction
// is the v2 design language, and enumeration resistance is a non-goal on a
// localhost single-user API, so the message names the actual grant.
func (g *ApiGrant) Describe() string {
	if g == nil {
		return "unscoped"
	}
	return fmt.Sprintf("spaces [%s] with %s access", strings.Join(g.Spaces, ", "), g.Perms)
}

// apiGrantCtxKey is the private carrier type for the authenticated key's
// grant on the request context.
type apiGrantCtxKey struct{}

// CtxWithApiGrant stores the authenticated key's grant on the context. The
// gin context carries the full session entry for route middleware; the
// request context is what the v2 service layer reads (the fan-out
// constraint and the ensureSpace backstop), so the grant must ride both.
func CtxWithApiGrant(ctx context.Context, grant *ApiGrant) context.Context {
	return context.WithValue(ctx, apiGrantCtxKey{}, grant)
}

// ApiGrantFromCtx returns the request's grant, nil when the key is
// unscoped/legacy (or the request never passed ensureAuthenticated).
func ApiGrantFromCtx(ctx context.Context) *ApiGrant {
	grant, _ := ctx.Value(apiGrantCtxKey{}).(*ApiGrant)
	return grant
}

//
// ---- WWW-Authenticate (RFC 6750) ----
//

// WwwAuthenticateHeader is emitted alongside the JSON error envelope on
// auth failures; MCP clients are required to parse it (spec rev 2025-06-18).
const WwwAuthenticateHeader = "WWW-Authenticate"

// BearerChallenge is the 401 challenge when the request carried no
// credentials at all (RFC 6750 §3: no error code then).
func BearerChallenge() string {
	return `Bearer realm="anytype"`
}

// BearerChallengeInvalidToken is the 401 challenge for a present but
// unusable credential (malformed, unknown, revoked, expired).
func BearerChallengeInvalidToken() string {
	return `Bearer realm="anytype", error="invalid_token"`
}

// BearerChallengeInsufficientScope is the 403 challenge for an
// authenticated key whose scope or grant does not cover the request. scope
// may be empty when the request maps to no single space.
func BearerChallengeInsufficientScope(scope string) string {
	if scope == "" {
		return `Bearer error="insufficient_scope"`
	}
	return fmt.Sprintf(`Bearer error="insufficient_scope", scope=%q`, scope)
}

// SpaceScope renders the implementation-defined RFC 6750 §3.1 scope string
// this API documents: `space:<spaceId>:<read|readwrite>` — the space the
// request addressed and the permission it needed.
func SpaceScope(spaceId, perms string) string {
	return fmt.Sprintf("space:%s:%s", spaceId, perms)
}
