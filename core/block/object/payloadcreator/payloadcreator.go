package payloadcreator

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// PayloadDerivationParams is a struct for deriving a payload
type PayloadDerivationParams struct {
	Key domain.UniqueKey

	// Derive object id using account private key.
	// It will activate anytype-heart's derivation mechanism, opposed to logic from any-sync
	UseAccountSignature bool
}

// PayloadCreationParams is a struct for creating a payload
type PayloadCreationParams struct {
	Time           time.Time
	SmartblockType coresb.SmartBlockType
}

type PayloadCreator interface {
	CreateTreePayload(ctx context.Context, params PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error)
	DeriveTreePayload(ctx context.Context, params PayloadDerivationParams) (storagePayload treestorage.TreeStorageCreatePayload, err error)
	DeriveObjectID(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error)
	DeriveObjectIdWithAccountSignature(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error)
}
