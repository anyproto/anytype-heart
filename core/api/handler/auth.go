package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

/*
TO BE DEPRECATED
// DisplayCodeHandler starts a new challenge and returns the challenge ID
//
// 	@Summary		Start challenge
// 	@Description	Generates a one-time authentication challenge for granting API access to the user's vault. Upon providing a valid `app_name`, the server issues a unique `challenge_id` and displays a short code within the Anytype Desktop. The `challenge_id` must then be used with the token endpoint (see below) to solve the challenge and retrieve an authentication token. This mechanism ensures that only trusted applications and authorized users gain access.
// 	@ID				create_auth_challenge
// 	@Tags			Auth
// 	@Accept			json
// 	@Produce		json
// 	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-05-20)
// 	@Param			app_name		query		string							true	"The name of the app requesting API access"
// 	@Success		200				{object}	apimodel.DisplayCodeResponse	"The challenge ID associated with the started challenge"
// 	@Failure		400				{object}	util.ValidationError			"Bad request"
// 	@Failure		500				{object}	util.ServerError				"Internal server error"
// 	@Router			/v1/auth/display_code [post]
*/
func DisplayCodeHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		appName := c.Query("app_name")

		challengeId, err := s.CreateChallenge(c.Request.Context(), appName)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrMissingAppName, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedCreateNewChallenge, http.StatusInternalServerError))

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(299, apimodel.DisplayCodeResponse{ChallengeId: challengeId})
	}
}

/*
TO BE DEPRECATED
// TokenHandler retrieves an authentication token using a code and challenge ID
//
//	@Summary		Solve challenge
//	@Description	After receiving a `challenge_id` from the `display_code` endpoint, the client calls this endpoint to provide the corresponding 4-digit code along with the challenge ID. The endpoint verifies that the challenge solution is correct and, if it is, returns a permanent `app_key`. This endpoint is central to the authentication process, as it validates the user's identity and issues a token that can be used for further interactions with the API.
//	@ID				solve_auth_challenge
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			challenge_id	query		string					true	"The ID of the challenge to solve"
//	@Param			code			query		string					true	"4-digit code retrieved from Anytype Desktop app"
//	@Success		200				{object}	apimodel.TokenResponse	"The app key that can be used in the Authorization header for subsequent requests"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Router			/v1/auth/token [post]
*/
func TokenHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		challengeId := c.Query("challenge_id")
		code := c.Query("code")

		appKey, err := s.SolveChallenge(c.Request.Context(), challengeId, code)
		errCode := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedAuthenticate, http.StatusInternalServerError),
		)

		if errCode != http.StatusOK {
			apiErr := util.CodeToApiError(errCode, err.Error())
			c.JSON(errCode, apiErr)
			return
		}

		c.JSON(299, apimodel.TokenResponse{AppKey: appKey})
	}
}

// CreateChallengeHandler creates a new challenge for API key generation
//
//	@Summary		Create Challenge
//	@Description	Generates a one-time authentication challenge for granting API access to the user's vault. Upon providing a valid `app_name`, the server issues a unique `challenge_id` and displays a 4-digit code within the Anytype Desktop. The `challenge_id` must then be used with the `/v1/auth/api_keys` endpoint to solve the challenge and retrieve an authentication token. This mechanism ensures that only trusted applications and authorized users gain access.
//	@ID				create_auth_challenge
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string								true	"The version of the API to use"	default(2025-05-20)
//	@Param			request			body		apimodel.CreateChallengeRequest		true	"The request body containing the app name"
//	@Success		201				{object}	apimodel.CreateChallengeResponse	"The challenge ID associated with the started challenge"
//	@Failure		400				{object}	util.ValidationError				"Bad request"
//	@Failure		500				{object}	util.ServerError					"Internal server error"
//	@Router			/v1/auth/challenges [post]
func CreateChallengeHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.CreateChallengeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		challengeId, err := s.CreateChallenge(c.Request.Context(), req.AppName)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrMissingAppName, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedCreateNewChallenge, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.CreateChallengeResponse{ChallengeId: challengeId})
	}
}

// CreateApiKeyHandler creates a new api key using a code and challenge ID
//
//	@Summary		Create API Key
//	@Description	After receiving a `challenge_id` from the `/v1/auth/challenges` endpoint, the client calls this endpoint to provide the corresponding 4-digit code along with the challenge ID. The endpoint verifies that the challenge solution is correct and, if it is, returns an `api_key`. This endpoint is central to the authentication process, as it validates the user's identity and issues a key that can be used for further interactions with the API.
//	@ID				create_api_key
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-05-20)
//	@Param			request			body		apimodel.CreateApiKeyRequest	true	"The request body containing the challenge ID and code"
//	@Success		201				{object}	apimodel.CreateApiKeyResponse	"The API key that can be used in the Authorization header for subsequent requests"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Router			/v1/auth/api_keys [post]
func CreateApiKeyHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.CreateApiKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		apiKey, err := s.SolveChallenge(c.Request.Context(), req.ChallengeId, req.Code)
		errCode := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedAuthenticate, http.StatusInternalServerError),
		)

		if errCode != http.StatusOK {
			apiErr := util.CodeToApiError(errCode, err.Error())
			c.JSON(errCode, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.CreateApiKeyResponse{ApiKey: apiKey})
	}
}
