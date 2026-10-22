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

func protoGrant() *model.AccountAuthAppGrant {
	return &model.AccountAuthAppGrant{
		SpaceIds: []string{"space1", "space2"},
		Perm:     model.AccountAuthAppGrant_ReadWrite,
	}
}

// approved runs the whole request-then-approve handshake for a Limited
// (webclipper) challenge — a plain allow, no grant — and returns the
// challenge id with the code the user would read off the screen.
func approved(t *testing.T, s *service, info *pb.EventAccountLinkApprovalRequestClientInfo) (id, code string) {
	t.Helper()
	id, _, err := s.StartNewChallenge(model.AccountAuth_Limited, info)
	require.NoError(t, err)
	code, _, err = s.ApproveChallenge(info.GetProcessPath(), info.GetOrigin(), true, nil)
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
	id, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
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
		_, _, _, _, err = s.SolveChallenge(id, fmt.Sprintf("%04d", guess), signingKey)
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
	clientInfo, token, scope, _, err := s.SolveChallenge(id, code, signingKey)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, model.AccountAuth_Limited, scope)
	assert.Equal(t, info, clientInfo)
}

func TestApproveChallenge_NothingPending(t *testing.T) {
	t.Run("caller never asked", func(t *testing.T) {
		s := newService(t)

		code, info, err := s.ApproveChallenge("", "chrome-extension://stranger", true, nil)

		assert.ErrorIs(t, err, ErrNoPendingChallenge)
		assert.Empty(t, code)
		assert.Nil(t, info)
	})

	t.Run("approving twice", func(t *testing.T) {
		s := newService(t)
		info := browser("chrome-extension://twice")
		approved(t, s, info)

		_, _, err := s.ApproveChallenge("", info.Origin, true, nil)

		assert.ErrorIs(t, err, ErrNoPendingChallenge)
	})

	t.Run("a different caller cannot answer this prompt", func(t *testing.T) {
		s := newService(t)
		_, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, browser("chrome-extension://asked"))
		require.NoError(t, err)

		_, _, err = s.ApproveChallenge("", "chrome-extension://someone-else", true, nil)

		assert.ErrorIs(t, err, ErrNoPendingChallenge)
	})
}

func TestApproveChallenge_DenyIsRemembered(t *testing.T) {
	t.Run("an attributable caller cannot ask again", func(t *testing.T) {
		// given a caller the user refused
		s := newService(t)
		info := browser("chrome-extension://denied")
		id, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
		require.NoError(t, err)

		// when
		code, hidden, err := s.ApproveChallenge("", info.Origin, false, nil)

		// then the prompt is gone and the challenge with it
		require.NoError(t, err)
		assert.Empty(t, code)
		assert.Equal(t, info, hidden, "the caller must be reported so its prompt can be hidden")
		_, _, _, _, err = s.SolveChallenge(id, "0000", signingKey)
		assert.ErrorIs(t, err, ErrChallengeIdNotFound)

		// ...and it cannot make the user press Deny a second time
		_, _, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
		assert.ErrorIs(t, err, ErrChallengeDenied)

		// ...while everyone else is unaffected
		_, _, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, browser("chrome-extension://innocent"))
		assert.NoError(t, err)
	})

	t.Run("an unnameable caller is not remembered", func(t *testing.T) {
		// Callers with neither an origin nor a resolvable process share one
		// key, so remembering a denial there would silence all of them.
		s := newService(t)
		anonymous := &pb.EventAccountLinkApprovalRequestClientInfo{}
		_, _, err := s.StartNewChallenge(model.AccountAuth_Limited, anonymous)
		require.NoError(t, err)
		_, _, err = s.ApproveChallenge("", "", false, nil)
		require.NoError(t, err)

		_, _, err = s.StartNewChallenge(model.AccountAuth_Limited, anonymous)

		assert.NoError(t, err)
	})
}

