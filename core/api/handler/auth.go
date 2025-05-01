package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// DisplayCodeHandler starts a new challenge and returns the challenge ID
//
//	@Summary		Start new challenge
//	@Description	This endpoint initiates a secure authentication flow by generating a new challenge. Clients must supply the name of the application (via a query parameter) that is requesting authentication. On success, the service returns a unique challenge ID. This challenge ID must then be used with the token endpoint (see below) to solve the challenge and retrieve an authentication token. In essence, this endpoint “boots up” the login process and is the first step in a multi-phase authentication sequence.
//	@ID				createAuthChallenge
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-04-22)
//	@Param			app_name		query		string							true	"App name requesting the challenge"
//	@Success		200				{object}	apimodel.DisplayCodeResponse	"Challenge ID"
//	@Failure		400				{object}	util.ValidationError			"Invalid input"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Router			/auth/display_code [post]
func DisplayCodeHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		appName := c.Query("app_name")

		challengeId, err := s.NewChallenge(c.Request.Context(), appName)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrMissingAppName, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedGenerateChallenge, http.StatusInternalServerError))

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.DisplayCodeResponse{ChallengeId: challengeId})
	}
}

// TokenHandler retrieves an authentication token using a code and challenge ID
//
//	@Summary		Solve challenge
//	@Description	After receiving a challenge ID from the display_code endpoint, the client calls this endpoint to provide the corresponding 4-digit code (also via a query parameter) along with the challenge ID. The endpoint verifies that the challenge solution is correct and, if it is, returns an ephemeral session token together with a permanent app key. These tokens are then used in subsequent API requests to authorize access. This endpoint is central to ensuring that only properly authenticated sessions can access further resources.
//	@ID				solveAuthChallenge
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			challenge_id	query		string					true	"Challenge ID"
//	@Param			code			query		string					true	"4-digit code retrieved from Anytype Desktop app"
//	@Success		200				{object}	apimodel.TokenResponse	"Authentication token"
//	@Failure		400				{object}	util.ValidationError	"Invalid input"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Router			/auth/token [post]
func TokenHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		challengeId := c.Query("challenge_id")
		code := c.Query("code")

		appKey, err := s.SolveChallenge(c.Request.Context(), challengeId, code)
		errCode := util.MapErrorCode(err,
			util.ErrToCode(service.ErrInvalidInput, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedAuthenticate, http.StatusInternalServerError),
		)

		if errCode != http.StatusOK {
			apiErr := util.CodeToAPIError(errCode, err.Error())
			c.JSON(errCode, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TokenResponse{AppKey: appKey})
	}
}
