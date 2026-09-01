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
	// AllSpaces grants every space in the account, including spaces created
	// after the grant was made (dynamic semantics). It never covers the
	// tech space — SpaceGrantRefusal owns that exclusion — and "all" is
	// never spelled as an empty Spaces list, which keeps denying everything.
	AllSpaces bool     `json:"allSpaces,omitempty"`
	Spaces    []string `json:"spaces"`
	Perms     string   `json:"perms"` // GrantPermsRead | GrantPermsReadWrite
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
		AllSpaces: grant.AllSpaces,
		Spaces:    append([]string(nil), grant.SpaceIds...),
		Perms:     perms,
	}
}

// AllowsSpace reports whether the grant covers spaceId. Nil means
// unscoped/legacy and callers branch on that BEFORE calling — a nil
// receiver here answers false, so a caller that forgets the branch fails
// closed instead of open. An EMPTY Spaces list also denies every space:
// empty must be impossible (persist-time validation rejects it) and, if
// ever encountered, must NEVER be read as "all spaces" — the loop's
// vacuous false is load-bearing; "all" is spelled only by the explicit
// AllSpaces flag.
//
// AllowsSpace stays a PURE function of the grant: under AllSpaces it
// answers true for every space id, the tech space included. The tech-space
// exclusion is SpaceGrantRefusal's, which the two enforcement points
// consult — a caller that needs the admission decision goes there.
func (g *ApiGrant) AllowsSpace(spaceId string) bool {
	if g == nil || spaceId == "" {
		return false
	}
	if g.AllSpaces {
		return true
	}
	for _, granted := range g.Spaces {
		if granted == spaceId {
			return true
		}
	}
	return false
}

// IsUnrestricted reports whether the grant grants no LESS than an unscoped
// key: allSpaces with readwrite. /v1's granted-key gate admits exactly this
// combination — the one grant /v1 can honor by doing nothing — and keeps
// refusing every narrower one, in the CanWrite fail-closed style: only the
// exact values pass. The recorded asymmetry (picker design §5): on /v1 such
// a key behaves exactly as an unscoped key does today, tech-space access
// included, while the same grant on /v2 excludes the tech space.
func (g *ApiGrant) IsUnrestricted() bool {
	return g != nil && g.AllSpaces && g.Perms == GrantPermsReadWrite
}

// SpaceGrantRefusal is the ONE space-admission check both enforcement
// points consult — the /v2 route gate (apiv2.ensureSpaceGrant) and the v2
// service backstop (Service.ensureSpaceGranted) — so a future caller cannot
// get the pair of rules half-right. It returns "" when the grant admits
// spaceId, else the refusal message for the 403.
//
// The tech-space rule lives here rather than in AllowsSpace, which stays a
// pure function of the grant: an all-spaces grant NEVER covers the tech
// space (account machinery — space views, profile — not user content, and
// writes to it can break the account). Reaching it requires listing it
// explicitly, which only a Full-scope CreateApp/UpdateApp caller can do. A
// nil grant is an unscoped/legacy key and is admitted unchanged.
func SpaceGrantRefusal(g *ApiGrant, spaceId, techSpaceId string) string {
	if g == nil {
		return ""
	}
	if g.AllSpaces && techSpaceId != "" && spaceId == techSpaceId {
		return "the tech space holds account machinery and is never covered by an all-spaces grant; it must be granted explicitly"
	}
	if !g.AllowsSpace(spaceId) {
		return fmt.Sprintf("key not granted space %q; granted: %s", spaceId, g.Describe())
	}
	return ""
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
	if g.AllSpaces {
		return fmt.Sprintf("all spaces with %s access", g.Perms)
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
