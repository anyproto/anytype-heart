package session

/*
AI generated

Name: Local API Session Manager
Scope: global

## Responsibility
- JWT token-based session management with scoped permissions
- Challenge-response authentication for external API clients (4-digit numeric codes)
- Session context for request/response event correlation

## Documentation
Challenge flow: StartNewChallenge registers a pending request and returns its id,
minting nothing -> the client shows the user who is asking (process path and
browser origin) and the user approves -> ApproveChallenge mints the 4-digit code
and returns it to the approving session only, never on the event bus -> the user
types it into the requesting client, which calls SolveChallenge -> on success,
creates session with requested scope.

An unapproved request has no code associated with it, so there is nothing to
brute-force before a human has named and accepted the caller. The remaining
limits guard the user's attention rather than the secret: one pending prompt per
caller, denials remembered for the app run, 5 tries per approved challenge, 30
challenge requests per run and 10 per caller, and a run-wide cap of 20 wrong
verifications. A correct verification resets the counters; denials survive it.
Prompts nobody answers expire after a minute, codes five minutes after approval;
SweepExpired drops them and reports which prompts to hide.

Callers are told apart by origin, else process path — see callerKey. Everything
we cannot name shares one bucket and is excluded from decisions that outlive a
request.

Full scope (AccountAuth_Full) not available via challenge - only Limited and JsonAPI.
*/

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/golang-jwt/jwt"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "session"

type Service interface {
	StartSession(privKey []byte, scope model.AccountAuthLocalApiScope) (string, error)
	ValidateToken(privKey []byte, token string) (model.AccountAuthLocalApiScope, error)
	StartNewChallenge(scope model.AccountAuthLocalApiScope, info *pb.EventAccountLinkApprovalRequestClientInfo) (id string, err error)
	ApproveChallenge(processPath string, origin string, allow bool) (value string, clientInfo *pb.EventAccountLinkApprovalRequestClientInfo, err error)
	SolveChallenge(challengeId string, challengeSolution string, signingKey []byte) (clientInfo *pb.EventAccountLinkApprovalRequestClientInfo, token string, scope model.AccountAuthLocalApiScope, err error)
	SweepExpired() []*pb.EventAccountLinkApprovalRequestClientInfo

	CloseSession(token string) error
}

type session struct {
	token string
	scope model.AccountAuthLocalApiScope
}

type service struct {
	lock       *sync.RWMutex
	sessions   map[string]session
	challenges map[string]challenge
	// challengeRequestsByCaller counts challenge requests per caller for the
	// app run, so one client cannot spend the whole run's budget.
	challengeRequestsByCaller map[string]int
	// pendingByCaller points a caller at its unanswered prompt, enforcing one
	// prompt per caller and making a pending challenge addressable by the
	// caller the prompt displayed.
	pendingByCaller map[string]string
	// deniedCallers remembers, for the app run, callers the user refused, so
	// they cannot make the user press Deny repeatedly. Attributable callers
	// only — see isAttributable.
	deniedCallers map[string]struct{}
	// clock is injectable so TTL expiry is testable; nil means time.Now.
	clock func() time.Time
}

func (s session) Scope() model.AccountAuthLocalApiScope {
	return s.scope
}

func New() Service {
	return &service{
		lock:                      &sync.RWMutex{},
		sessions:                  map[string]session{},
		challenges:                map[string]challenge{},
		challengeRequestsByCaller: map[string]int{},
		pendingByCaller:           map[string]string{},
		deniedCallers:             map[string]struct{}{},
	}
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) StartSession(privKey []byte, scope model.AccountAuthLocalApiScope) (string, error) {
	if _, scopeExists := model.AccountAuthLocalApiScope_name[int32(scope)]; !scopeExists {
		return "", ErrInvalidScope
	}

	token, err := generateToken(privKey)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.sessions[token]; ok {
		return "", fmt.Errorf("session is already started")
	}
	s.sessions[token] = session{
		token: token,
		scope: scope,
	}
	return token, nil
}

type scopeGetter interface {
	Scope() model.AccountAuthLocalApiScope
}

func (s *service) ValidateToken(privKey []byte, token string) (model.AccountAuthLocalApiScope, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	var (
		ok    bool
		scope scopeGetter
	)
	if scope, ok = s.sessions[token]; !ok {
		return 0, fmt.Errorf("session is not registered")
	}

	err := validateToken(privKey, token)
	if err != nil {
		return 0, err
	}

	return scope.Scope(), nil
}

func (s *service) CloseSession(token string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.sessions[token]; !ok {
		return fmt.Errorf("session is not started")
	}
	delete(s.sessions, token)
	return nil
}

func generateToken(privKey []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		// "expiresAt": time.Now().Add(10 * time.Minute).Unix(),
		"seed": randStringRunes(8),
	})

	// Sign and get the complete encoded token as a string using the secret
	return token.SignedString(privKey)
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func validateToken(privKey []byte, rawToken string) error {
	token, err := jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// hmacSampleSecret is a []byte containing your secret, e.g. []byte("my_secret_key")
		return privKey, nil
	})
	if err != nil {
		return fmt.Errorf("parse token: %w", err)
	}

	if token != nil && !token.Valid {
		return fmt.Errorf("token is invalid")
	}
	return nil
}
