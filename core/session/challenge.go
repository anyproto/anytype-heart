package session

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/globalsign/mgo/bson"
	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	challengeMaxTries     = 5
	challengeDigits       = 4 // 0000 - 9999
	maxChallengesRequests = 50
	// maxFailedChallengeSolves caps wrong code verifications per app run. Once
	// reached, the local-link flow is locked until the process restarts, so an
	// attacker cannot keep requesting fresh codes to brute-force the 4-digit
	// space (20 wrong guesses over 10^4 possibilities ≈ 0.2%). A correct
	// verification resets the count, so a user who mistypes a code — or pairs
	// several apps over a long session — is never locked out.
	maxFailedChallengeSolves = 20
)

var (
	ErrChallengeTriesExceeded    = fmt.Errorf("challenge tries exceeded")
	ErrChallengeIdNotFound       = fmt.Errorf("challenge id not found")
	ErrChallengeSolutionWrong    = fmt.Errorf("challenge solution is wrong")
	ErrInvalidScope              = fmt.Errorf("invalid scope")
	ErrTooManyChallengeRequests  = fmt.Errorf("too many challenge requests per session")
	ErrChallengeAttemptsExceeded = fmt.Errorf("too many failed challenge attempts, restart the app to try again")
	currentChallengesRequests    = atomic.NewInt32(0)
	// failedChallengeSolves counts wrong verifications for the whole process
	// run; it is reset only by a correct verification, never decremented.
	failedChallengeSolves = atomic.NewInt32(0)
)

func (s *service) StartNewChallenge(scope model.AccountAuthLocalApiScope, info *pb.EventAccountLinkChallengeClientInfo, requestedGrant *model.AccountAuthAppGrant) (challengeId string, challengeValue string, err error) {
	if failedChallengeSolves.Load() >= maxFailedChallengeSolves {
		// Locked after too many wrong guesses: refuse to hand out fresh codes.
		return "", "", ErrChallengeAttemptsExceeded
	}
	if currentChallengesRequests.Load() >= maxChallengesRequests {
		return "", "", ErrTooManyChallengeRequests
	}
	switch scope {
	case model.AccountAuth_Limited, model.AccountAuth_JsonAPI:
		// full scope is not allowed via challenge
	default:
		return "", "", ErrInvalidScope
	}
	// generate random challenge id
	id := bson.NewObjectId().Hex()
	s.lock.Lock()
	defer s.lock.Unlock()
	// generate random challenge value
	value := fmt.Sprintf("%0*d", challengeDigits, rand.Intn(int(math.Pow10(challengeDigits))))

	s.challenges[id] = challenge{
		tries:          0,
		value:          value,
		clientInfo:     info,
		scope:          scope,
		requestedGrant: requestedGrant,
	}

	currentChallengesRequests.Inc()
	return id, value, nil
}

// SolveChallenge verifies the 4-digit answer and, on success, mints a session
// carrying the scope — and hands back the requested grant — the challenge was
// created with. Unnamed results on purpose: a named `scope` return once
// shadowed `challenge.scope` here and the pairing session was silently minted
// with the zero value (Limited) — see the H1 hole in
// docs/ApiKeyScopingResearch.md §2.2.
func (s *service) SolveChallenge(challengeId string, challengeSolution string, signingKey []byte) (*pb.EventAccountLinkChallengeClientInfo, string, model.AccountAuthLocalApiScope, *model.AccountAuthAppGrant, error) {
	if failedChallengeSolves.Load() >= maxFailedChallengeSolves {
		return nil, "", 0, nil, ErrChallengeAttemptsExceeded
	}

	s.lock.Lock()
	challenge, ok := s.challenges[challengeId]
	if !ok {
		s.lock.Unlock()

		return nil, "", 0, nil, ErrChallengeIdNotFound
	}
	if challenge.tries >= challengeMaxTries {
		s.lock.Unlock()

		return nil, "", 0, nil, ErrChallengeTriesExceeded
	}

	if challenge.value != challengeSolution {

		challenge.tries++
		s.challenges[challengeId] = challenge
		s.lock.Unlock()
		// Count the wrong guess against the whole-run budget, not just this
		// challenge, so cycling through fresh codes cannot widen the window.
		failedChallengeSolves.Inc()
		return nil, "", 0, nil, ErrChallengeSolutionWrong
	}

	delete(s.challenges, challengeId)
	s.lock.Unlock()

	sessionToken, err := s.StartSession(signingKey, challenge.scope)
	if err != nil {
		return nil, "", 0, nil, fmt.Errorf("start session for solved challenge: %w", err)
	}
	// A correct verification clears the run's failure and request budgets, so a
	// user who mistyped earlier or pairs another device keeps working.
	failedChallengeSolves.Store(0)
	currentChallengesRequests.Store(0)
	return challenge.clientInfo, sessionToken, challenge.scope, challenge.requestedGrant, nil
}

type challenge struct {
	tries      int
	value      string
	clientInfo *pb.EventAccountLinkChallengeClientInfo
	scope      model.AccountAuthLocalApiScope
	// requestedGrant is the restriction the pairing client asked for; it is
	// persisted verbatim on solve. Grants only narrow, so honoring a
	// self-requested restriction is fail-safe by monotonicity.
	requestedGrant *model.AccountAuthAppGrant
}
