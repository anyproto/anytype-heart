package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var signingKey = []byte("test-signing-key")

// resetChallengeCounters clears the process-wide budgets so each test starts
// from a known state (they are package globals shared across the run).
func resetChallengeCounters() {
	failedChallengeSolves.Store(0)
	currentChallengesRequests.Store(0)
}

func newService(t *testing.T) *service {
	t.Helper()
	resetChallengeCounters()
	return New().(*service)
}

func browser(origin string) *pb.EventAccountLinkApprovalRequestClientInfo {
	return &pb.EventAccountLinkApprovalRequestClientInfo{Origin: origin}
}

// approved runs the whole request-then-approve handshake and returns the
// challenge id with the code the user would read off the screen.
func approved(t *testing.T, s *service, info *pb.EventAccountLinkApprovalRequestClientInfo) (id, code string) {
	t.Helper()
	id, err := s.StartNewChallenge(model.AccountAuth_Limited, info)
	require.NoError(t, err)
	code, _, err = s.ApproveChallenge(info.GetProcessPath(), info.GetOrigin(), true)
	require.NoError(t, err)
	require.Len(t, code, challengeDigits)
	return id, code
}

func wrongAnswer(code string) string {
	if code == "0000" {
		return "1111"
	}
	return "0000"
}

func TestStartNewChallenge_MintsNothingBeforeApproval(t *testing.T) {
	// given a challenge nobody has approved
	s := newService(t)
	info := browser("chrome-extension://pending")
	id, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
	require.NoError(t, err)

	// then it holds no code at all
	s.lock.RLock()
	stored := s.challenges[id]
	s.lock.RUnlock()
	assert.Equal(t, challengePending, stored.state)
	assert.Empty(t, stored.value)

	// ...and no answer in the whole 4-digit space unlocks it, which is what
	// "there is nothing to brute-force" has to mean concretely.
	for guess := 0; guess < 10000; guess++ {
		_, _, _, err = s.SolveChallenge(id, fmt.Sprintf("%04d", guess), signingKey)
		require.ErrorIs(t, err, ErrChallengeNotApproved)
	}

	// ...and none of that counted as a wrong guess, or a caller could lock
	// pairing for everyone by solving its own unapproved challenge.
	assert.Equal(t, int32(0), failedChallengeSolves.Load())
}

func TestApproveChallenge_MintsTheCode(t *testing.T) {
	// given
	s := newService(t)
	info := browser("chrome-extension://approved")

	// when
	id, code := approved(t, s, info)

	// then the code the user was shown is the one that pairs
	clientInfo, token, scope, err := s.SolveChallenge(id, code, signingKey)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, model.AccountAuth_Limited, scope)
	assert.Equal(t, info, clientInfo)
}

func TestApproveChallenge_NothingPending(t *testing.T) {
	t.Run("caller never asked", func(t *testing.T) {
		s := newService(t)

		code, info, err := s.ApproveChallenge("", "chrome-extension://stranger", true)

		assert.ErrorIs(t, err, ErrNoPendingChallenge)
		assert.Empty(t, code)
		assert.Nil(t, info)
	})

	t.Run("approving twice", func(t *testing.T) {
		s := newService(t)
		info := browser("chrome-extension://twice")
		approved(t, s, info)

		_, _, err := s.ApproveChallenge("", info.Origin, true)

		assert.ErrorIs(t, err, ErrNoPendingChallenge)
	})

	t.Run("a different caller cannot answer this prompt", func(t *testing.T) {
		s := newService(t)
		_, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, browser("chrome-extension://asked"))
		require.NoError(t, err)

		_, _, err = s.ApproveChallenge("", "chrome-extension://someone-else", true)

		assert.ErrorIs(t, err, ErrNoPendingChallenge)
	})
}

func TestApproveChallenge_DenyIsRemembered(t *testing.T) {
	t.Run("an attributable caller cannot ask again", func(t *testing.T) {
		// given a caller the user refused
		s := newService(t)
		info := browser("chrome-extension://denied")
		id, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
		require.NoError(t, err)

		// when
		code, hidden, err := s.ApproveChallenge("", info.Origin, false)

		// then the prompt is gone and the challenge with it
		require.NoError(t, err)
		assert.Empty(t, code)
		assert.Equal(t, info, hidden, "the caller must be reported so its prompt can be hidden")
		_, _, _, err = s.SolveChallenge(id, "0000", signingKey)
		assert.ErrorIs(t, err, ErrChallengeIdNotFound)

		// ...and it cannot make the user press Deny a second time
		_, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
		assert.ErrorIs(t, err, ErrChallengeDenied)

		// ...while everyone else is unaffected
		_, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, browser("chrome-extension://innocent"))
		assert.NoError(t, err)
	})

	t.Run("an unnameable caller is not remembered", func(t *testing.T) {
		// Callers with neither an origin nor a resolvable process share one
		// key, so remembering a denial there would silence all of them.
		s := newService(t)
		anonymous := &pb.EventAccountLinkApprovalRequestClientInfo{}
		_, err := s.StartNewChallenge(model.AccountAuth_Limited, anonymous)
		require.NoError(t, err)
		_, _, err = s.ApproveChallenge("", "", false)
		require.NoError(t, err)

		_, err = s.StartNewChallenge(model.AccountAuth_Limited, anonymous)

		assert.NoError(t, err)
	})
}

