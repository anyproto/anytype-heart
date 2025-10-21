package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

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

		apiKey, err := s.CreateApiKey(c.Request.Context(), req.ChallengeId, req.Code)
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
