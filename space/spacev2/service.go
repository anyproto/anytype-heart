package spacev2

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// CName is intentionally distinct from the v1 "client.space" so both services can
// be registered side-by-side during migration. Rename to "client.space" at cutover.
const CName = "client.space.v2"

// errNotImplemented marks every method the clean-room implementation must fill in.
var errNotImplemented = errors.New("spacev2: not implemented")

// Service is the outward contract the rest of anytype-heart depends on. It mirrors
// the v1 space.Service surface verbatim so consumers can be cut over without churn
// (docs/SpaceController.md §11 "Keep"). You MAY redesign the internal controller
// contract (see controller.go), but keep THIS surface stable unless you also update
// every consumer and provide an adapter — §5.2/§5.3 list who depends on it.
type Service interface {
	// Create builds a new shareable space (and its SpaceView); OneToOne is routed
	// to CreateOneToOne. See docs/SpaceController.md §6.4.
	Create(ctx context.Context, description *spaceinfo.SpaceDescription) (space clientspace.Space, err error)
	CreateOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription, bobProfile *model.IdentityProfileWithKey) (sp clientspace.Space, err error)

	// Join/InviteJoin/CancelLeave drive AccountStatus transitions on the SpaceView.
	Join(ctx context.Context, id, aclHeadId string) error
	InviteJoin(ctx context.Context, id, aclHeadId string) error
	CancelLeave(ctx context.Context, id string) (err error)

	// Get returns a loaded space (errors if no controller exists / it cannot load
	// now). Wait blocks until the space appears (promotes a deferred/lazy space).
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

	// AccountMetadata* expose the SLIP-0021-derived profile key/payload. The
	// derivation MUST stay byte-identical to v1 (docs/SpaceController.md §8, §10).
	AccountMetadataSymKey() crypto.SymKey
	AccountMetadataPayload() []byte

	// PreloadRemainingSpaces releases the lazy-load backlog (idempotent).
	PreloadRemainingSpaces(ctx context.Context) error

	app.ComponentRunnable
}

// Error set the v1 consumers branch on. Preserve these (or map to them) so
// callers keep working (docs/SpaceController.md §5.1).
var (
	ErrIncorrectSpaceID   = errors.New("incorrect space id")
	ErrSpaceNotExists     = errors.New("space not exists")
	ErrSpaceStorageMissig = errors.New("space storage missing")
	ErrSpaceDeleted       = errors.New("space is deleted")
	ErrSpaceIsClosing     = errors.New("space is closing")
	ErrFailedToLoad       = errors.New("failed to load space")
)

// New returns the not-yet-implemented v2 service. Do NOT register it in
// core/anytype/bootstrap.go until it passes the parity tests (see HANDOFF.md).
func New() Service {
	return &service{}
}

type service struct {
	// TODO(spacev2): fields. docs/SpaceController.md §8 lists the required
	// dependencies (spaceCore, factory, accountService, config, subscription.Service,
	// notificationService, spaceNameGetter, inboxSender, identityService, aclJoiner,
	// coordinatorStatusUpdater) and §9 the concurrency invariants the state must honor.
}

// Compile-time assertion that the stub satisfies the target contract. Keep this
// as you implement so signature drift is caught at build time.
var _ Service = (*service)(nil)

func (s *service) Init(a *app.App) (err error)     { return errNotImplemented }
func (s *service) Name() (name string)             { return CName }
func (s *service) Run(ctx context.Context) error   { return errNotImplemented }
func (s *service) Close(ctx context.Context) error { return nil }

func (s *service) Create(ctx context.Context, description *spaceinfo.SpaceDescription) (clientspace.Space, error) {
	return nil, errNotImplemented
}

func (s *service) CreateOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription, bobProfile *model.IdentityProfileWithKey) (clientspace.Space, error) {
	return nil, errNotImplemented
}

func (s *service) Join(ctx context.Context, id, aclHeadId string) error { return errNotImplemented }
func (s *service) InviteJoin(ctx context.Context, id, aclHeadId string) error {
	return errNotImplemented
}
func (s *service) CancelLeave(ctx context.Context, id string) error { return errNotImplemented }

func (s *service) Get(ctx context.Context, id string) (clientspace.Space, error) {
	return nil, errNotImplemented
}

func (s *service) Wait(ctx context.Context, spaceId string) (clientspace.Space, error) {
	return nil, errNotImplemented
}

func (s *service) AddStreamable(ctx context.Context, id string, guestKey crypto.PrivKey) error {
	return errNotImplemented
}

func (s *service) Delete(ctx context.Context, id string) error { return errNotImplemented }

func (s *service) TechSpaceId() string               { return "" }
func (s *service) PersonalSpaceId() string           { return "" }
func (s *service) FirstCreatedSpaceId() string       { return "" }
func (s *service) TechSpace() *clientspace.TechSpace { return nil }

func (s *service) GetPersonalSpace(ctx context.Context) (clientspace.Space, error) {
	return nil, errNotImplemented
}

func (s *service) GetTechSpace(ctx context.Context) (clientspace.Space, error) {
	return nil, errNotImplemented
}

func (s *service) SpaceViewId(spaceId string) (string, error) { return "", errNotImplemented }

func (s *service) AccountMetadataSymKey() crypto.SymKey { return nil }
func (s *service) AccountMetadataPayload() []byte       { return nil }

func (s *service) PreloadRemainingSpaces(ctx context.Context) error { return errNotImplemented }
