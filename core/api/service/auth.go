package service

import (
	"context"
	"errors"
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrMissingAppName            = errors.New("missing app name")
	ErrFailedCreateNewChallenge  = errors.New("failed to create a new challenge")
	ErrFailedAuthenticate        = errors.New("failed to authenticate user")
	ErrPairingRateLimited        = errors.New("a new pairing request cannot be started. Stop requesting challenges and ask the user to check Anytype Desktop for a pairing prompt")
	ErrAccountNotRunning         = errors.New("open Anytype Desktop and sign in to the account before pairing")
	ErrInvalidChallengeID        = errors.New("the challenge ID is no longer valid. Start a new challenge if the user still wants to pair")
	ErrChallengeNotApproved      = errors.New("wait for the user to approve this challenge in Anytype Desktop, then submit the displayed code")
	ErrIncorrectAnswer           = errors.New("the code is incorrect. Ask the user to check the four-digit code displayed for this challenge")
	ErrChallengeAttemptsExceeded = errors.New("pairing attempts are exhausted. Stop submitting codes and ask the user to start a new pairing when ready")
)

// CreateChallenge calls AccountLocalLinkNewChallenge and returns the challenge ID
func (s *Service) CreateChallenge(ctx context.Context, appName string) (string, error) {
	if appName == "" {
		return "", ErrMissingAppName
	}

	resp := s.mw.AccountLocalLinkNewChallenge(ctx, &pb.RpcAccountLocalLinkNewChallengeRequest{
		AppName: appName,
		Scope:   model.AccountAuth_JsonAPI,
	})

	// Several reasons for refusing a new prompt intentionally share one RPC
	// code. Its description can distinguish them and must stay private.
	switch resp.GetError().GetCode() {
	case pb.RpcAccountLocalLinkNewChallengeResponseError_NULL:
		return resp.ChallengeId, nil
	case pb.RpcAccountLocalLinkNewChallengeResponseError_BAD_INPUT:
		return "", util.ErrBadInput(fmt.Sprintf("provide a nonempty app_name of at most %d bytes", domain.MaxIntegrationNameLen))
	case pb.RpcAccountLocalLinkNewChallengeResponseError_ACCOUNT_IS_NOT_RUNNING:
		return "", ErrAccountNotRunning
	case pb.RpcAccountLocalLinkNewChallengeResponseError_TOO_MANY_REQUESTS:
		return "", ErrPairingRateLimited
	default:
		return "", ErrFailedCreateNewChallenge
	}
}

// CreateApiKey returns the issued key and the user's persisted grant.
func (s *Service) CreateApiKey(ctx context.Context, challengeId string, code string) (apimodel.CreateApiKeyResponse, error) {
	if challengeId == "" || code == "" {
		return apimodel.CreateApiKeyResponse{}, util.ErrBadInput("challenge_id or code is empty")
	}

	resp := s.mw.AccountLocalLinkSolveChallenge(ctx, &pb.RpcAccountLocalLinkSolveChallengeRequest{
		ChallengeId: challengeId,
		Answer:      code,
	})

	switch resp.GetError().GetCode() {
	case pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL:
		// Return the key and grant only after a successful exchange.
	case pb.RpcAccountLocalLinkSolveChallengeResponseError_BAD_INPUT:
		return apimodel.CreateApiKeyResponse{}, util.ErrBadInput("provide the challenge_id and the four-digit code from Anytype Desktop")
	case pb.RpcAccountLocalLinkSolveChallengeResponseError_ACCOUNT_IS_NOT_RUNNING:
		return apimodel.CreateApiKeyResponse{}, ErrAccountNotRunning
	case pb.RpcAccountLocalLinkSolveChallengeResponseError_INVALID_CHALLENGE_ID:
		return apimodel.CreateApiKeyResponse{}, ErrInvalidChallengeID
	case pb.RpcAccountLocalLinkSolveChallengeResponseError_CHALLENGE_NOT_APPROVED:
		return apimodel.CreateApiKeyResponse{}, ErrChallengeNotApproved
	case pb.RpcAccountLocalLinkSolveChallengeResponseError_INCORRECT_ANSWER:
		return apimodel.CreateApiKeyResponse{}, ErrIncorrectAnswer
	case pb.RpcAccountLocalLinkSolveChallengeResponseError_CHALLENGE_ATTEMPTS_EXCEEDED:
		return apimodel.CreateApiKeyResponse{}, ErrChallengeAttemptsExceeded
	default:
		return apimodel.CreateApiKeyResponse{}, ErrFailedAuthenticate
	}

	result := apimodel.CreateApiKeyResponse{ApiKey: resp.AppKey}
	if grant := util.ApiGrantFromProto(resp.Grant); grant != nil {
		result.Grant = &apimodel.ApiKeyGrant{
			AllSpaces:  grant.AllSpaces,
			SpaceIds:   append([]string{}, grant.Spaces...),
			Permission: grant.Perms,
		}
	}
	return result, nil
}
