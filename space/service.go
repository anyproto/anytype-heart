// Package space is the outward face of the space subsystem: the Service
// interface consumers depend on, its error set, and the component
// registration. The orchestration implementation lives in space/spacev2
// (see its DESIGN.md); the domain layers it drives (clientspace, techspace,
// spacecore, spaceinfo, storage) live in their own subpackages, unchanged.
package space

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/spacev2"
)

const CName = spacev2.CName

// The documented error set callers branch on; aliased to the implementation's
// values so errors.Is works across both import paths.
var (
	ErrSpaceNotExists     = spacev2.ErrSpaceNotExists
	ErrSpaceStorageMissig = spacev2.ErrSpaceStorageMissing
	ErrSpaceDeleted       = spacev2.ErrSpaceDeleted
	ErrSpaceIsClosing     = spacev2.ErrSpaceIsClosing
	ErrFailedToLoad       = spacev2.ErrFailedToLoad
)

func New() Service {
	return spacev2.New()
}

type Service interface {
	Create(ctx context.Context, description *spaceinfo.SpaceDescription) (space clientspace.Space, err error)
	CreateOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription, bobProfile *model.IdentityProfileWithKey) (sp clientspace.Space, err error)
	Join(ctx context.Context, id, aclHeadId string) error
	InviteJoin(ctx context.Context, id, aclHeadId string) error
	CancelLeave(ctx context.Context, id string) (err error)
	Get(ctx context.Context, id string) (space clientspace.Space, err error)
	Wait(ctx context.Context, spaceId string) (sp clientspace.Space, err error)
	AddStreamable(ctx context.Context, id string, guestKey crypto.PrivKey) (err error)
	Delete(ctx context.Context, id string) (err error)
	TechSpaceId() string
	PersonalSpaceId() string
	FirstCreatedSpaceId() string
	TechSpace() *clientspace.TechSpace
	GetPersonalSpace(ctx context.Context) (space clientspace.Space, err error)
	GetTechSpace(ctx context.Context) (space clientspace.Space, err error)
	SpaceViewId(spaceId string) (spaceViewId string, err error)
	AccountMetadataSymKey() crypto.SymKey
	AccountMetadataPayload() []byte
	PreloadRemainingSpaces(ctx context.Context) error
	app.ComponentRunnable
}
