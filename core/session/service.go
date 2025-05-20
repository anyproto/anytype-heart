package session

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/golang-jwt/jwt"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "session"

type Service interface {
	StartSession(privKey []byte, scope model.AccountAuthLocalApiScope) (string, error)
	ValidateToken(privKey []byte, token string) (model.AccountAuthLocalApiScope, error)
	StartNewChallenge(scope model.AccountAuthLocalApiScope, info *pb.EventAccountLinkChallengeClientInfo, appName string) (id string, value string, err error)
	SolveChallenge(challengeId string, challengeSolution string, signingKey []byte) (clientInfo *pb.EventAccountLinkChallengeClientInfo, token string, scope model.AccountAuthLocalApiScope, err error)

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
}

func (s session) Scope() model.AccountAuthLocalApiScope {
	return s.scope
}

func New() Service {
	return &service{
		lock:       &sync.RWMutex{},
		sessions:   map[string]session{},
		challenges: map[string]challenge{},
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