func TestStartNewChallenge_OnePromptPerCaller(t *testing.T) {
	// given a caller with a prompt already on screen
	s := newService(t)
	info := browser("chrome-extension://noisy")
	first, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
	require.NoError(t, err)

	// when it asks again
	second, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)

	// then it is refused, and crucially is not handed the pending id: callers
	// sharing a key would otherwise solve a challenge approved for someone else
	assert.ErrorIs(t, err, ErrChallengePendingApproval)
	assert.Empty(t, second)
	assert.NotEqual(t, first, second)
}

func TestStartNewChallenge_SupersedesOwnApprovedChallenge(t *testing.T) {
	// given a caller that was approved but never solved
	s := newService(t)
	info := browser("chrome-extension://forgetful")
	stale, staleCode := approved(t, s, info)

	// when it asks again, it gets a fresh request needing its own approval
	fresh, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
	require.NoError(t, err)
	assert.NotEqual(t, stale, fresh)

	// then the old code is dead rather than lingering as a second way in
	_, _, _, err = s.SolveChallenge(stale, staleCode, signingKey)
	assert.ErrorIs(t, err, ErrChallengeIdNotFound)
}

func TestSweepExpired(t *testing.T) {
	t.Run("an unanswered prompt expires", func(t *testing.T) {
		// given
		now := time.Now()
		s := newService(t)
		s.clock = func() time.Time { return now }
		info := browser("chrome-extension://ignored")
		id, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
		require.NoError(t, err)

		// when the user never answers
		now = now.Add(pendingChallengeTTL + time.Second)
		expired := s.SweepExpired()

		// then the prompt is reported so it can be taken off screen...
		require.Len(t, expired, 1)
		assert.Equal(t, info, expired[0])
		_, _, _, err = s.SolveChallenge(id, "0000", signingKey)
		assert.ErrorIs(t, err, ErrChallengeIdNotFound)

		// ...and the caller is free to ask again rather than stuck pending
		_, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
		assert.NoError(t, err)
	})

	t.Run("an approved code expires on its own longer clock", func(t *testing.T) {
		// given
		now := time.Now()
		s := newService(t)
		s.clock = func() time.Time { return now }
		id, code := approved(t, s, browser("chrome-extension://slow"))

		// when past the pending TTL but not the approved one
		now = now.Add(pendingChallengeTTL + time.Second)
		assert.Empty(t, s.SweepExpired())
		_, _, _, err := s.SolveChallenge(id, code, signingKey)
		assert.NoError(t, err, "a minted code must survive the pending TTL")

		// and when past the approved TTL
		id2, code2 := approved(t, s, browser("chrome-extension://slower"))
		now = now.Add(approvedChallengeTTL + time.Second)
		require.Len(t, s.SweepExpired(), 1)
		_, _, _, err = s.SolveChallenge(id2, code2, signingKey)
		assert.ErrorIs(t, err, ErrChallengeIdNotFound)
	})
}

func TestStartNewChallenge_PerCallerBudgetProtectsOtherClients(t *testing.T) {
	// given one caller spamming the pairing endpoint, approving each time so
	// the one-prompt-per-caller rule is not what stops it
	s := newService(t)
	noisy := browser("chrome-extension://noisy")
	for i := 0; i < maxChallengesRequestsPerCaller; i++ {
		approved(t, s, noisy)
	}

	// when it asks once more
	id, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, noisy)

	// then it is throttled...
	assert.ErrorIs(t, err, ErrTooManyCallerChallengeRequests)
	assert.Empty(t, id)

	// ...and everyone else can still pair, which is the whole point
	for _, other := range []*pb.EventAccountLinkApprovalRequestClientInfo{
		browser("chrome-extension://quiet"),
		browser("http://localhost:3000"),
		{ProcessPath: "/usr/local/bin/some-cli"},
		{}, // native client we cannot tell apart
	} {
		id, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, other)
		require.NoError(t, err)
		assert.NotEmpty(t, id)
	}
}

