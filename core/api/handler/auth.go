package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/session"
)

// CreateChallengeHandler creates a new challenge for API key generation
//
//	@Summary		Create a challenge
//	@Description	Starts pairing for the named app and returns a challenge_id. The user chooses spaces and permissions in Anytype Desktop, then approves to reveal a 4-digit code. Submit the challenge_id and code to /v1/auth/api_keys. No existing API key is required.
//	@ID				create_auth_challenge
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string								false	"The version of the API to use"	default(2025-11-08)
//	@Param			request			body		apimodel.CreateChallengeRequest		true	"The request body containing the app name"
//	@Success		201				{object}	apimodel.CreateChallengeResponse	"The challenge ID associated with the started challenge"
//	@Failure		400				{object}	util.ValidationError				"bad_request: send valid JSON with a nonempty app_name of at most 128 bytes"
//	@Failure		403				{object}	util.ForbiddenError					"Untrusted request origin or host"
//	@Failure		429				{object}	apimodel.PairingError				"pairing_rate_limited: stop requesting challenges and ask the user to check Anytype Desktop for a pairing prompt"
//	@Failure		500				{object}	util.ServerError					"Internal server error"
//	@Failure		503				{object}	apimodel.PairingError				"account_not_running: open Anytype Desktop and sign in before pairing"
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
		if err != nil {
			writePairingError(c, err)
			return
		}

		c.JSON(http.StatusCreated, apimodel.CreateChallengeResponse{ChallengeId: challengeId})
	}
}

// CreateApiKeyHandler creates a new api key using a code and challenge ID
//
//	@Summary		Create an API key
//	@Description	Exchanges a challenge_id from /v1/auth/challenges and the code revealed after Desktop approval for an api_key and the grant approved by the user. The grant states the permission, full space_ids, and whether all current and future user spaces are covered. Use the key as a bearer credential in the Authorization header. No existing API key is required.
//	@ID				create_api_key
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							false	"The version of the API to use"	default(2025-11-08)
//	@Param			request			body		apimodel.CreateApiKeyRequest	true	"The request body containing the challenge ID and code"
//	@Success		201				{object}	apimodel.CreateApiKeyResponse	"The API key and its approved grant"
//	@Failure		400				{object}	apimodel.PairingError			"bad_request: check the JSON, challenge_id and code. incorrect_answer: ask the user to check the four-digit code for this challenge"
//	@Failure		403				{object}	util.ForbiddenError				"Untrusted request origin or host"
//	@Failure		404				{object}	apimodel.PairingError			"invalid_challenge_id: start a new challenge if the user still wants to pair"
//	@Failure		409				{object}	apimodel.PairingError			"challenge_not_approved: wait for Desktop approval and submit the displayed code with the same challenge_id"
//	@Failure		429				{object}	apimodel.PairingError			"Too many pairing attempts"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Failure		503				{object}	apimodel.PairingError			"account_not_running: open Anytype Desktop and sign in before pairing"
//	@Router			/v1/auth/api_keys [post]
func CreateApiKeyHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.CreateApiKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		if !session.IsWellFormedChallengeCode(req.Code) {
			writePairingError(c, util.ErrBadInput("enter the four-digit code exactly as shown in Anytype Desktop"))
			return
		}
		result, err := s.CreateApiKey(c.Request.Context(), req.ChallengeId, req.Code)
		if err != nil {
			writePairingError(c, err)
			return
		}

		c.JSON(http.StatusCreated, result)
	}
}

func writePairingError(c *gin.Context, err error) {
	var status int
	var code string
	switch {
	case errors.Is(err, service.ErrMissingAppName), errors.Is(err, util.ErrBad):
		status, code = http.StatusBadRequest, "bad_request"
	case errors.Is(err, service.ErrPairingRateLimited):
		status, code = http.StatusTooManyRequests, "pairing_rate_limited"
	case errors.Is(err, service.ErrAccountNotRunning):
		status, code = http.StatusServiceUnavailable, "account_not_running"
	case errors.Is(err, service.ErrInvalidChallengeID):
		status, code = http.StatusNotFound, "invalid_challenge_id"
	case errors.Is(err, service.ErrChallengeNotApproved):
		status, code = http.StatusConflict, "challenge_not_approved"
	case errors.Is(err, service.ErrIncorrectAnswer):
		status, code = http.StatusBadRequest, "incorrect_answer"
	case errors.Is(err, service.ErrChallengeAttemptsExceeded):
		status, code = http.StatusTooManyRequests, "challenge_attempts_exceeded"
	default:
		status, code = http.StatusInternalServerError, "internal_server_error"
	}
	c.JSON(status, apimodel.PairingError{Object: "error", Status: status, Code: code, Message: err.Error()})
}
