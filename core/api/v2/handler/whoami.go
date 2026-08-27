package v2handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// WhoamiHandler introspects the API key
//
// whoami is DISCOVERY, not enforcement, and the body is derived from the
// SAME grant record the space-grant gate reads — the request-context
// carriers ensureAuthenticated populated — never computed separately: a
// second derivation path is how this mirror and the gate drift apart and
// the mirror starts lying to agents that shape their tool surface from it
// (the derivation itself lives in Service.Whoami). The credential is read
// ONLY from the Authorization header, by the shared auth middleware; a
// token is never accepted as a query or body parameter — that is what
// would turn this endpoint into the enumeration oracle RFC 7662 §4 warns
// about, so RFC 7662's shape (POST form, `active` field) is deliberately
// not implemented. An unknown or revoked key never reaches this handler: it
// gets the auth middleware's plain 401.
//
//	@Summary		Describe the calling key
//	@Description	This describes the key, not a person; there is one account behind this API. Branch on `grant.scoped`. False is a legacy key with no space restriction, and its `spaces` list is empty rather than absent. True means the key reaches exactly the spaces listed, with the permission listed beside each one.
//	@Id				auth_whoami
//	@Tags			Auth
//	@Produce		json
//	@Param			ids	query		string					false	"How grant.spaces[].id is spelled: compact (default) is the short space reference; full is the whole <cid>.<replicationKey> id, and the spelling to store outside this API"
//	@Success		200	{object}	v2model.WhoamiResponse	"The key's grant, as it is enforced"
//	@Failure		401	{object}	util.UnauthorizedError	"Missing, unknown, revoked or expired key. This is the shared auth envelope, not this API's error shape."
//	@Failure		403	{object}	util.ForbiddenError		"The key's scope does not admit this API. This is the shared scope gate's envelope."
//	@Security		bearerauth
//	@Router			/v2/auth/whoami [get]
func WhoamiHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := s.Whoami(c.Request.Context())
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}
