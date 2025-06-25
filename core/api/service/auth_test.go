package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	mockedAppName     = "api-test"
	mockedChallengeId = "mocked-challenge-id"
	mockedCode        = "mocked-mockedCode"
	mockedAppKey      = "mocked-app-key"
)

func TestAuthService_GenerateNewChallenge(t *testing.T) {
	t.Run("successful challenge creation", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("AccountLocalLinkNewChallenge", mock.Anything, &pb.RpcAccountLocalLinkNewChallengeRequest{
			AppName: mockedAppName,
			Scope:   model.AccountAuth_JsonAPI,
		}).
			Return(&pb.RpcAccountLocalLinkNewChallengeResponse{
				ChallengeId: mockedChallengeId,
				Error:       &pb.RpcAccountLocalLinkNewChallengeResponseError{Code: pb.RpcAccountLocalLinkNewChallengeResponseError_NULL},
			}).Once()

		// when
		challengeId, err := fx.service.CreateChallenge(ctx, mockedAppName)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedChallengeId, challengeId)
	})

	t.Run("bad request - missing app name", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// when
		challengeId, err := fx.service.CreateChallenge(ctx, "")

		// then
		require.Error(t, err)
		require.Equal(t, ErrMissingAppName, err)
		require.Empty(t, challengeId)
	})

	t.Run("failed challenge creation", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("AccountLocalLinkNewChallenge", mock.Anything, &pb.RpcAccountLocalLinkNewChallengeRequest{
			AppName: mockedAppName,
			Scope:   model.AccountAuth_JsonAPI,
		}).
			Return(&pb.RpcAccountLocalLinkNewChallengeResponse{
				Error: &pb.RpcAccountLocalLinkNewChallengeResponseError{Code: pb.RpcAccountLocalLinkNewChallengeResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		challengeId, err := fx.service.CreateChallenge(ctx, mockedAppName)

		// then
		require.Error(t, err)
		require.Equal(t, ErrFailedCreateNewChallenge, err)
		require.Empty(t, challengeId)
	})
}

func TestAuthService_SolveChallengeForToken(t *testing.T) {
	t.Run("successful token retrieval", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, &pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: mockedChallengeId,
			Answer:      mockedCode,
		}).
			Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{
				AppKey: mockedAppKey,
				Error:  &pb.RpcAccountLocalLinkSolveChallengeResponseError{Code: pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL},
			}).Once()

		// when
		appKey, err := fx.service.SolveChallenge(ctx, mockedChallengeId, mockedCode)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedAppKey, appKey)

	})

	t.Run("bad request - missing challenge id or code", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// when
		appKey, err := fx.service.SolveChallenge(ctx, "", "")

		// then
		require.Error(t, err)
		require.ErrorIs(t, err, util.ErrBad)
		require.Empty(t, appKey)
	})

	t.Run("failed token retrieval", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, &pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: mockedChallengeId,
			Answer:      mockedCode,
		}).
			Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{
				Error: &pb.RpcAccountLocalLinkSolveChallengeResponseError{Code: pb.RpcAccountLocalLinkSolveChallengeResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		appKey, err := fx.service.SolveChallenge(ctx, mockedChallengeId, mockedCode)

		// then
		require.Error(t, err)
		require.Equal(t, ErrFailedAuthenticate, err)
		require.Empty(t, appKey)
	})
}
