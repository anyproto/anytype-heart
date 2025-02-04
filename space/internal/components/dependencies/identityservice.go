package dependencies

import (
	"context"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type IdentityService interface {
	GetMyProfileDetails(ctx context.Context) (identity string, metadataKey crypto.SymKey, details *domain.Details)

	RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observer func(identity string, profile *model.IdentityProfile)) error

	// UnregisterIdentity removes the observer for the identity in specified space
	UnregisterIdentity(spaceId string, identity string)
	// UnregisterIdentitiesInSpace removes all identity observers in the space
	UnregisterIdentitiesInSpace(spaceId string)

	WaitProfile(ctx context.Context, identity string) *model.IdentityProfile
}
