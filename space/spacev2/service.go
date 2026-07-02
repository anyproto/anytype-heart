package spacev2

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclobjectmanager"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacedomain"
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

// NotificationSender mirrors v1 space.NotificationSender (participant-remove
// notification on remote deletion, wired in M4).
type NotificationSender interface {
	app.Component
	CreateAndSend(notification *model.Notification) error
}

// techSpaceProvider isolates tech-space construction (v1: spacefactory
// CreateAndSetTechSpace / LoadAndSetTechSpace) behind a seam so the bootstrap
// fallbacks are testable (docs/SpaceController.md §11.7). The real
// implementation arrives with the tech provider (techprovider.go); tests
// pre-set a fake before Init.
type techSpaceProvider interface {
	Create(ctx context.Context) (*clientspace.TechSpace, error)
	Load(ctx context.Context) (*clientspace.TechSpace, error)
}

type service struct {
	registry  *registry
	watcher   *spaceWatcher
	spaceCore spacecore.SpaceCoreService

	accountService      accountservice.Service
	config              *config.Config
	notificationService NotificationSender
	spaceNameGetter     objectstore.SpaceNameGetter
	spaceLoaderListener aclobjectmanager.SpaceLoaderListener
	techProvider        techSpaceProvider

	techSpace      *clientspace.TechSpace
	techSpaceReady chan struct{}

	personalSpaceId        string
	techSpaceId            string
	newAccount             bool
	repKey                 uint64
	accountMetadataSymKey  crypto.SymKey
	accountMetadataPayload []byte
	firstCreatedSpaceId    string

	ctx       context.Context // long operations within the service lifecycle, excluding Run
	ctxCancel context.CancelFunc
	isClosing atomic.Bool
}

// Compile-time assertion that the stub satisfies the target contract. Keep this
// as you implement so signature drift is caught at build time.
var _ Service = (*service)(nil)

func (s *service) Init(a *app.App) (err error) {
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.config = app.MustComponent[*config.Config](a)
	s.newAccount = s.config.IsNewAccount()
	s.notificationService = app.MustComponent[NotificationSender](a)
	s.spaceNameGetter = app.MustComponent[objectstore.SpaceNameGetter](a)
	s.spaceLoaderListener = app.MustComponent[aclobjectmanager.SpaceLoaderListener](a)
	subService := app.MustComponent[subscription.Service](a)

	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeRegular)
	if err != nil {
		return fmt.Errorf("derive personal space id: %w", err)
	}
	s.techSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeTech)
	if err != nil {
		return fmt.Errorf("derive tech space id: %w", err)
	}
	// Byte-exact contract (docs/SpaceController.md §8): the payload is this
	// member's ACL metadata; the sym key encrypts the identity-repo profile.
	accountMetadata, metadataSymKey, err := domain.DeriveAccountMetadata(s.accountService.Account().SignKey)
	if err != nil {
		return fmt.Errorf("derive account metadata: %w", err)
	}
	s.accountMetadataSymKey = metadataSymKey
	s.accountMetadataPayload, err = accountMetadata.Marshal()
	if err != nil {
		return fmt.Errorf("marshal account metadata: %w", err)
	}
	s.repKey, err = getRepKey(s.personalSpaceId)
	if err != nil {
		return fmt.Errorf("get replication key: %w", err)
	}

	s.registry = newRegistry()
	s.techSpaceReady = make(chan struct{})
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.watcher = newSpaceWatcher(s.techSpaceId, subService, s)
	return nil
}

func (s *service) Name() (name string) { return CName }

// Run is wired in the M2 Run/Close task; nil keeps test apps startable.
func (s *service) Run(ctx context.Context) error   { return nil }
func (s *service) Close(ctx context.Context) error { return nil }

// onSpaceStatusUpdated is the unidirectional apply path (statusApplier);
// completed in the M2 Run/Close task.
func (s *service) onSpaceStatusUpdated(status spaceViewStatus) {}

func getRepKey(spaceId string) (uint64, error) {
	sepIdx := strings.Index(spaceId, ".")
	if sepIdx == -1 {
		return 0, fmt.Errorf("space id is incorrect")
	}
	return strconv.ParseUint(spaceId[sepIdx+1:], 36, 64)
}

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

func (s *service) TechSpaceId() string               { return s.techSpaceId }
func (s *service) PersonalSpaceId() string           { return s.personalSpaceId }
func (s *service) FirstCreatedSpaceId() string       { return s.firstCreatedSpaceId }
func (s *service) TechSpace() *clientspace.TechSpace { return s.techSpace }

func (s *service) GetPersonalSpace(ctx context.Context) (clientspace.Space, error) {
	return nil, errNotImplemented
}

func (s *service) GetTechSpace(ctx context.Context) (clientspace.Space, error) {
	return nil, errNotImplemented
}

func (s *service) SpaceViewId(spaceId string) (string, error) { return "", errNotImplemented }

func (s *service) AccountMetadataSymKey() crypto.SymKey { return s.accountMetadataSymKey }
func (s *service) AccountMetadataPayload() []byte       { return s.accountMetadataPayload }

func (s *service) PreloadRemainingSpaces(ctx context.Context) error { return errNotImplemented }