func TestStartNewChallenge_RunBudgetStillCapsAllCallers(t *testing.T) {
	// given requests spread over enough callers that no per-caller budget
	// trips, so the run-wide cap is what stops them
	s := newService(t)
	requests := 0
	for caller := 0; requests < maxChallengesRequests; caller++ {
		info := browser(fmt.Sprintf("chrome-extension://caller%d", caller))
		for i := 0; i < maxChallengesRequestsPerCaller && requests < maxChallengesRequests; i++ {
			approved(t, s, info)
			requests++
		}
	}

	// when
	_, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, browser("chrome-extension://latecomer"))

	// then
	assert.ErrorIs(t, err, ErrTooManyChallengeRequests)
}

func TestSolveChallenge_LocksAfterTooManyFailures(t *testing.T) {
	s := newService(t)

	// Burn the whole run's failure budget with wrong guesses. Each challenge
	// only allows challengeMaxTries, so keep asking for fresh ones — exactly
	// the cycling an attacker would do, except now every round costs an
	// approval the user has to grant.
	failures := 0
	for failures < maxFailedChallengeSolves {
		id, code := approved(t, s, browser(fmt.Sprintf("chrome-extension://round%d", failures)))
		wrong := wrongAnswer(code)
		for tries := 0; tries < challengeMaxTries && failures < maxFailedChallengeSolves; tries++ {
			_, _, _, err := s.SolveChallenge(id, wrong, signingKey)
			require.ErrorIs(t, err, ErrChallengeSolutionWrong)
			failures++
		}
	}

	// The run is now locked: no more solves, and no fresh challenges either.
	_, _, _, err := s.SolveChallenge("anything", "0000", signingKey)
	assert.ErrorIs(t, err, ErrChallengeAttemptsExceeded)

	lockedId, err := s.StartNewChallenge(model.AccountAuth_Limited, browser("chrome-extension://after"))
	assert.ErrorIs(t, err, ErrChallengeAttemptsExceeded)
	assert.Empty(t, lockedId)
}

func TestSolveChallenge_SuccessResetsBudgets(t *testing.T) {
	s := newService(t)

	// given one shy of the lock
	failures := 0
	for failures < maxFailedChallengeSolves-1 {
		id, code := approved(t, s, browser(fmt.Sprintf("chrome-extension://round%d", failures)))
		wrong := wrongAnswer(code)
		for tries := 0; tries < challengeMaxTries && failures < maxFailedChallengeSolves-1; tries++ {
			_, _, _, err := s.SolveChallenge(id, wrong, signingKey)
			require.ErrorIs(t, err, ErrChallengeSolutionWrong)
			failures++
		}
	}
	assert.Equal(t, int32(maxFailedChallengeSolves-1), failedChallengeSolves.Load())

	// when a correct verification lands
	okId, okCode := approved(t, s, browser("chrome-extension://good"))
	_, token, _, err := s.SolveChallenge(okId, okCode, signingKey)

	// then the budgets are clear and the flow keeps working
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, int32(0), failedChallengeSolves.Load())
	assert.Equal(t, int32(0), currentChallengesRequests.Load())

	nextId, nextCode := approved(t, s, browser("chrome-extension://next"))
	_, token, _, err = s.SolveChallenge(nextId, nextCode, signingKey)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestSolveChallenge_SuccessKeepsDenials(t *testing.T) {
	// A denial is the user's decision, not a rate limit, so pairing something
	// else must not quietly re-admit a refused caller.
	s := newService(t)
	denied := browser("chrome-extension://denied")
	_, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, denied)
	require.NoError(t, err)
	_, _, err = s.ApproveChallenge("", denied.Origin, false)
	require.NoError(t, err)

	id, code := approved(t, s, browser("chrome-extension://unrelated"))
	_, _, _, err = s.SolveChallenge(id, code, signingKey)
	require.NoError(t, err)

	_, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, denied)
	assert.ErrorIs(t, err, ErrChallengeDenied)
}

func TestSolveChallenge_PerChallengeTriesStillCapped(t *testing.T) {
	s := newService(t)
	id, code := approved(t, s, browser("chrome-extension://fumbling"))
	wrong := wrongAnswer(code)

	for i := 0; i < challengeMaxTries; i++ {
		_, _, _, err := s.SolveChallenge(id, wrong, signingKey)
		require.ErrorIs(t, err, ErrChallengeSolutionWrong)
	}

	// The 6th attempt on the same challenge is refused by the per-challenge cap.
	_, _, _, err := s.SolveChallenge(id, wrong, signingKey)
	assert.ErrorIs(t, err, ErrChallengeTriesExceeded)
}

func TestStartNewChallenge_RejectsFullScope(t *testing.T) {
	s := newService(t)

	_, err := s.StartNewChallenge(model.AccountAuth_Full, browser("chrome-extension://greedy"))

	assert.ErrorIs(t, err, ErrInvalidScope)
}
