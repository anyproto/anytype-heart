package session

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/globalsign/mgo/bson"
	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	challengeMaxTries = 5
	challengeDigits   = 4 // 0000 - 9999
	// maxChallengesRequests caps challenge requests per app run across all
	// callers. It exists so an attacker cannot cycle through fresh prompts.
	maxChallengesRequests = 30
	// maxChallengesRequestsPerCaller caps them per caller, so one browser
	// extension or script cannot spend the whole run's budget and lock every
	// other client out of pairing. Callers are told apart by origin, or by
	// process path for native clients; see callerKey.
	maxChallengesRequestsPerCaller = 10
	// maxFailedChallengeSolves caps wrong code verifications per app run. Once
	// reached, the local-link flow is locked until the process restarts, so an
	// attacker cannot keep requesting fresh codes to brute-force the 4-digit
	// space (20 wrong guesses over 10^4 possibilities ≈ 0.2%). A correct
	// verification resets the count, so a user who mistypes a code — or pairs
	// several apps over a long session — is never locked out.
	//
	// Since codes are only minted after approval, reaching this at all now
	// requires the user to have approved the caller doing the guessing.
	maxFailedChallengeSolves = 20
	// pendingChallengeTTL is how long an unanswered prompt stays live. Short:
	// the user is looking at it right now, or never will.
	pendingChallengeTTL = time.Minute
	// approvedChallengeTTL is how long a minted code stays usable, counted
	// from approval. Long enough to read four digits off a screen and type
	// them into another app.
	approvedChallengeTTL = 5 * time.Minute
)

var (
	ErrChallengeTriesExceeded    = fmt.Errorf("challenge tries exceeded")
	ErrChallengeIdNotFound       = fmt.Errorf("challenge id not found")
	ErrChallengeSolutionWrong    = fmt.Errorf("challenge solution is wrong")
	ErrInvalidScope              = fmt.Errorf("invalid scope")
	ErrTooManyChallengeRequests  = fmt.Errorf("too many challenge requests per session")
	ErrChallengeAttemptsExceeded = fmt.Errorf("too many failed challenge attempts, restart the app to try again")
	// ErrTooManyCallerChallengeRequests is the per-caller counterpart of
	// ErrTooManyChallengeRequests: this caller is throttled, other clients can
	// still pair.
	ErrTooManyCallerChallengeRequests = fmt.Errorf("too many challenge requests from this caller")
	// ErrChallengePendingApproval means this caller already has a prompt on
	// screen. Answering it is the way forward, not asking again.
	ErrChallengePendingApproval = fmt.Errorf("a challenge from this caller is already waiting for approval")
	// ErrChallengeDenied means the user said no to this caller during this app
	// run, so it is refused without raising another prompt.
	ErrChallengeDenied = fmt.Errorf("challenges from this caller were denied")
	// ErrNoPendingChallenge means there is nothing to approve for that caller:
	// never requested, already decided, or expired.
	ErrNoPendingChallenge = fmt.Errorf("no challenge is pending approval for this caller")
	// ErrChallengeNotApproved means the code does not exist yet because the
	// user has not approved the request. It is not a wrong answer, and must
	// not be counted as one.
	ErrChallengeNotApproved   = fmt.Errorf("challenge is waiting for user approval")
	currentChallengesRequests = atomic.NewInt32(0)
	// failedChallengeSolves counts wrong verifications for the whole process
	// run; it is reset only by a correct verification, never decremented.
	failedChallengeSolves = atomic.NewInt32(0)
)

// challengeState tracks whether a human has agreed to this pairing yet. A
// challenge carries no code until it is approved, so an unapproved request has
// no secret for an attacker to guess.
type challengeState int

const (
	challengePending challengeState = iota
	challengeApproved
)

type challenge struct {
	tries      int
	value      string // empty until approved
	clientInfo *pb.EventAccountLinkApprovalRequestClientInfo
	scope      model.AccountAuthLocalApiScope
	state      challengeState
	// stateSince is when the challenge entered its current state, and is what
	// the TTL is measured from.
	stateSince time.Time
}