func TestStartNewChallenge_OnePromptPerCaller(t *testing.T) {
	// given a caller with a prompt already on screen
	s := newService(t)
	info := browser("chrome-extension://noisy")
	first, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
	require.NoError(t, err)

	// when it asks again
	second, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)

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
	fresh, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
	require.NoError(t, err)
	assert.NotEqual(t, stale, fresh)

	// then the old code is dead rather than lingering as a second way in
	_, _, _, _, err = s.SolveChallenge(stale, staleCode, signingKey)
	assert.ErrorIs(t, err, ErrChallengeIdNotFound)
}

func TestSweepExpired(t *testing.T) {
	t.Run("an unanswered prompt expires", func(t *testing.T) {
		// given
		now := time.Now()
		s := newService(t)
		s.clock = func() time.Time { return now }
		info := browser("chrome-extension://ignored")
		id, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
		require.NoError(t, err)

		// when the user never answers
		now = now.Add(pendingChallengeTTL + time.Second)
		expired := s.SweepExpired()

		// then the prompt is reported so it can be taken off screen...
		require.Len(t, expired, 1)
		assert.Equal(t, info, expired[0])
		_, _, _, _, err = s.SolveChallenge(id, "0000", signingKey)
		assert.ErrorIs(t, err, ErrChallengeIdNotFound)

		// ...and the caller is free to ask again rather than stuck pending
		_, _, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, info)
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
		_, _, _, _, err := s.SolveChallenge(id, code, signingKey)
		assert.NoError(t, err, "a minted code must survive the pending TTL")

		// and when past the approved TTL
		id2, code2 := approved(t, s, browser("chrome-extension://slower"))
		now = now.Add(approvedChallengeTTL + time.Second)
		require.Len(t, s.SweepExpired(), 1)
		_, _, _, _, err = s.SolveChallenge(id2, code2, signingKey)
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
	id, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, noisy)

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
		id, _, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, other)
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
	_, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, browser("chrome-extension://latecomer"))

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
			_, _, _, _, err := s.SolveChallenge(id, wrong, signingKey)
			require.ErrorIs(t, err, ErrChallengeSolutionWrong)
			failures++
		}
	}

	// The run is now locked: no more solves, and no fresh challenges either.
	_, _, _, _, err := s.SolveChallenge("anything", "0000", signingKey)
	assert.ErrorIs(t, err, ErrChallengeAttemptsExceeded)

	lockedId, _, err := s.StartNewChallenge(model.AccountAuth_Limited, browser("chrome-extension://after"))
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
			_, _, _, _, err := s.SolveChallenge(id, wrong, signingKey)
			require.ErrorIs(t, err, ErrChallengeSolutionWrong)
			failures++
		}
	}
	assert.Equal(t, int32(maxFailedChallengeSolves-1), failedChallengeSolves.Load())

	// when a correct verification lands
	okId, okCode := approved(t, s, browser("chrome-extension://good"))
	_, token, _, _, err := s.SolveChallenge(okId, okCode, signingKey)

	// then the budgets are clear and the flow keeps working
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, int32(0), failedChallengeSolves.Load())
	assert.Equal(t, int32(0), currentChallengesRequests.Load())

	nextId, nextCode := approved(t, s, browser("chrome-extension://next"))
	_, token, _, _, err = s.SolveChallenge(nextId, nextCode, signingKey)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestSolveChallenge_SuccessKeepsDenials(t *testing.T) {
	// A denial is the user's decision, not a rate limit, so pairing something
	// else must not quietly re-admit a refused caller.
	s := newService(t)
	denied := browser("chrome-extension://denied")
	_, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, denied)
	require.NoError(t, err)
	_, _, err = s.ApproveChallenge("", denied.Origin, false, nil)
	require.NoError(t, err)

	id, code := approved(t, s, browser("chrome-extension://unrelated"))
	_, _, _, _, err = s.SolveChallenge(id, code, signingKey)
	require.NoError(t, err)

	_, _, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, denied)
	assert.ErrorIs(t, err, ErrChallengeDenied)
}

