package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// resetChallengeCounters clears the process-wide budgets so each test starts
// from a known state (they are package globals shared across the run).
func resetChallengeCounters() {
	failedChallengeSolves.Store(0)
	currentChallengesRequests.Store(0)
}

func newChallenge(t *testing.T, s *service) (id, value string) {
	t.Helper()
	id, value, err := s.StartNewChallenge(model.AccountAuth_Limited, &pb.EventAccountLinkChallengeClientInfo{}, nil)
	require.NoError(t, err)
	return id, value
}

func TestSolveChallenge_LocksAfterTooManyFailures(t *testing.T) {
	resetChallengeCounters()
	s := New().(*service)
	key := []byte("test-signing-key")

	// Burn the whole run's failure budget with wrong guesses. Each challenge
	// only allows challengeMaxTries, so keep requesting fresh codes — exactly
	// the cycling an attacker would do.
	failures := 0
	for failures < maxFailedChallengeSolves {
		id, value := newChallenge(t, s)
		wrong := "0000"
		if value == wrong {
			wrong = "1111"
		}
		for tries := 0; tries < challengeMaxTries && failures < maxFailedChallengeSolves; tries++ {
			_, _, _, _, err := s.SolveChallenge(id, wrong, key)
			require.ErrorIs(t, err, ErrChallengeSolutionWrong)
			failures++
		}
	}

	// The run is now locked: no more solves, and no fresh codes either.
	_, _, _, _, err := s.SolveChallenge("anything", "0000", key)
	assert.ErrorIs(t, err, ErrChallengeAttemptsExceeded)

	lockedId, lockedValue, err := s.StartNewChallenge(model.AccountAuth_Limited, &pb.EventAccountLinkChallengeClientInfo{}, nil)
	assert.ErrorIs(t, err, ErrChallengeAttemptsExceeded)
	assert.Empty(t, lockedId)
	assert.Empty(t, lockedValue)
}

func TestSolveChallenge_SuccessResetsFailureBudget(t *testing.T) {
	resetChallengeCounters()
	s := New().(*service)
	key := []byte("test-signing-key")

	// One shy of the lock.
	id, value := newChallenge(t, s)
	wrong := "0000"
	if value == wrong {
		wrong = "1111"
	}
	for i := 0; i < maxFailedChallengeSolves-1; i++ {
		// fresh code every challengeMaxTries so we never trip the per-challenge cap
		if i > 0 && i%challengeMaxTries == 0 {
			id, value = newChallenge(t, s)
			wrong = "0000"
			if value == wrong {
				wrong = "1111"
			}
		}
		_, _, _, _, err := s.SolveChallenge(id, wrong, key)
		require.ErrorIs(t, err, ErrChallengeSolutionWrong)
	}
	assert.Equal(t, int32(maxFailedChallengeSolves-1), failedChallengeSolves.Load())

	// A correct verification clears the budget.
	okId, okValue := newChallenge(t, s)
	_, token, scope, _, err := s.SolveChallenge(okId, okValue, key)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, model.AccountAuth_Limited, scope)
	assert.Equal(t, int32(0), failedChallengeSolves.Load())

	// ...so the flow keeps working afterwards.
	nextId, nextValue := newChallenge(t, s)
	_, token, _, _, err = s.SolveChallenge(nextId, nextValue, key)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

// TestSolveChallenge_MintsSessionWithChallengeScope guards against P0.1 in
// docs/superpowers/specs/2026-08-06-api-key-scoping-design.md: the pairing
// session must carry the scope the challenge was created with, not the zero
// value (Limited). The JsonAPI case is the one that used to pass vacuously —
// the minted session silently held Limited gRPC privileges.
func TestSolveChallenge_MintsSessionWithChallengeScope(t *testing.T) {
	tests := []struct {
		name string
		want model.AccountAuthLocalApiScope
	}{
		{name: "JsonAPI challenge mints JsonAPI session", want: model.AccountAuth_JsonAPI},
		{name: "Limited challenge mints Limited session", want: model.AccountAuth_Limited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			resetChallengeCounters()
			s := New().(*service)
			key := []byte("test-signing-key")
			id, value, err := s.StartNewChallenge(tt.want, &pb.EventAccountLinkChallengeClientInfo{}, nil)
			require.NoError(t, err)

			// when
			_, token, returnedScope, _, err := s.SolveChallenge(id, value, key)

			// then
			require.NoError(t, err)
			require.NotEmpty(t, token)
			assert.Equal(t, tt.want, returnedScope)

			// the session registered for the token must carry the same scope:
			// this is what the gRPC Authorize interceptor consults on every call
			sessionScope, err := s.ValidateToken(key, token)
			require.NoError(t, err)
			assert.Equal(t, tt.want, sessionScope)
		})
	}
}

func TestSolveChallenge_PerChallengeTriesStillCapped(t *testing.T) {
	resetChallengeCounters()
	s := New().(*service)
	key := []byte("test-signing-key")

	id, value := newChallenge(t, s)
	wrong := "0000"
	if value == wrong {
		wrong = "1111"
	}
	for i := 0; i < challengeMaxTries; i++ {
		_, _, _, _, err := s.SolveChallenge(id, wrong, key)
		require.ErrorIs(t, err, ErrChallengeSolutionWrong)
	}
	// The 6th attempt on the same challenge is refused by the per-challenge cap.
	_, _, _, _, err := s.SolveChallenge(id, wrong, key)
	assert.ErrorIs(t, err, ErrChallengeTriesExceeded)
}

// TestSolveChallenge_CarriesRequestedGrant pins that the restriction a client
// asks for at pairing survives to the solve — the application layer persists
// exactly what comes back here, so a dropped grant would silently mint an
// UNSCOPED key for a client that asked for a scoped one.
func TestSolveChallenge_CarriesRequestedGrant(t *testing.T) {
	tests := []struct {
		name string
		want *model.AccountAuthAppGrant
	}{
		{
			name: "requested grant comes back on solve",
			want: &model.AccountAuthAppGrant{
				SpaceIds: []string{"space1", "space2"},
				Perm:     model.AccountAuthAppGrant_ReadWrite,
			},
		},
		{
			name: "no requested grant stays nil",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			resetChallengeCounters()
			s := New().(*service)
			key := []byte("test-signing-key")
			id, value, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, &pb.EventAccountLinkChallengeClientInfo{}, tt.want)
			require.NoError(t, err)

			// when
			_, token, _, gotGrant, err := s.SolveChallenge(id, value, key)

			// then
			require.NoError(t, err)
			require.NotEmpty(t, token)
			assert.Equal(t, tt.want, gotGrant)
		})
	}
}
