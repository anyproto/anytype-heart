package auth

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
)

var (
	ErrFailedGenerateChallenge = errors.New("failed to generate a new challenge")
	ErrInvalidInput            = errors.New("invalid input")
	ErrFailedAuthenticate      = errors.New("failed to authenticate user")
)

type Service interface {
	GenerateNewChallenge(ctx context.Context, appName string) (string, error)
	SolveChallengeForToken(ctx context.Context, challengeID, code string) (sessionToken, appKey string, err error)
}

type AuthService struct {
	mw service.ClientCommandsServer
}

func NewService(mw service.ClientCommandsServer) *AuthService {
	return &AuthService{mw: mw}
}

// GenerateNewChallenge calls mw.AccountLocalLinkNewChallenge(...)
// and returns the challenge ID, or an error if it fails.
func (s *AuthService) GenerateNewChallenge(ctx context.Context, appName string) (string, error) {
	resp := s.mw.AccountLocalLinkNewChallenge(ctx, &pb.RpcAccountLocalLinkNewChallengeRequest{AppName: "api-test"})

	if resp.Error.Code != pb.RpcAccountLocalLinkNewChallengeResponseError_NULL {
		return "", ErrFailedGenerateChallenge
	}

	return resp.ChallengeId, nil
}

// SolveChallengeForToken calls mw.AccountLocalLinkSolveChallenge(...)
// and returns the session token + app key, or an error if it fails.
func (s *AuthService) SolveChallengeForToken(ctx context.Context, challengeID, code string) (sessionToken, appKey string, err error) {
	if challengeID == "" || code == "" {
		return "", "", ErrInvalidInput
	}

	// Call AccountLocalLinkSolveChallenge to retrieve session token and app key
	resp := s.mw.AccountLocalLinkSolveChallenge(ctx, &pb.RpcAccountLocalLinkSolveChallengeRequest{
		ChallengeId: challengeID,
		Answer:      code,
	})

	if resp.Error.Code != pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL {
		return "", "", ErrFailedAuthenticate
	}

	return resp.SessionToken, resp.AppKey, nil
}