func TestSolveChallenge_PerChallengeTriesStillCapped(t *testing.T) {
	s := newService(t)
	id, code := approved(t, s, browser("chrome-extension://fumbling"))
	wrong := wrongAnswer(code)

	for i := 0; i < challengeMaxTries; i++ {
		_, _, _, _, err := s.SolveChallenge(id, wrong, signingKey)
		require.ErrorIs(t, err, ErrChallengeSolutionWrong)
	}

	// The 6th attempt on the same challenge is refused by the per-challenge cap.
	_, _, _, _, err := s.SolveChallenge(id, wrong, signingKey)
	assert.ErrorIs(t, err, ErrChallengeTriesExceeded)
}

func TestStartNewChallenge_RejectsFullScope(t *testing.T) {
	s := newService(t)

	_, _, err := s.StartNewChallenge(model.AccountAuth_Full, browser("chrome-extension://greedy"))

	assert.ErrorIs(t, err, ErrInvalidScope)
}

// TestSolveChallenge_MintsSessionWithChallengeScope guards P0.1 in
// docs/superpowers/specs/2026-08-06-api-key-scoping-design.md: the pairing
// session must carry the scope the challenge was created with, not the zero
// value (Limited). The JsonAPI case is the one that used to pass vacuously —
// the minted session silently held Limited gRPC privileges.
func TestSolveChallenge_MintsSessionWithChallengeScope(t *testing.T) {
	tests := []struct {
		name  string
		want  model.AccountAuthLocalApiScope
		grant *model.AccountAuthAppGrant
	}{
		{name: "JsonAPI challenge mints JsonAPI session", want: model.AccountAuth_JsonAPI, grant: protoGrant()},
		{name: "Limited challenge mints Limited session", want: model.AccountAuth_Limited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			s := newService(t)
			info := browser("chrome-extension://scoped")
			id, _, err := s.StartNewChallenge(tt.want, info)
			require.NoError(t, err)
			code, _, err := s.ApproveChallenge("", info.Origin, true, tt.grant)
			require.NoError(t, err)

			// when
			_, token, returnedScope, _, err := s.SolveChallenge(id, code, signingKey)

			// then
			require.NoError(t, err)
			require.NotEmpty(t, token)
			assert.Equal(t, tt.want, returnedScope)

			// the session registered for the token must carry the same scope:
			// this is what the gRPC Authorize interceptor consults on every call
			sessionScope, err := s.ValidateToken(signingKey, token)
			require.NoError(t, err)
			assert.Equal(t, tt.want, sessionScope)
		})
	}
}

// TestApproveChallenge_GrantRules pins the scope-dependent rules of an allow
// decision — and that a rejected grant leaves the challenge pending: the
// prompt stays answerable, nothing was minted, no state was burned.
func TestApproveChallenge_GrantRules(t *testing.T) {
	tests := []struct {
		name    string
		scope   model.AccountAuthLocalApiScope
		grant   *model.AccountAuthAppGrant
		fix     *model.AccountAuthAppGrant // a valid retry for the scope
		wantErr string
	}{
		{
			name:    "JsonAPI approval requires a grant",
			scope:   model.AccountAuth_JsonAPI,
			grant:   nil,
			fix:     protoGrant(),
			wantErr: "requires a grant",
		},
		{
			name:    "grant on a Limited challenge is refused",
			scope:   model.AccountAuth_Limited,
			grant:   protoGrant(),
			fix:     nil,
			wantErr: "grant requires JsonAPI scope",
		},
		{
			name:    "grant with no spaces is refused",
			scope:   model.AccountAuth_JsonAPI,
			grant:   &model.AccountAuthAppGrant{Perm: model.AccountAuthAppGrant_Read},
			fix:     protoGrant(),
			wantErr: "spaces must be non-empty",
		},
		{
			name:    "allSpaces beside an explicit list is refused",
			scope:   model.AccountAuth_JsonAPI,
			grant:   &model.AccountAuthAppGrant{AllSpaces: true, SpaceIds: []string{"s1"}, Perm: model.AccountAuthAppGrant_Read},
			fix:     protoGrant(),
			wantErr: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			s := newService(t)
			info := browser("chrome-extension://granting")
			_, _, err := s.StartNewChallenge(tt.scope, info)
			require.NoError(t, err)

			// when
			code, _, err := s.ApproveChallenge("", info.Origin, true, tt.grant)

			// then: refused, and no code exists
			require.ErrorContains(t, err, tt.wantErr)
			assert.Empty(t, code)

			// ...and the challenge is STILL pending — the same prompt can be
			// answered again with a valid decision
			code, _, err = s.ApproveChallenge("", info.Origin, true, tt.fix)
			require.NoError(t, err)
			assert.Len(t, code, challengeDigits)
		})
	}
}

