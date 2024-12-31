package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/cmd/api/util"
)

// AuthDisplayCodeHandler generates a new challenge and returns the challenge ID
//
//	@Summary	Open a modal window with a code in Anytype Desktop app
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	AuthDisplayCodeResponse	"Challenge ID"
//	@Failure	502	{object}	util.ServerError		"Internal server error"
//	@Router		/auth/displayCode [post]
func AuthDisplayCodeHandler(s *AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		challengeId, err := s.GenerateNewChallenge(c.Request.Context(), "api-test")
		code := util.MapErrorCode(err, util.ErrToCode(ErrFailedGenerateChallenge, http.StatusInternalServerError))

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, AuthDisplayCodeResponse{ChallengeId: challengeId})
	}
}

// AuthTokenHandler retrieves an authentication token using a code and challenge ID
//
//	@Summary	Retrieve an authentication token using a code
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		code			query		string					true	"The code retrieved from Anytype Desktop app"
//	@Param		challenge_id	query		string					true	"The challenge ID"
//	@Success	200				{object}	AuthTokenResponse		"Authentication token"
//	@Failure	400				{object}	util.ValidationError	"Invalid input"
//	@Failure	502				{object}	util.ServerError		"Internal server error"
//	@Router		/auth/token [get]
func AuthTokenHandler(s *AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		challengeID := c.Query("challenge_id")
		code := c.Query("code")

		sessionToken, appKey, err := s.SolveChallengeForToken(c.Request.Context(), challengeID, code)
		errCode := util.MapErrorCode(err,
			util.ErrToCode(ErrInvalidInput, http.StatusBadRequest),
			util.ErrToCode(ErrFailedAuthenticate, http.StatusInternalServerError),
		)

		if errCode != http.StatusOK {
			apiErr := util.CodeToAPIError(errCode, err.Error())
			c.JSON(errCode, apiErr)
			return
		}

		c.JSON(http.StatusOK, AuthTokenResponse{
			SessionToken: sessionToken,
			AppKey:       appKey,
		})
	}
}
