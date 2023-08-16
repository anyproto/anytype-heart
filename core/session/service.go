package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/golang-jwt/jwt"
	"sync"

	"github.com/anyproto/any-sync/app"
)

const CName = "session"

type Service interface {
	app.Component

	StartSession(privKey []byte) (string, error)
	ValidateToken(privKey []byte, token string) error
	CloseSession(token string) error
}

type service struct {
	lock     *sync.RWMutex
	sessions map[string]struct{}
}

func New() Service {
	return &service{
		lock:     &sync.RWMutex{},
		sessions: map[string]struct{}{},
	}
}

func (s *service) Init(a *app.App) (err error) {
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) StartSession(privKey []byte) (string, error) {
	token, err := generateToken(privKey)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.sessions[token]; ok {
		return "", fmt.Errorf("session is already started")
	}
	s.sessions[token] = struct{}{}
	return token, nil
}

func (s *service) ValidateToken(privKey []byte, token string) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if _, ok := s.sessions[token]; !ok {
		return fmt.Errorf("session is not registered")
	}

	return validateToken(privKey, token)
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
		"seed": randBytesInHex(8),
	})

	// Sign and get the complete encoded token as a string using the secret
	return token.SignedString(privKey)
}

// Return hexlify representation of a random byte[n]
func randBytesInHex(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
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
		return fmt.Errorf("parse token %s: %w", rawToken, err)
	}

	if token != nil && !token.Valid {
		return fmt.Errorf("token is invalid")
	}
	return nil
}