// TestSolveChallenge_ReturnsApprovedGrant pins §7 of the picker design: the
// grant handed back at solve is exactly the one the USER approved. The solver
// has no vector to supply one — SolveChallenge.Request carries only the id
// and the answer — so the stored record is the only possible source.
func TestSolveChallenge_ReturnsApprovedGrant(t *testing.T) {
	tests := []struct {
		name  string
		scope model.AccountAuthLocalApiScope
		want  *model.AccountAuthAppGrant
	}{
		{
			name:  "space-listed grant comes back on solve",
			scope: model.AccountAuth_JsonAPI,
			want:  protoGrant(),
		},
		{
			name:  "allSpaces grant comes back on solve",
			scope: model.AccountAuth_JsonAPI,
			want:  &model.AccountAuthAppGrant{AllSpaces: true, Perm: model.AccountAuthAppGrant_ReadWrite},
		},
		{
			name:  "a Limited solve returns no grant",
			scope: model.AccountAuth_Limited,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			s := newService(t)
			info := browser("chrome-extension://granted")
			id, _, err := s.StartNewChallenge(tt.scope, info)
			require.NoError(t, err)
			code, _, err := s.ApproveChallenge("", info.Origin, true, tt.want)
			require.NoError(t, err)

			// when
			_, token, _, gotGrant, err := s.SolveChallenge(id, code, signingKey)

			// then
			require.NoError(t, err)
			require.NotEmpty(t, token)
			assert.Equal(t, tt.want, gotGrant)
		})
	}
}

// TestCallerKey_NormalizesOriginSpelling closes the bypass the 4-lens review
// found: Policy.AllowOrigin normalizes (lowercase, trailing slash) BEFORE
// admitting an origin, so several spellings are one admitted caller — but
// callerKey used the raw header, giving that caller a fresh bucket per
// spelling. Deny memory, the one-prompt-per-caller rule and the per-caller
// budget all key off this, so each was walked around by re-spelling.
func TestCallerKey_NormalizesOriginSpelling(t *testing.T) {
	canonical := callerKey(browser("chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf"))

	for _, respelling := range []string{
		"chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf/",
		"Chrome-Extension://JBNAMMHJIPLHPJFNCNLEJJJEJGHIMDKF",
		"  chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf  ",
	} {
		assert.Equal(t, canonical, callerKey(browser(respelling)),
			"respelling %q must not buy a second bucket", respelling)
	}
}

