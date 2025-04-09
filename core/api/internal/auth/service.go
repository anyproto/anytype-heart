package auth

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrMissingAppName          = errors.New("missing app name")
	ErrFailedGenerateChallenge = errors.New("failed to generate a new challenge")
	ErrInvalidInput            = errors.New("invalid input")
	ErrFailedAuthenticate      = errors.New("failed to authenticate user")
)

type Service interface {
	NewChallenge(ctx context.Context, appName string) (string, error)
	SolveChallenge(ctx context.Context, challengeId string, code string) (sessionToken, appKey string, err error)
}

type service struct {
	mw apicore.ClientCommands
}

func NewService(mw apicore.ClientCommands) Service {
	return &service{mw: mw}
}

// NewChallenge calls AccountLocalLinkNewChallenge and returns the challenge ID, or an error if it fails.
func (s *service) NewChallenge(ctx context.Context, appName string) (string, error) {
	if appName == "" {
		return "", ErrMissingAppName
	}

	resp := s.mw.AccountLocalLinkNewChallenge(ctx, &pb.RpcAccountLocalLinkNewChallengeRequest{
		AppName: appName,
		Scope:   model.AccountAuth_JsonAPI,
	})

	if resp.Error.Code != pb.RpcAccountLocalLinkNewChallengeResponseError_NULL {
		return "", ErrFailedGenerateChallenge
	}

	return resp.ChallengeId, nil
}

// SolveChallenge calls AccountLocalLinkSolveChallenge and returns the session token + app key, or an error if it fails.
func (s *service) SolveChallenge(ctx context.Context, challengeId string, code string) (sessionToken string, appKey string, err error) {
	if challengeId == "" || code == "" {
		return "", "", ErrInvalidInput
	}

	// Call AccountLocalLinkSolveChallenge to retrieve session token and app key
	resp := s.mw.AccountLocalLinkSolveChallenge(ctx, &pb.RpcAccountLocalLinkSolveChallengeRequest{
		ChallengeId: challengeId,
		Answer:      code,
	})

	if resp.Error.Code != pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL {
		return "", "", ErrFailedAuthenticate
	}

	return resp.SessionToken, resp.AppKey, nil
}
