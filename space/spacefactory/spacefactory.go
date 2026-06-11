package spacefactory

/*
AI generated

Name: Space Controller Factory
Scope: global

## Responsibility
- Creates and initializes SpaceController instances for all space types (personal, shareable, streamable, marketplace, one-to-one)
- Creates and initializes TechSpace (special space storing SpaceViews for all other spaces)
- Handles SpaceView creation in TechSpace when creating new spaces

## Documentation
Space types and their controllers:
- Personal: user's primary space, derived deterministically from account
- Shareable: spaces that can be shared with others (inviting, active states)
- Streamable: spaces accessed via private key (e.g., for streaming/guest access)
- Marketplace: virtual space for bundled types/relations (deprecated)
- OneToOne: direct messaging spaces between two participants

Create vs New methods:
- Create*: derives/creates underlying storage, creates SpaceView in TechSpace, then creates controller
- New*: loads existing space (SpaceView already exists), creates controller only
*/

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/accountspace"
	dependencies "github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/personalmigration"
	"github.com/anyproto/anytype-heart/space/internal/marketplacespace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// SpaceFactory constructs space controllers (New*) and creates the durable
// artifacts of new spaces — storage marks and space views (Create*).
// Controllers are registered exclusively through space view events, so the
// Create* methods do not build controllers; the watcher does.
type SpaceFactory interface {
	app.Component
	NewPersonalSpace(ctx context.Context, metadata []byte) (spacecontroller.SpaceController, error)
	NewShareableSpace(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo) (spacecontroller.SpaceController, error)
	NewStreamableSpace(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo, metadata []byte) (spacecontroller.SpaceController, error)
	CreateShareableSpace(ctx context.Context, id string, desc *spaceinfo.SpaceDescription) error
	CreateStreamableSpace(ctx context.Context, privKey crypto.PrivKey, id string) error
	CreateOneToOneSpace(ctx context.Context, id string, description *spaceinfo.SpaceDescription, participantData spaceinfo.OneToOneParticipantData) error
	CreateMarketplaceSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error)
	CreateAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error)
	LoadAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error)
}

const CName = "client.space.spacefactory"

type ctxKey int

// SkipCheckSpaceViewKey, when set to true in ctx, makes NewPersonalSpace create
// the personal space view unconditionally instead of probing for it first.
// Used when restoring old accounts that never had a space view.
const SkipCheckSpaceViewKey ctxKey = iota

func shouldCheckSpaceView(ctx context.Context) bool {
	skip, ok := ctx.Value(SkipCheckSpaceViewKey).(bool)
	return !ok || !skip
}

type spaceFactory struct {
	app             *app.App
	spaceCore       spacecore.SpaceCoreService
	techSpace       techspace.TechSpace
	accountService  accountservice.Service
	objectFactory   objectcache.ObjectFactory
	indexer         dependencies.SpaceIndexer
	installer       dependencies.BundledObjectsInstaller
	storageService  storage.ClientStorage
	personalSpaceId string
}

func New() SpaceFactory {
	return &spaceFactory{}
}

func (s *spaceFactory) Init(a *app.App) (err error) {
	s.app = a
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.objectFactory = app.MustComponent[objectcache.ObjectFactory](a)
	s.indexer = app.MustComponent[dependencies.SpaceIndexer](a)
	s.installer = app.MustComponent[dependencies.BundledObjectsInstaller](a)
	s.storageService = app.MustComponent[storage.ClientStorage](a)
	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeRegular)
	if err != nil {
		return
	}
	return
}

func (s *spaceFactory) NewPersonalSpace(ctx context.Context, metadata []byte) (ctrl spacecontroller.SpaceController, err error) {
	id, err := s.spaceCore.DeriveID(ctx, spacedomain.SpaceTypeRegular)
	if err != nil {
		return nil, err
	}
	if err = s.ensurePersonalSpaceView(ctx, id); err != nil {
		return nil, err
	}
	return s.newPersonalController(id, metadata)
}

