package service

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrMissingAppName           = errors.New("missing app name")
	ErrFailedCreateNewChallenge = errors.New("failed to create a new challenge")
	ErrFailedAuthenticate       = errors.New("failed to authenticate user")
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

	if resp.Error != nil && resp.Error.Code != pb.RpcAccountLocalLinkNewChallengeResponseError_NULL {
		return "", ErrFailedCreateNewChallenge
	}

	return resp.ChallengeId, nil
}

// SolveChallenge calls AccountLocalLinkSolveChallenge and returns the session token + app key
func (s *Service) SolveChallenge(ctx context.Context, challengeId string, code string) (appKey string, err error) {
	if challengeId == "" || code == "" {
		return "", util.ErrBadInput("challenge_id or code is empty")
	}

	resp := s.mw.AccountLocalLinkSolveChallenge(ctx, &pb.RpcAccountLocalLinkSolveChallengeRequest{
		ChallengeId: challengeId,
		Answer:      code,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL {
		return "", ErrFailedAuthenticate
	}

	return resp.AppKey, nil
}
