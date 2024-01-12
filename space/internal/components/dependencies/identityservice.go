package dependencies

import (
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type IdentityService interface {
	RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observer func(identity string, profile *model.IdentityProfile)) error

	// UnregisterIdentity removes the observer for the identity in specified space
	UnregisterIdentity(spaceId string, identity string)
	// UnregisterIdentitiesInSpace removes all identity observers in the space
	UnregisterIdentitiesInSpace(spaceId string)
}