// ensurePersonalSpaceView creates the personal space view if it is missing.
// Old accounts predate space views, so the view may need to be created on
// first start; the spacestatus component requires it to exist.
func (s *spaceFactory) ensurePersonalSpaceView(ctx context.Context, id string) error {
	var (
		exists bool
		err    error
	)
	if shouldCheckSpaceView(ctx) {
		exists, err = s.techSpace.SpaceViewExists(ctx, id)
	}
	if !exists || err != nil {
		info := spaceinfo.NewSpacePersistentInfo(id)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
		if err := s.techSpace.SpaceViewCreate(ctx, id, false, info, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *spaceFactory) newPersonalController(id string, metadata []byte) (spacecontroller.SpaceController, error) {
	return accountspace.NewSpaceController(accountspace.Descriptor{
		SpaceId:       id,
		IsPersonal:    true,
		OwnerMetadata: metadata,
		ExtraLoaderComponents: func() []app.Component {
			return []app.Component{personalmigration.New()}
		},
	}, s.app)
}

func (s *spaceFactory) CreateAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	techSpace := techspace.New()
	techCoreSpace, err := s.spaceCore.Derive(ctx, spacedomain.SpaceTypeTech)
	if err != nil {
		return nil, fmt.Errorf("derive tech space: %w", err)
	}
	kvObserver := techCoreSpace.KeyValueObserver()
	deps := clientspace.TechSpaceDeps{
		CommonSpace:      techCoreSpace,
		ObjectFactory:    s.objectFactory,
		AccountService:   s.accountService,
		PersonalSpaceId:  s.personalSpaceId,
		Indexer:          s.indexer,
		Installer:        s.installer,
		TechSpace:        techSpace,
		KeyValueObserver: kvObserver,
	}
	ts, err := clientspace.NewTechSpace(deps)
	if err != nil {
		return nil, fmt.Errorf("build tech space: %w", err)
	}

	s.techSpace = ts
	s.app = s.app.ChildApp()
	s.app.Register(s.techSpace)
	err = ts.Run(techCoreSpace, ts.Cache, true)
	if err != nil {
		return nil, fmt.Errorf("run tech space: %w", err)
	}

	return ts, nil
}

func (s *spaceFactory) LoadAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	techSpace := techspace.New()
	id, err := s.spaceCore.DeriveID(ctx, spacedomain.SpaceTypeTech)
	if err != nil {
		return nil, fmt.Errorf("derive tech space id: %w", err)
	}
	techCoreSpace, err := s.spaceCore.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("derive tech space: %w", err)
	}
	kvObserver := techCoreSpace.KeyValueObserver()
	deps := clientspace.TechSpaceDeps{
		CommonSpace:      techCoreSpace,
		ObjectFactory:    s.objectFactory,
		AccountService:   s.accountService,
		PersonalSpaceId:  s.personalSpaceId,
		Indexer:          s.indexer,
		Installer:        s.installer,
		TechSpace:        techSpace,
		KeyValueObserver: kvObserver,
	}
	ts, err := clientspace.NewTechSpace(deps)
	if err != nil {
		return nil, fmt.Errorf("build tech space: %w", err)
	}
	s.techSpace = ts
	s.app = s.app.ChildApp()
	s.app.Register(s.techSpace)
	err = ts.Run(techCoreSpace, ts.Cache, false)
	if err != nil {
		return nil, fmt.Errorf("run tech space: %w", err)
	}
	err = s.indexer.ReindexSpace(ts)
	if err != nil {
		return nil, fmt.Errorf("reindex tech space: %w", err)
	}
	return ts, nil
}

func (s *spaceFactory) NewShareableSpace(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo) (spacecontroller.SpaceController, error) {
	return s.newShareableController(id)
}

func (s *spaceFactory) newShareableController(id string) (spacecontroller.SpaceController, error) {
	return accountspace.NewSpaceController(accountspace.Descriptor{
		SpaceId: id,
	}, s.app)
}

