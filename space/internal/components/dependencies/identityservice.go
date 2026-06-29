package dependencies

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type IdentityService interface {
	GetMyProfileDetails(ctx context.Context) (identity string, metadataKey crypto.SymKey, details *domain.Details)

	// RegisterIdentity starts tracking the identity for the given space: its profile is
	// polled from the identity repo and fanned out into the space's participant record.
	// The encryption key is persisted; pass nil to reuse a previously persisted key.
	RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey) error

	// UnregisterIdentity stops tracking the identity for the specified space
	UnregisterIdentity(spaceId string, identity string)
	// UnregisterIdentitiesInSpace stops tracking all identities for the space
	UnregisterIdentitiesInSpace(spaceId string)

	WaitProfile(ctx context.Context, identity string) *model.IdentityProfile
	WaitProfileWithKey(ctx context.Context, identity string) (*model.IdentityProfileWithKey, error)
	GetMetadataKey(identity string) (crypto.SymKey, error)
	app.Component
}
