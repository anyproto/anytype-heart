package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/util"
)

// DisplayCodeHandler starts a new challenge and returns the challenge ID
//
//	@Summary	Start new challenge
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		app_name	query		string					true	"App name requesting the challenge"
//	@Success	200			{object}	DisplayCodeResponse		"Challenge ID"
//	@Failure	400			{object}	util.ValidationError	"Invalid input"
//	@Failure	500			{object}	util.ServerError		"Internal server error"
//	@Router		/auth/display_code [post]
func DisplayCodeHandler(s *AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		appName := c.Query("app_name")

		challengeId, err := s.NewChallenge(c.Request.Context(), appName)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrMissingAppName, http.StatusBadRequest),
			util.ErrToCode(ErrFailedGenerateChallenge, http.StatusInternalServerError))

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, DisplayCodeResponse{ChallengeId: challengeId})
	}
}

// TokenHandler retrieves an authentication token using a code and challenge ID
//
//	@Summary	Retrieve token
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		challenge_id	query		string					true	"Challenge ID"
//	@Param		code			query		string					true	"4-digit code retrieved from Anytype Desktop app"
//	@Success	200				{object}	TokenResponse			"Authentication token"
//	@Failure	400				{object}	util.ValidationError	"Invalid input"
//	@Failure	500				{object}	util.ServerError		"Internal server error"
//	@Router		/auth/token [post]
func TokenHandler(s *AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		challengeId := c.Query("challenge_id")
		code := c.Query("code")

		sessionToken, appKey, err := s.SolveChallenge(c.Request.Context(), challengeId, code)
		errCode := util.MapErrorCode(err,
			util.ErrToCode(ErrInvalidInput, http.StatusBadRequest),
			util.ErrToCode(ErrFailedAuthenticate, http.StatusInternalServerError),
		)

		if errCode != http.StatusOK {
			apiErr := util.CodeToAPIError(errCode, err.Error())
			c.JSON(errCode, apiErr)
			return
		}

		c.JSON(http.StatusOK, TokenResponse{
			SessionToken: sessionToken,
			AppKey:       appKey,
		})
	}
}