// CreateShareableSpace marks the freshly created space storage and creates
// its space view; the watcher registers the controller from the view event.
func (s *spaceFactory) CreateShareableSpace(ctx context.Context, id string, spaceDesc *spaceinfo.SpaceDescription) (err error) {
	coreSpace, err := s.spaceCore.Get(ctx, id)
	if err != nil {
		return
	}
	err = coreSpace.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx)
	if err != nil {
		return
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
	return s.techSpace.SpaceViewCreate(ctx, id, true, info, spaceDesc)
}

// CreateStreamableSpace creates a space view carrying the encoded guest key;
// the watcher registers a streamable controller from the view event.
func (s *spaceFactory) CreateStreamableSpace(ctx context.Context, privKey crypto.PrivKey, id string) error {
	encodedKey, err := crypto.EncodeKeyToString(privKey)
	if err != nil {
		return fmt.Errorf("encode guest key: %w", err)
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown).
		SetEncodedKey(encodedKey)
	return s.techSpace.SpaceViewCreate(ctx, id, false, info, nil)
}

func (s *spaceFactory) NewStreamableSpace(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo, metadata []byte) (spacecontroller.SpaceController, error) {
	decodedSignKey, err := crypto.DecodeKeyFromString(
		info.EncodedKey,
		crypto.UnmarshalEd25519PrivateKey,
		nil)
	if err != nil {
		return nil, fmt.Errorf("decode streamable space key: %w", err)
	}
	return accountspace.NewSpaceController(accountspace.Descriptor{
		SpaceId:       id,
		GuestKey:      decodedSignKey,
		OwnerMetadata: metadata,
	}, s.app)
}

// CreateMarketplaceSpace constructs the marketplace controller; the caller
// starts it (initMarketplaceSpace).
func (s *spaceFactory) CreateMarketplaceSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error) {
	return marketplacespace.NewSpaceController(s.app, s.personalSpaceId), nil
}

// CreateOneToOneSpace marks the one-to-one space storage and creates or
// repairs its space view; the watcher registers the controller from the view
// event.
func (s *spaceFactory) CreateOneToOneSpace(ctx context.Context, spaceId string, description *spaceinfo.SpaceDescription, participantData spaceinfo.OneToOneParticipantData) (err error) {
	oneToOneSpace, err := s.spaceCore.Get(ctx, spaceId)
	if err != nil {
		return
	}

	err = oneToOneSpace.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx)
	if err != nil {
		return
	}

	info := spaceinfo.NewSpacePersistentInfo(spaceId)
	info.OneToOneIdentity = participantData.Identity
	info.Name = description.Name
	requestMetadataKeyStr := base64.StdEncoding.EncodeToString(participantData.RequestMetadataKey)
	info.OneToOneRequestMetadataKey = requestMetadataKeyStr
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)

	spaceView, err := s.techSpace.GetSpaceView(ctx, spaceId)
	if err != nil {
		if !errors.Is(err, techspace.ErrSpaceViewNotExists) {
			return fmt.Errorf("get space view: %w", err)
		}
	}

	// nolint: nestif
	if spaceView == nil {
		if err := s.techSpace.SpaceViewCreate(ctx, spaceId, true, info, description); err != nil {
			return err
		}
	} else {
		// check if space is active
		existingLocalInfo := spaceView.GetLocalInfo()
		if existingLocalInfo.GetLocalStatus() != spaceinfo.LocalStatusOk {
			// space has been removed, reset statuses and recreate
			localInfo := spaceinfo.NewSpaceLocalInfo(spaceId)
			localInfo.SetLocalStatus(spaceinfo.LocalStatusUnknown)
			localInfo.SetRemoteStatus(spaceinfo.RemoteStatusUnknown)
			if err := spaceView.SetSpaceLocalInfo(localInfo); err != nil {
				return err
			}
			if err := spaceView.SetSpacePersistentInfo(info); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *spaceFactory) Name() (name string) {
	return CName
}
