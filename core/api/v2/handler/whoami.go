package v2handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
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
//	@Summary		Get API key details
//	@Description	Describes the key, not a person. Branch on `grant.restricted`: true means the key reaches only the spaces in `spaces`; false means every space, either an all-spaces grant or a legacy key with no grant. An all-spaces grant reports `space_count` and lists its spaces only when `spaces` is true.
//	@Id				auth_whoami
//	@Tags			Auth
//	@Produce		json
//	@Param			ids		query		string					false	"How grant.spaces[].id is spelled: compact (default) is the short space reference; full is the whole <cid>.<replicationKey> id, and the spelling to store outside this API"
//	@Param			spaces	query		bool					false	"For an all-spaces grant: list the current live spaces in grant.spaces. Default false, the count alone"
//	@Success		200		{object}	v2model.WhoamiResponse	"The key's grant, as it is enforced"
//	@Failure		401		{object}	util.UnauthorizedError	"Missing, unknown, revoked or expired key. This is the shared auth envelope, not this API's error shape."
//	@Failure		403		{object}	util.ForbiddenError		"The key's scope does not admit this API. This is the shared scope gate's envelope."
//	@Security		bearerauth
//	@Router			/v2/auth/whoami [get]
func WhoamiHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		enumerate := false
		switch value, present := c.GetQuery("spaces"); {
		case !present, value == "false":
		case value == "true":
			enumerate = true
		default:
			RespondError(c, v2model.ValidationFailed("invalid spaces value",
				v2model.Issue{Path: "spaces", Message: fmt.Sprintf("unknown value %q", c.Query("spaces")), Hint: "allowed: true, false"}))
			return
		}
		resp, err := s.Whoami(c.Request.Context(), enumerate)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}
