package v2handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// WhoamiV2Handler introspects the API key
//
// whoami is DISCOVERY, not enforcement, and the body is derived from the
// SAME grant record the space-grant gate reads — the request-context
// carriers ensureAuthenticated populated — never computed separately: a
// second derivation path is how this mirror and the gate drift apart and
// the mirror starts lying to agents that shape their tool surface from it
// (the derivation itself lives in V2Service.Whoami). The credential is read
// ONLY from the Authorization header, by the shared auth middleware; a
// token is never accepted as a query or body parameter — that is what
// would turn this endpoint into the enumeration oracle RFC 7662 §4 warns
// about, so RFC 7662's shape (POST form, `active` field) is deliberately
// not implemented. An unknown or revoked key never reaches this handler: it
// gets the auth middleware's plain 401.
//
//	@Summary		Introspect the API key
//	@Description	Describes the CREDENTIAL presented in the Authorization header — never the person; there is only one account on this API. `grant.scoped` is the load-bearing field: false means a legacy unscoped key (then `spaces` is `[]` and `permission` is null — never null spaces), true means enforcement constrains the key to exactly the listed spaces with the listed permission. `spaces[]` entries carry a per-entry `permission` (uniform today; per-space grants land without a wire change) and a `name` resolved through the same grant-intersected path as GET /v2/spaces. `keyStatus`/`notice` repeat the Anytype-Key-Status/Anytype-Notice header signal in the body. The token is read ONLY from the Authorization header; it is never accepted as a query or body parameter, and an unknown or revoked key gets a plain 401.
//	@Id				v2_auth_whoami
//	@Tags			V2
//	@Produce		json
//	@Param			ids	query		string					false	"How grant.spaces[].id is spelled: compact (default) = the short space reference; full = the full <cid>.<replicationKey> id — the spelling to persist outside this API"
//	@Success		200	{object}	v2model.WhoamiResponse	"The credential's grant as enforced"
//	@Failure		401	{object}	util.UnauthorizedError	"Missing, unknown, revoked or expired key — the shared auth middleware's envelope (APIV2.md §8.9 seam), not the C6 shape"
//	@Failure		403	{object}	util.ForbiddenError		"Key scope does not admit the JSON API (e.g. Limited) — the shared scope gate's envelope"
//	@Security		bearerauth
//	@Router			/v2/auth/whoami [get]
func WhoamiV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := s.Whoami(c.Request.Context())
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}