// TestDenyMemory_SurvivesOriginRespelling is the behavioral half: the user
// said no once, and saying it again must not be required per spelling.
func TestDenyMemory_SurvivesOriginRespelling(t *testing.T) {
	// given a denied caller
	s := newService(t)
	denied := browser("chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf")
	_, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, denied)
	require.NoError(t, err)
	_, _, err = s.ApproveChallenge("", denied.Origin, false, nil)
	require.NoError(t, err)

	// when it asks again under every spelling the policy admits as the same
	// origin, then it is still refused and raises no second prompt
	for _, respelling := range []string{
		"chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf",
		"chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf/",
		"Chrome-Extension://JBNAMMHJIPLHPJFNCNLEJJJEJGHIMDKF",
	} {
		_, _, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, browser(respelling))
		assert.ErrorIs(t, err, ErrChallengeDenied, "respelling %q evaded the denial", respelling)
	}
}

// TestSolveChallenge_ClearsOnlyTheSolversBudget closes P2 from the 4-lens
// review: a successful solve used to clear the whole per-caller map, so a
// caller throttled at its limit regained a full allowance the moment any
// unrelated app paired. The caps were really per-inter-success-interval.
func TestSolveChallenge_ClearsOnlyTheSolversBudget(t *testing.T) {
	// given a caller that has spent its per-caller budget
	s := newService(t)
	noisy := browser("chrome-extension://noisy")
	for i := 0; i < maxChallengesRequestsPerCaller; i++ {
		approved(t, s, noisy)
	}
	_, _, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, noisy)
	require.ErrorIs(t, err, ErrTooManyCallerChallengeRequests)

	// when an unrelated caller pairs successfully
	id, code := approved(t, s, browser("chrome-extension://unrelated"))
	_, _, _, _, err = s.SolveChallenge(id, code, signingKey)
	require.NoError(t, err)

	// then the throttled caller is still throttled
	_, _, err = s.StartNewChallenge(model.AccountAuth_JsonAPI, noisy)
	assert.ErrorIs(t, err, ErrTooManyCallerChallengeRequests,
		"an unrelated success must not refill this caller's budget")
}

// TestSolveChallenge_TriesExhaustedDropsTheChallenge closes P8c: burning the
// tries is terminal, so the entry must go and the prompt must be reportable —
// it used to linger with a dead code on display until the approved TTL swept
// it minutes later.
func TestSolveChallenge_TriesExhaustedDropsTheChallenge(t *testing.T) {
	// given an approved challenge whose tries are spent
	s := newService(t)
	info := browser("chrome-extension://spent")
	id, code := approved(t, s, info)
	for i := 0; i < challengeMaxTries; i++ {
		_, _, _, _, err := s.SolveChallenge(id, wrongAnswer(code), signingKey)
		require.ErrorIs(t, err, ErrChallengeSolutionWrong)
	}

	// when the next attempt lands
	clientInfo, _, _, _, err := s.SolveChallenge(id, wrongAnswer(code), signingKey)

	// then it is terminal, and it names the prompt to take off screen
	require.ErrorIs(t, err, ErrChallengeTriesExceeded)
	assert.Equal(t, info, clientInfo)

	// ...and the entry is gone rather than waiting for the sweep
	s.lock.RLock()
	_, still := s.challenges[id]
	s.lock.RUnlock()
	assert.False(t, still, "a spent challenge must not linger until the TTL")
}

// TestStartNewChallenge_SupersedingReportsTheStalePrompt closes P8b: a caller
// asking again destroys its own approved-but-unsolved challenge, and the code
// the user already approved silently stopped working with nothing taken off
// screen.
func TestStartNewChallenge_SupersedingReportsTheStalePrompt(t *testing.T) {
	// given a caller with an approved, unsolved code on display
	s := newService(t)
	info := browser("chrome-extension://again")
	staleId, _ := approved(t, s, info)

	// when it asks again
	_, superseded, err := s.StartNewChallenge(model.AccountAuth_JsonAPI, info)

	// then the killed prompt is reported so it can be dismissed
	require.NoError(t, err)
	require.Len(t, superseded, 1)
	assert.Equal(t, info, superseded[0])

	// ...and the old code is genuinely dead
	s.lock.RLock()
	_, still := s.challenges[staleId]
	s.lock.RUnlock()
	assert.False(t, still)
}