// StartNewChallenge registers a pairing request and returns its id. No code is
// generated here: the request is pending until the user approves it through
// ApproveChallenge, which is the only place a code is minted.
func (s *service) StartNewChallenge(scope model.AccountAuthLocalApiScope, info *pb.EventAccountLinkApprovalRequestClientInfo) (challengeId string, err error) {
	switch scope {
	case model.AccountAuth_Limited, model.AccountAuth_JsonAPI:
		// full scope is not allowed via challenge
	default:
		return "", ErrInvalidScope
	}
	if failedChallengeSolves.Load() >= maxFailedChallengeSolves {
		// Locked after too many wrong guesses: refuse to hand out fresh codes.
		return "", ErrChallengeAttemptsExceeded
	}
	if currentChallengesRequests.Load() >= maxChallengesRequests {
		return "", ErrTooManyChallengeRequests
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	caller := callerKey(info)
	if isAttributable(caller) {
		if _, denied := s.deniedCallers[caller]; denied {
			// The user already said no. Refuse without prompting again.
			return "", ErrChallengeDenied
		}
	}
	if s.challengeRequestsByCaller[caller] >= maxChallengesRequestsPerCaller {
		return "", ErrTooManyCallerChallengeRequests
	}
	if _, pending := s.pendingByCaller[caller]; pending {
		// One prompt per caller. Deliberately not returning the pending id:
		// callers sharing a key — everything with neither an origin nor a
		// resolvable process shares the unattributable one — would then be
		// able to solve a challenge the user approved for someone else.
		return "", ErrChallengePendingApproval
	}
	// A caller asking again supersedes its own approved-but-unsolved
	// challenge; the new request needs its own approval.
	s.dropCallerChallengesLocked(caller)

	id := bson.NewObjectId().Hex()
	s.challenges[id] = challenge{
		clientInfo: info,
		scope:      scope,
		state:      challengePending,
		stateSince: s.now(),
	}
	s.pendingByCaller[caller] = id

	s.challengeRequestsByCaller[caller]++
	currentChallengesRequests.Inc()
	return id, nil
}

// ApproveChallenge records the user's decision on the challenge pending for a
// caller and, when allowed, mints the code and returns it. The code is only
// ever returned here, never broadcast, so it reaches nothing but the session
// that approved it.
//
// The caller is addressed by the process path and origin the prompt displayed.
// Only one challenge can be pending per caller, so the pair is unambiguous.
func (s *service) ApproveChallenge(processPath, origin string, allow bool) (value string, clientInfo *pb.EventAccountLinkApprovalRequestClientInfo, err error) {
	caller := callerKey(&pb.EventAccountLinkApprovalRequestClientInfo{ProcessPath: processPath, Origin: origin})

	s.lock.Lock()
	defer s.lock.Unlock()

	id, ok := s.pendingByCaller[caller]
	if !ok {
		return "", nil, ErrNoPendingChallenge
	}
	ch, ok := s.challenges[id]
	if !ok || ch.state != challengePending {
		// pendingByCaller outlived its challenge; heal and report honestly.
		delete(s.pendingByCaller, caller)
		return "", nil, ErrNoPendingChallenge
	}
	delete(s.pendingByCaller, caller)

	if !allow {
		delete(s.challenges, id)
		if isAttributable(caller) {
			// Remember the refusal so this caller cannot make the user press
			// Deny again for the rest of the run. Unattributable callers are
			// exempt: they share one key, so one denial would block every
			// native client.
			s.deniedCallers[caller] = struct{}{}
		}
		return "", ch.clientInfo, nil
	}

	ch.value = fmt.Sprintf("%0*d", challengeDigits, rand.Intn(int(math.Pow10(challengeDigits))))
	ch.state = challengeApproved
	ch.stateSince = s.now()
	s.challenges[id] = ch
	return ch.value, ch.clientInfo, nil
}

func (s *service) SolveChallenge(challengeId string, challengeSolution string, signingKey []byte) (clientInfo *pb.EventAccountLinkApprovalRequestClientInfo, token string, scope model.AccountAuthLocalApiScope, err error) {
	if failedChallengeSolves.Load() >= maxFailedChallengeSolves {
		return nil, "", 0, ErrChallengeAttemptsExceeded
	}

	s.lock.Lock()
	challenge, ok := s.challenges[challengeId]
	if !ok {
		s.lock.Unlock()

		return nil, "", 0, ErrChallengeIdNotFound
	}
	if challenge.state != challengeApproved {
		s.lock.Unlock()
		// Not a wrong guess: there is no code yet. Counting it would let a
		// caller lock pairing for everyone by solving its own unapproved
		// challenge maxFailedChallengeSolves times.
		return nil, "", 0, ErrChallengeNotApproved
	}
	if challenge.tries >= challengeMaxTries {
		s.lock.Unlock()

		return clientInfo, "", 0, ErrChallengeTriesExceeded
	}

	if challenge.value != challengeSolution {

		challenge.tries++
		s.challenges[challengeId] = challenge
		s.lock.Unlock()
		// Count the wrong guess against the whole-run budget, not just this
		// challenge, so cycling through fresh codes cannot widen the window.
		failedChallengeSolves.Inc()
		return clientInfo, "", 0, ErrChallengeSolutionWrong
	}

	delete(s.challenges, challengeId)
	// A correct verification clears the per-caller budgets too, so a client
	// that pairs successfully starts fresh next time. Denials are a user
	// decision and are deliberately kept.
	clear(s.challengeRequestsByCaller)
	s.lock.Unlock()

	sessionToken, err := s.StartSession(signingKey, scope)
	if err != nil {
		return nil, "", 0, err
	}
	// A correct verification clears the run's failure and request budgets, so a
	// user who mistyped earlier or pairs another device keeps working.
	failedChallengeSolves.Store(0)
	currentChallengesRequests.Store(0)
	return challenge.clientInfo, sessionToken, challenge.scope, nil
}

// SweepExpired drops challenges that outlived their TTL and returns the client
// info of each, so the caller can hide the prompts they left on screen. It is
// the only cleanup path for prompts nobody ever answered.
func (s *service) SweepExpired() []*pb.EventAccountLinkApprovalRequestClientInfo {
	s.lock.Lock()
	defer s.lock.Unlock()

	now := s.now()
	var expired []*pb.EventAccountLinkApprovalRequestClientInfo
	for id, ch := range s.challenges {
		ttl := approvedChallengeTTL
		if ch.state == challengePending {
			ttl = pendingChallengeTTL
		}
		if now.Sub(ch.stateSince) < ttl {
			continue
		}
		delete(s.challenges, id)
		caller := callerKey(ch.clientInfo)
		if pendingId, ok := s.pendingByCaller[caller]; ok && pendingId == id {
			delete(s.pendingByCaller, caller)
		}
		expired = append(expired, ch.clientInfo)
	}
	return expired
}

// dropCallerChallengesLocked removes any challenge this caller still holds.
// Callers must hold s.lock.
func (s *service) dropCallerChallengesLocked(caller string) {
	for id, ch := range s.challenges {
		if callerKey(ch.clientInfo) == caller {
			delete(s.challenges, id)
		}
	}
	delete(s.pendingByCaller, caller)
}

func (s *service) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

// callerKey names the client a challenge request came from, so per-caller
// budgets, the one-prompt-at-a-time rule and denials all apply to one client
// rather than being shared. The origin is set by the browser and cannot be
// chosen by the caller; the process path is the native equivalent. Clients we
// cannot tell apart share the empty bucket, which is the conservative
// direction: they throttle each other rather than nobody.
func callerKey(info *pb.EventAccountLinkApprovalRequestClientInfo) string {
	if info == nil {
		return ""
	}
	if info.Origin != "" {
		return "origin:" + info.Origin
	}
	if info.ProcessPath != "" {
		return "process:" + info.ProcessPath
	}
	return ""
}

// isAttributable reports whether a caller key names one specific client.
// Decisions that outlive a single request — denials — may only be remembered
// for these, or one bad client would speak for every client we cannot name.
func isAttributable(caller string) bool {
	return caller != ""
}
