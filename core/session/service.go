package session

import (
	"fmt"
	"math/rand"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/golang-jwt/jwt/v4"
)

const CName = "session"

type Service interface {
	app.Component

	StartSession(token string) error
	CloseSession(token string) error
}

type service struct {
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) StartSession(token string) error {
	//TODO implement me
	panic("implement me")
}

func (s *service) CloseSession(token string) error {
	//TODO implement me
	panic("implement me")
}

func GenerateToken(privKey []byte) (string, error) {
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

func ValidateToken(privKey []byte, rawToken string) error {
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
