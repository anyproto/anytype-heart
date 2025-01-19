package interfaces

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

// TokenValidator is an interface that exposes ValidateApiToken from core to avoid circular dependencies.
type TokenValidator interface {
	ValidateApiToken(token string) (model.AccountAuthLocalApiScope, error)
}
